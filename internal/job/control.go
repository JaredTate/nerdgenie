package job

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// Pause stops the job after the running task finishes.
func (jobs *Jobs) Pause(ctx context.Context, jobID string) error {
	return jobs.setState(ctx, jobID, contract.JobPaused)
}

// SwitchOff stops the job for good.
func (jobs *Jobs) SwitchOff(ctx context.Context, jobID string) error {
	return jobs.setState(ctx, jobID, contract.JobOff)
}

// PutDown pauses a job on one of its tasks and writes the mark into the job's
// state, where a restart reads it back. The record's own status moves to
// waiting with it, which is what "/jobs" shows for a job holding on a task. A
// job that is switched off is refused, because the person ended it and a mark
// would let "continue" start it again behind their back; a task that is
// finished is refused, because there is nothing of it left to pick up.
func (jobs *Jobs) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	jobID, taskID := mark.Task.JobID, mark.Task.TaskID
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	if held.state.State == contract.JobOff {
		return fmt.Errorf("job %s is switched off, so its task %s is not put down; run /cron run %s to start the job again",
			jobID, taskID, jobID)
	}
	if _, err := unfinishedTask(held, jobID, taskID); err != nil {
		return err
	}
	return jobs.putDown(ctx, jobID, held, mark)
}

// putDown writes the mark and pauses the job on it. The caller holds the lock
// and has checked that the job may be put down on that task.
func (jobs *Jobs) putDown(ctx context.Context, jobID string, held *heldJob, mark contract.PutDownMark) error {
	if err := held.keeper.SetStatus(ctx, contract.RecordStatusOfJob(contract.JobPaused)); err != nil {
		return fmt.Errorf("cannot write the status of job %s put down on task %s: %w", jobID, mark.Task.TaskID, err)
	}
	changed := held.state
	changed.State, changed.PutDown = contract.JobPaused, &mark
	return jobs.saveState(ctx, jobID, held, changed)
}

// PutDownTask returns the mark of the paused job put down most recently, which
// is the one whose run is newest, because that is the task the person was just
// looking at. A job switched off is passed over: the person ended it, and a
// word that carries on is not meant for it.
func (jobs *Jobs) PutDownTask(_ context.Context) (contract.PutDownMark, bool, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	newest, there := contract.PutDownMark{}, false
	for _, jobID := range jobs.order {
		held := jobs.held[jobID]
		if held.state.PutDown == nil || held.state.State != contract.JobPaused {
			continue
		}
		if !there || contract.RunNumberOf(held.state.PutDown.Run) > contract.RunNumberOf(newest.Run) {
			newest, there = *held.state.PutDown, true
		}
	}
	return newest, there, nil
}

// Resume sets a paused job running again and nothing else: every task keeps
// its date, a schedule keeps its next tick, the claims the paused run held are
// let go, and the mark of a job put down on a task is forgotten. It also
// forgets the failures that stopped the job, so that the next failure starts
// the count afresh rather than stopping it at once. It is what the loop picks
// a put-down task up with, and what "/resume" does.
func (jobs *Jobs) Resume(ctx context.Context, jobID string) error {
	return jobs.setState(ctx, jobID, contract.JobRunning)
}

// RunNow starts the job's next task without waiting for its date. The date is
// taken off the next unfinished task rather than merely ignored once, because a
// run-now that only changed where the job stood would leave the task waiting for
// the very date the user has just overridden. A job with a schedule ticks at
// once as well. A job put down on a task runs that task first, with its claim
// let go and the mark forgotten, and every other task keeps its date: the
// task the person stopped is the one they mean, and a run-now that took the
// date off the first unfinished task ran a task dated next week ahead of it.
func (jobs *Jobs) RunNow(ctx context.Context, jobID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	next := theTaskRunNowStarts(held)
	if err := jobs.startWorking(ctx, jobID, held); err != nil {
		return err
	}
	changed := held.state
	if changed.Schedule != nil {
		changed.NextRun = jobs.clock.Now()
	}
	written := []record.NewJobTask{}
	for _, task := range held.keeper.Record().Work.Tasks {
		due := task.DueAt
		if task.TaskID == next {
			due = ""
		}
		written = append(written, record.NewJobTask{TaskID: task.TaskID, Text: task.Text, DueAt: due})
	}
	if next != "" {
		if err := held.keeper.Apply(ctx, record.Update{Tasks: written}); err != nil {
			return fmt.Errorf("cannot take the date off task %s of job %s: %w", next, jobID, err)
		}
		changed = changed.withTask(next, taskFacts{Unattended: changed.facts(next).Unattended})
	}
	if err := jobs.saveState(ctx, jobID, held, changed); err != nil {
		return err
	}
	return jobs.writeProgress(ctx, jobID, held)
}

// theTaskRunNowStarts is the task a run-now starts without waiting: the task
// the job is put down on when it is put down on one, and the first unfinished
// task otherwise, or nothing when every task is done. The caller holds the
// lock.
func theTaskRunNowStarts(held *heldJob) string {
	if held.state.PutDown != nil {
		return held.state.PutDown.Task.TaskID
	}
	for _, task := range held.keeper.Record().Work.Tasks {
		if !task.Done {
			return task.TaskID
		}
	}
	return ""
}

// SetMonitor says whether a job is watching for a change. A job that is watching
// keeps quiet on a tick whose report reads exactly the same as the last one, so
// that the model is woken only when what it is watching has really moved.
//
// The contract's NewJob has no field for this yet, so a job is set to watch
// through this type after it is created.
func (jobs *Jobs) SetMonitor(ctx context.Context, jobID string, watching bool) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	if held.state.Schedule == nil && watching {
		return fmt.Errorf("job %s has no schedule, so there is no tick for it to watch with, and watching is refused", jobID)
	}
	changed := held.state
	changed.Monitor = watching
	return jobs.saveState(ctx, jobID, held, changed)
}

// KeepRunningWhenItsTasksFail says this job is never paused or switched off for
// failing, however many of its tasks fail in a row. It is for a job whose work
// is to report what it finds: the nightly self-check finishes its task as failed
// on every night it finds a broken skill, which is the check working rather than
// the check breaking, and a check that switches itself off on the tenth such
// night goes quiet exactly when it is earning its keep. The failures are still
// counted and the incidents are still kept, so the user still reads what went
// wrong.
//
// The contract's NewJob has no field for this yet, so a job is told through this
// type after it is created, as SetMonitor is.
func (jobs *Jobs) KeepRunningWhenItsTasksFail(ctx context.Context, jobID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	if held.state.KeepRunning {
		return nil
	}
	changed := held.state
	changed.KeepRunning = true
	return jobs.saveState(ctx, jobID, held, changed)
}

// setState moves a job to where the user has put it and moves its record's own
// status with it.
func (jobs *Jobs) setState(ctx context.Context, jobID string, state contract.JobState) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	if state == contract.JobRunning {
		return jobs.startWorking(ctx, jobID, held)
	}
	if err := held.keeper.SetStatus(ctx, contract.RecordStatusOfJob(state)); err != nil {
		return fmt.Errorf("cannot write the status of job %s: %w", jobID, err)
	}
	changed := held.state
	changed.State = state
	return jobs.saveState(ctx, jobID, held, changed)
}

// startWorking puts a job back to running, lets go of the claims its tasks
// still hold so that the task it was paused on is handed out first rather than
// skipped as running, forgets the failures behind it and the task it was put
// down on, and wakes the store, because a job set running has work due at
// once. A job whose every task finished while it was paused closes here, the
// way it would have closed had it been running when the last one finished,
// because there is nothing left for it to run. The caller holds the lock.
func (jobs *Jobs) startWorking(ctx context.Context, jobID string, held *heldJob) error {
	if err := jobs.releaseTheClaimsOf(ctx, jobID); err != nil {
		return err
	}
	if err := held.keeper.SetStatus(ctx, contract.StatusRunning); err != nil {
		return fmt.Errorf("cannot set job %s running again: %w", jobID, err)
	}
	changed := held.state
	changed.State, changed.FailuresInARow, changed.Backoff, changed.PutDown = contract.JobRunning, 0, 0, nil
	if err := jobs.saveState(ctx, jobID, held, changed); err != nil {
		return err
	}
	jobs.wake()
	return jobs.closeIfEveryTaskIsDone(ctx, jobID, held)
}
