// One task per tick, and a tick that was missed while the machine was asleep
// making one task rather than one for every hour that went by, is ZeroClaw's
// design, at ~/Code/zeroclaw/crates/zeroclaw-runtime/src/cron/scheduler.rs. The
// single timer that never sleeps longer than a minute, so that a schedule
// changed while the agent waits is noticed, is Prime's, at
// ~/Code/prime-agent/packages/coding-agent/src/core/cron-jobs.ts. The Go here is
// written fresh.

package job

import (
	"context"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// NextTask returns the next task that may start now: the first unfinished task
// nobody is running, of the oldest running job, whose due time has passed. A job
// with a schedule whose tick has come makes one task from its template first.
//
// A task dated next week does not hold up the one behind it. The list is read
// past a task that is waiting for its date rather than stopped at it, because a
// tick's task added today would otherwise sit behind a task the user dated for
// the end of the month and never run.
func (jobs *Jobs) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	if jobs.mayStartWork != nil && !jobs.mayStartWork() {
		return contract.TaskToRun{}, false, nil
	}
	if err := jobs.releaseTasksPastTheirBudget(ctx, now); err != nil {
		return contract.TaskToRun{}, false, err
	}

	for _, jobID := range jobs.order {
		held := jobs.held[jobID]
		if held.state.State != contract.JobRunning {
			continue
		}
		if err := jobs.tickSchedule(ctx, jobID, held, now); err != nil {
			return contract.TaskToRun{}, false, err
		}
		// A tick can stop the job it ticked, by filling its list, so where the
		// job stands is read again before any of its work is handed out.
		if held.state.State != contract.JobRunning {
			continue
		}
		next, due, err := jobs.taskOf(ctx, jobID, held, now)
		if err != nil {
			return contract.TaskToRun{}, false, err
		}
		if due {
			return next, true, nil
		}
	}
	return contract.TaskToRun{}, false, nil
}

// taskOf claims and returns the first task of one job that may start now. The
// caller holds the lock.
func (jobs *Jobs) taskOf(ctx context.Context, jobID string, held *heldJob, now time.Time) (contract.TaskToRun, bool, error) {
	running, err := jobs.claimedTasks(ctx, jobID, now)
	if err != nil {
		return contract.TaskToRun{}, false, err
	}
	for _, task := range held.keeper.Record().Work.Tasks {
		facts := held.state.facts(task.TaskID)
		if task.Done || running[task.TaskID] || facts.DueAt.After(now) {
			continue
		}
		won, err := jobs.claim(ctx, jobID, task.TaskID, now)
		if err != nil {
			return contract.TaskToRun{}, false, err
		}
		if !won {
			continue
		}
		return contract.TaskToRun{
			JobID: jobID, TaskID: task.TaskID, Text: task.Text, Unattended: facts.Unattended,
		}, true, nil
	}
	return contract.TaskToRun{}, false, nil
}

// tickSchedule makes one task from a job's template when its tick has come, and
// works out when the next tick is from the moment now rather than from the tick
// that has just fired. A machine that was asleep for a week therefore comes back
// to one task rather than to a hundred and sixty-eight. The caller holds the
// lock.
func (jobs *Jobs) tickSchedule(ctx context.Context, jobID string, held *heldJob, now time.Time) error {
	if held.state.Schedule == nil || held.state.NextRun.IsZero() || now.Before(held.state.NextRun) {
		return nil
	}
	firedAt := held.state.NextRun
	following, err := nextRun(*held.state.Schedule, now)
	if err != nil {
		return err
	}
	changed := held.state
	changed.NextRun = following
	if err := jobs.saveState(ctx, jobID, held, changed); err != nil {
		return err
	}

	_, err = jobs.addTask(ctx, jobID, held, held.state.Template, firedAt, true)
	if err == nil {
		return nil
	}
	// The one thing that can go wrong here and is not a broken database is the
	// task list being full, and a job whose list is full is finished with rather
	// than broken, so it is switched off with a line saying why.
	if len(held.keeper.Record().Work.Tasks) < MaxTasksPerJob {
		return err
	}
	return jobs.stopTheJob(ctx, jobID, held, contract.JobOff,
		fmt.Sprintf("job %s was switched off because its list has reached the %d tasks one job holds", jobID, MaxTasksPerJob),
		"a schedule that has ticked this many times has filled the job's list")
}

// releaseTasksPastTheirBudget gives up every claim whose task has been running
// longer than a task's budget and counts it as a failure, which is what happens
// to a task whose process died holding it. The caller holds the lock.
func (jobs *Jobs) releaseTasksPastTheirBudget(ctx context.Context, now time.Time) error {
	overdue, err := jobs.expiredClaims(ctx, now)
	if err != nil {
		return err
	}
	for _, claimed := range overdue {
		jobID, taskID := claimed[0], claimed[1]
		held, there := jobs.held[jobID]
		if !there {
			if err := jobs.release(ctx, jobID, taskID); err != nil {
				return err
			}
			continue
		}
		report := fmt.Sprintf("failed: task %s ran for longer than the %s a task is given and was given up", taskID, TaskBudget)
		if _, err := jobs.finishTask(ctx, jobID, held, taskID, report, true, now); err != nil {
			return err
		}
	}
	return nil
}

// Wait sleeps until there could be work to do, and never for longer than the
// clamp, so that a job created or changed while the agent waits is picked up
// within the minute. It returns as soon as the context is done.
//
// It also never comes back twice inside RestBetweenWaits. Work that is already
// past its moment makes the wait nothing at all, and the driver that asks may
// not be able to take that work yet, because a task of its own is running; with
// no rest it would ask again as fast as the processor allows and burn a whole
// core for the length of that task.
func (jobs *Jobs) Wait(ctx context.Context) error {
	if err := jobs.clock.Sleep(ctx, jobs.restStillOwed(jobs.clock.Now())); err != nil {
		return err
	}
	if err := jobs.clock.Sleep(ctx, jobs.timeUntilWork(jobs.clock.Now())); err != nil {
		return err
	}
	jobs.rememberThisWait(jobs.clock.Now())
	return nil
}

// restStillOwed is how much of the rest between two waits has not been taken
// yet, and is nothing at all for the first wait of all.
func (jobs *Jobs) restStillOwed(now time.Time) time.Duration {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	if jobs.lastWaitEnded.IsZero() {
		return 0
	}
	return RestBetweenWaits - now.Sub(jobs.lastWaitEnded)
}

// rememberThisWait writes down when a wait came back, which is what the next
// wait's rest is measured from.
func (jobs *Jobs) rememberThisWait(now time.Time) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	jobs.lastWaitEnded = now
}

// timeUntilWork is how long the store may sleep before something is due: the
// soonest date or tick of any running job, clamped to the minute, and never
// below nothing.
func (jobs *Jobs) timeUntilWork(now time.Time) time.Duration {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	waiting := TimerClamp
	for _, jobID := range jobs.order {
		held := jobs.held[jobID]
		if held.state.State != contract.JobRunning {
			continue
		}
		for _, moment := range jobs.momentsOf(held) {
			if moment.IsZero() {
				continue
			}
			if until := moment.Sub(now); until < waiting {
				waiting = until
			}
		}
	}
	if waiting < 0 {
		return 0
	}
	return waiting
}

// momentsOf is every moment one job could next want to be looked at: its next
// tick, and the date of each task still waiting for one.
func (jobs *Jobs) momentsOf(held *heldJob) []time.Time {
	moments := []time.Time{held.state.NextRun}
	for _, task := range held.keeper.Record().Work.Tasks {
		if !task.Done {
			moments = append(moments, held.state.facts(task.TaskID).DueAt)
		}
	}
	return moments
}
