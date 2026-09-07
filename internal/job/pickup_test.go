package job_test

import (
	"testing"
	"time"
)

// TestAJobLetsATaskBePickedUpOnceAndRemembersItAcrossARestart: a task the
// harness's guard stopped is picked up by the job itself once, so the store
// answers yes the first time and no from then on, and it remembers across a
// restart, because the pick-up rides in the job's state snapshot in the log.
// A task that is finished or not there is refused with an error naming it.
func TestAJobLetsATaskBePickedUpOnceAndRemembersItAcrossARestart(t *testing.T) {
	ctx := t.Context()
	holding := newJobs(t)
	jobID := holding.aJob(t, "Post the campaign.")
	taskID := holding.aTask(t, jobID, "post the tweet", time.Time{})
	other := holding.aTask(t, jobID, "write the summary", time.Time{})

	first, err := holding.jobs.PickUpOnce(ctx, jobID, taskID)
	if err != nil || !first {
		t.Fatalf("the first pick-up answered %v (error %v), want yes", first, err)
	}
	holding = holding.restart(t)
	second, err := holding.jobs.PickUpOnce(ctx, jobID, taskID)
	if err != nil || second {
		t.Errorf("the second pick-up after a restart answered %v (error %v), want no: the pick-up is tried once", second, err)
	}
	if again, err := holding.jobs.PickUpOnce(ctx, jobID, other); err != nil || !again {
		t.Errorf("the other task's first pick-up answered %v (error %v), want yes: the count is per task", again, err)
	}
	if _, err := holding.jobs.PickUpOnce(ctx, jobID, "t99"); err == nil {
		t.Error("a task that is not on the list was picked up without an error naming it")
	}
	if _, err := holding.jobs.PickUpOnce(ctx, "99", taskID); err == nil {
		t.Error("a job that is not there was picked up without an error naming it")
	}
	if _, err := holding.jobs.FinishTask(ctx, jobID, other, "the summary is written", false); err != nil {
		t.Fatalf("cannot finish the other task: %v", err)
	}
	if _, err := holding.jobs.PickUpOnce(ctx, jobID, other); err == nil {
		t.Error("a finished task was picked up without an error naming it, and there is nothing of it left to pick up")
	}
}
