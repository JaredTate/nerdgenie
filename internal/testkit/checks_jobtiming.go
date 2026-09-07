package testkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theTimeTheFirstTaskTakes is how long CheckJobTiming lets its first task run
// on the clock before it takes the report, an odd span so that a store that
// rounds to the minute, or reads its own clock instead of the moment it was
// handed, fails here.
const theTimeTheFirstTaskTakes = 3*time.Minute + 12*time.Second

// CheckJobTiming asserts what every job store promises about time: the job
// is started when it is made, a task is started at the moment it is handed
// out and finished at the moment its report is taken, a task that has not
// started has no moments, the job has no finish while a task is left, and it
// is finished when its last task is. The clock is the one the store reads,
// so that the check can move it. It makes exactly one job of two tasks.
func CheckJobTiming(ctx context.Context, jobs contract.Job, clock *FakeClock) error {
	if _, err := jobs.Timing(ctx, "no-such-job"); err == nil {
		return errors.New("the timing of a job that is not there returned no error, and it must name what is missing")
	}
	made := clock.Now()
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "time two tasks", Name: "Timing"})
	if err != nil {
		return fmt.Errorf("creating the timed job failed: %w", err)
	}
	first, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the first timed task"})
	if err != nil {
		return fmt.Errorf("adding the first timed task failed: %w", err)
	}
	second, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the second timed task"})
	if err != nil {
		return fmt.Errorf("adding the second timed task failed: %w", err)
	}
	clock.Advance(time.Minute)
	handedOut := clock.Now()
	if err := takeTheTask(ctx, jobs, jobID, first, handedOut); err != nil {
		return err
	}
	clock.Advance(theTimeTheFirstTaskTakes)
	if _, err := jobs.FinishTask(ctx, jobID, first, "the first timed task is done", false); err != nil {
		return fmt.Errorf("finishing the first timed task failed: %w", err)
	}
	timing, err := jobs.Timing(ctx, jobID)
	if err != nil {
		return fmt.Errorf("reading the timing after the first task failed: %w", err)
	}
	if !timing.Started.Equal(made) {
		return fmt.Errorf("the job started at %v, want %v, the moment it was made on the store's clock", timing.Started, made)
	}
	if !timing.Finished.IsZero() {
		return fmt.Errorf("the job is finished at %v with a task still to run, want no finish yet", timing.Finished)
	}
	firstTiming := timing.Tasks[first]
	if !firstTiming.Started.Equal(handedOut) || !firstTiming.Finished.Equal(handedOut.Add(theTimeTheFirstTaskTakes)) {
		return fmt.Errorf("the first task's timing reads %+v, want started at %v, the moment it was handed out, and finished at %v, the moment its report was taken", firstTiming, handedOut, handedOut.Add(theTimeTheFirstTaskTakes))
	}
	if held, there := timing.Tasks[second]; there && (!held.Started.IsZero() || !held.Finished.IsZero()) {
		return fmt.Errorf("the second task has the timing %+v before it was handed out, want none", held)
	}
	return checkTheJobFinishesOnItsLastTask(ctx, jobs, clock, jobID, second, timing.Started)
}

// takeTheTask asks the store for its next task at the moment given and holds
// it to the one expected.
func takeTheTask(ctx context.Context, jobs contract.Job, jobID string, taskID string, now time.Time) error {
	handed, found, err := jobs.NextTask(ctx, now)
	if err != nil {
		return fmt.Errorf("asking for the next task failed: %w", err)
	}
	if !found || handed.JobID != jobID || handed.TaskID != taskID {
		return fmt.Errorf("the next task is %+v (found %v), want task %s of job %s", handed, found, taskID, jobID)
	}
	return nil
}

// checkTheJobFinishesOnItsLastTask runs the second task and holds the job to
// a finish at the moment its last report was taken, with its start unchanged.
func checkTheJobFinishesOnItsLastTask(ctx context.Context, jobs contract.Job, clock *FakeClock, jobID string, second string, started time.Time) error {
	if err := takeTheTask(ctx, jobs, jobID, second, clock.Now()); err != nil {
		return err
	}
	clock.Advance(time.Minute)
	if _, err := jobs.FinishTask(ctx, jobID, second, "the second timed task is done", false); err != nil {
		return fmt.Errorf("finishing the second timed task failed: %w", err)
	}
	timing, err := jobs.Timing(ctx, jobID)
	if err != nil {
		return fmt.Errorf("reading the timing after the last task failed: %w", err)
	}
	if !timing.Started.Equal(started) {
		return fmt.Errorf("the job's start moved to %v after its last task, want it kept at %v", timing.Started, started)
	}
	if !timing.Finished.Equal(clock.Now()) {
		return fmt.Errorf("the job finished at %v, want %v, the moment its last report was taken", timing.Finished, clock.Now())
	}
	if held := timing.Tasks[second]; !held.Finished.Equal(clock.Now()) {
		return fmt.Errorf("the second task's timing reads %+v, want it finished at %v", held, clock.Now())
	}
	return nil
}
