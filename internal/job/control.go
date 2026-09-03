package job

import (
	"context"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// Pause stops the job after the running task finishes.
func (jobs *Jobs) Pause(ctx context.Context, jobID string) error {
	return jobs.setState(ctx, jobID, contract.JobPaused)
}

// SwitchOff stops the job for good.
func (jobs *Jobs) SwitchOff(ctx context.Context, jobID string) error {
	return jobs.setState(ctx, jobID, contract.JobOff)
}

// Resume starts a job working again and forgets the failures that stopped it, so
// that the next failure starts the count afresh rather than stopping it at once.
//
// The contract has no Resume yet, so "/resume" reaches it through this type.
func (jobs *Jobs) Resume(ctx context.Context, jobID string) error {
	return jobs.setState(ctx, jobID, contract.JobRunning)
}

// RunNow starts the job's next task without waiting for its date. The date is
// taken off the next unfinished task rather than merely ignored once, because a
// run-now that only changed where the job stood would leave the task waiting for
// the very date the user has just overridden. A job with a schedule ticks at
// once as well.
func (jobs *Jobs) RunNow(ctx context.Context, jobID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	if err := jobs.startWorking(ctx, jobID, held); err != nil {
		return err
	}
	changed := held.state
	if changed.Schedule != nil {
		changed.NextRun = jobs.clock.Now()
	}
	next := ""
	written := []record.NewJobTask{}
	for _, task := range held.keeper.Record().Work.Tasks {
		due := task.DueAt
		if !task.Done && next == "" {
			next, due = task.TaskID, ""
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
	if err := held.keeper.SetStatus(ctx, recordStatusOfJob(state)); err != nil {
		return fmt.Errorf("cannot write the status of job %s: %w", jobID, err)
	}
	changed := held.state
	changed.State = state
	return jobs.saveState(ctx, jobID, held, changed)
}

// startWorking puts a job back to running and forgets the failures behind it.
// The caller holds the lock.
func (jobs *Jobs) startWorking(ctx context.Context, jobID string, held *heldJob) error {
	if err := held.keeper.SetStatus(ctx, contract.StatusRunning); err != nil {
		return fmt.Errorf("cannot set job %s running again: %w", jobID, err)
	}
	changed := held.state
	changed.State, changed.FailuresInARow, changed.Backoff = contract.JobRunning, 0, 0
	return jobs.saveState(ctx, jobID, held, changed)
}
