package testkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theRunTheContractCheckPutsDown is the number the contract check says it ran
// the task under when it puts the task down.
const theRunTheContractCheckPutsDown = "7"

// theTimeAClaimIsGiven is how long the check waits, on the clock it hands the
// store, before it asks whether a claim let go is still counted: three hours,
// which is past the hour a task is given, so a claim a store kept rather than
// let go would have run out by then and been written down as a failure.
const theTimeAClaimIsGiven = 3 * time.Hour

// checkJobHoldsAPutDownTask is the middle of CheckJob: a job put down on its
// task is paused and carries the mark, the mark comes back as the one most
// recently put down, and a job set running again forgets it and hands the task
// it was paused on out first, whether it was put down or only paused. A store
// that forgets the mark leaves the person's "continue" with nothing to pick up
// after a restart, one that keeps it past a run-now would pick a task up that is
// already running, and one that keeps the claim the paused run held skips the
// task as running and then counts it as a failure when the claim runs out.
func checkJobHoldsAPutDownTask(ctx context.Context, jobs contract.Job, task contract.TaskToRun, now time.Time) error {
	if err := jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: "no-such-job", TaskID: task.TaskID}}); err == nil {
		return errors.New("putting down a task of a job that is not there returned no error, and it must name what is missing")
	}
	mark := contract.PutDownMark{Task: task, Run: theRunTheContractCheckPutsDown, HasRecord: true}
	if err := jobs.PutDown(ctx, mark); err != nil {
		return fmt.Errorf("putting the task down failed: %w", err)
	}
	if state, err := stateOfJob(ctx, jobs, task.JobID); err != nil || state != contract.JobPaused {
		return fmt.Errorf("a job put down on its task is %q (error %v), want %q, because nothing of a put-down job runs on its own", state, err, contract.JobPaused)
	}
	held, there, err := jobs.PutDownTask(ctx)
	if err != nil {
		return fmt.Errorf("asking for the put-down task failed: %w", err)
	}
	if !there || held != mark {
		return fmt.Errorf("the put-down task is %+v (there %v), want the mark that was just written, %+v", held, there, mark)
	}
	if err := jobs.RunNow(ctx, task.JobID); err != nil {
		return fmt.Errorf("setting the put-down job running again failed: %w", err)
	}
	if held, there, err := jobs.PutDownTask(ctx); err != nil || there {
		return fmt.Errorf("after the job was set running again the put-down task is %+v (there %v, error %v), want none, because a job set running again forgets its mark", held, there, err)
	}
	later := now.Add(theTimeAClaimIsGiven)
	if err := checkAJobSetRunningAgainHandsItsTaskOutFirst(ctx, jobs, task, later); err != nil {
		return err
	}
	// The same rule for a job that was only paused, with the task claimed.
	if err := jobs.Pause(ctx, task.JobID); err != nil {
		return fmt.Errorf("pausing the job failed: %w", err)
	}
	if err := jobs.RunNow(ctx, task.JobID); err != nil {
		return fmt.Errorf("setting the paused job running again failed: %w", err)
	}
	return checkAJobSetRunningAgainHandsItsTaskOutFirst(ctx, jobs, task, later.Add(theTimeAClaimIsGiven))
}

// checkAJobSetRunningAgainHandsItsTaskOutFirst asks, long after the claim the
// paused run held would have run out, for the next task: it must be the task
// the job was paused on, handed out again, with no failure counted for a claim
// the store was to let go.
func checkAJobSetRunningAgainHandsItsTaskOutFirst(ctx context.Context, jobs contract.Job, task contract.TaskToRun, later time.Time) error {
	handed, due, err := jobs.NextTask(ctx, later)
	if err != nil {
		return fmt.Errorf("asking for the next task after the job was set running again failed: %w", err)
	}
	if !due || handed.TaskID != task.TaskID {
		return fmt.Errorf("after the job was set running again the next task is %+v (due %v), want %s, the task it was paused on, because a job set running again lets go of the claims its paused run held", handed, due, task.TaskID)
	}
	listed, err := jobs.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the jobs failed: %w", err)
	}
	for _, summary := range listed {
		if summary.ID == task.JobID && summary.FailuresInARow != 0 {
			return fmt.Errorf("the job counts %d failures after it was set running again, want none: the claim its paused run held was let go, not left to run out", summary.FailuresInARow)
		}
	}
	return nil
}

// stateOfJob reads one job's state out of the listing.
func stateOfJob(ctx context.Context, jobs contract.Job, jobID string) (contract.JobState, error) {
	listed, err := jobs.List(ctx)
	if err != nil {
		return "", fmt.Errorf("listing the jobs failed: %w", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary.State, nil
		}
	}
	return "", fmt.Errorf("the job %s is not in the listing", jobID)
}
