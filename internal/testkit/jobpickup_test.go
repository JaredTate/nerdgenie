package testkit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFakeJobPicksATaskUpOnceAndRefusesWhatIsNotThere: the fake answers
// the way the real store does, yes once and no after, and refuses a job that
// is not there, a task that is not on the list, and a task that is finished,
// each with an error naming it.
func TestTheFakeJobPicksATaskUpOnceAndRefusesWhatIsNotThere(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "post the campaign", Why: "the user asked"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "post the tweet"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}

	if first, err := jobs.PickUpOnce(ctx, jobID, taskID); err != nil || !first {
		t.Errorf("the first pick-up answered %v (error %v), want yes", first, err)
	}
	if second, err := jobs.PickUpOnce(ctx, jobID, taskID); err != nil || second {
		t.Errorf("the second pick-up answered %v (error %v), want no", second, err)
	}
	if _, err := jobs.PickUpOnce(ctx, "99", taskID); err == nil {
		t.Error("a job that is not there was picked up without an error naming it")
	}
	if _, err := jobs.PickUpOnce(ctx, jobID, "t99"); err == nil {
		t.Error("a task that is not on the list was picked up without an error naming it")
	}
	if _, err := jobs.FinishTask(ctx, jobID, taskID, "the tweet is up", false); err != nil {
		t.Fatalf("cannot finish the task: %v", err)
	}
	if _, err := jobs.PickUpOnce(ctx, jobID, taskID); err == nil {
		t.Error("a finished task was picked up without an error naming it")
	}
}

// stingyPickUpStore never lets a job pick a task up itself.
type stingyPickUpStore struct{ *testkit.FakeJob }

// PickUpOnce answers no every time.
func (store stingyPickUpStore) PickUpOnce(ctx context.Context, jobID string, taskID string) (bool, error) {
	_, err := store.FakeJob.PickUpOnce(ctx, jobID, taskID)
	return false, err
}

// forgivingPickUpStore lets a task of a job that is not there be picked up.
type forgivingPickUpStore struct{ *testkit.FakeJob }

// PickUpOnce answers yes for a job that is not there instead of an error.
func (store forgivingPickUpStore) PickUpOnce(ctx context.Context, jobID string, taskID string) (bool, error) {
	if jobID == "no-such-job" {
		return true, nil
	}
	return store.FakeJob.PickUpOnce(ctx, jobID, taskID)
}

// pickUpRefusingStore fails every pick-up with an error.
type pickUpRefusingStore struct{ *testkit.FakeJob }

// PickUpOnce fails with an error whatever it is asked.
func (store pickUpRefusingStore) PickUpOnce(context.Context, string, string) (bool, error) {
	return false, errors.New("the store cannot pick anything up today")
}

// secondPickUpFailingStore answers the first pick-up and fails the second.
type secondPickUpFailingStore struct {
	*testkit.FakeJob
	asked int
}

// PickUpOnce passes the first real ask on and fails the one after it.
func (store *secondPickUpFailingStore) PickUpOnce(ctx context.Context, jobID string, taskID string) (bool, error) {
	if jobID == "no-such-job" {
		return store.FakeJob.PickUpOnce(ctx, jobID, taskID)
	}
	store.asked++
	if store.asked > 1 {
		return false, errors.New("the store lost count of the pick-ups")
	}
	return store.FakeJob.PickUpOnce(ctx, jobID, taskID)
}

// TestTheJobCheckCatchesAStoreThatBreaksThePickUpPromise: the check fails a
// store that never allows the pick-up, one that allows it for a job that is
// not there, one that fails the first ask, and one that fails the second.
func TestTheJobCheckCatchesAStoreThatBreaksThePickUpPromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
	}{
		{"a store that never lets a job pick a task up itself", stingyPickUpStore{workingJobStore()}},
		{"a store that picks up a task of a job that is not there", forgivingPickUpStore{workingJobStore()}},
		{"a store that fails every pick-up", pickUpRefusingStore{workingJobStore()}},
		{"a store that fails the second pick-up", &secondPickUpFailingStore{FakeJob: workingJobStore()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckJob(ctx, test.jobs); err == nil {
				t.Fatal("the job check passed, and it was given a store that breaks the pick-up promise")
			}
		})
	}
}
