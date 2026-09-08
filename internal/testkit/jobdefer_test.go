package testkit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFakeJobDefersATaskOnceAndPassesOverIt: the fake answers the way
// the real store does. The first deferral of a task answers yes and the
// second no; NextTask passes over a deferred task while another task ahead
// of the job's last is unfinished and not deferred, and hands it out again
// before the last task once only deferred tasks remain there; and a job that
// is not there, a task not on the list, and a finished task are refused with
// an error naming them.
func TestTheFakeJobDefersATaskOnceAndPassesOverIt(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "build the flight simulator", Why: "the user asked"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	sky, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the sky and atmosphere"})
	if err != nil {
		t.Fatalf("cannot add the sky task: %v", err)
	}
	cockpit, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the cockpit"})
	if err != nil {
		t.Fatalf("cannot add the cockpit task: %v", err)
	}
	if _, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the final regression"}); err != nil {
		t.Fatalf("cannot add the regression task: %v", err)
	}
	if next, due, err := jobs.NextTask(ctx, time.Unix(0, 0).UTC()); err != nil || !due || next.TaskID != sky {
		t.Fatalf("the first task handed out is %+v (due %v, error %v), want the sky task", next, due, err)
	}

	if first, err := jobs.Defer(ctx, jobID, sky); err != nil || !first {
		t.Errorf("the first deferral answered %v (error %v), want yes", first, err)
	}
	if second, err := jobs.Defer(ctx, jobID, sky); err != nil || second {
		t.Errorf("the second deferral answered %v (error %v), want no", second, err)
	}
	if next, due, err := jobs.NextTask(ctx, time.Unix(0, 0).UTC()); err != nil || !due || next.TaskID != cockpit {
		t.Errorf("after the deferral the next task is %+v (due %v, error %v), want the cockpit task: a deferred task is passed over", next, due, err)
	}
	if _, err := jobs.Defer(ctx, "99", sky); err == nil {
		t.Error("a job that is not there had a task deferred without an error naming it")
	}
	if _, err := jobs.Defer(ctx, jobID, "t99"); err == nil {
		t.Error("a task that is not on the list was deferred without an error naming it")
	}
	if _, err := jobs.FinishTask(ctx, jobID, cockpit, "the cockpit is built", false); err != nil {
		t.Fatalf("cannot finish the cockpit task: %v", err)
	}
	if next, due, err := jobs.NextTask(ctx, time.Unix(0, 0).UTC()); err != nil || !due || next.TaskID != sky {
		t.Errorf("once only the deferred task remains ahead of the last the next task is %+v (due %v, error %v), want the sky task again, before the regression", next, due, err)
	}
	if _, err := jobs.FinishTask(ctx, jobID, sky, "the sky is drawn", false); err != nil {
		t.Fatalf("cannot finish the sky task: %v", err)
	}
	if _, err := jobs.Defer(ctx, jobID, sky); err == nil {
		t.Error("a finished task was deferred without an error naming it")
	}
}

// stingyDeferStore never sets a task aside.
type stingyDeferStore struct{ *testkit.FakeJob }

// Defer answers no every time.
func (store stingyDeferStore) Defer(ctx context.Context, jobID string, taskID string) (bool, error) {
	_, err := store.FakeJob.Defer(ctx, jobID, taskID)
	return false, err
}

// forgivingDeferStore sets aside a task of a job that is not there.
type forgivingDeferStore struct{ *testkit.FakeJob }

// Defer answers yes for a job that is not there instead of an error.
func (store forgivingDeferStore) Defer(ctx context.Context, jobID string, taskID string) (bool, error) {
	if jobID == "no-such-job" {
		return true, nil
	}
	return store.FakeJob.Defer(ctx, jobID, taskID)
}

// deferRefusingStore fails every deferral with an error.
type deferRefusingStore struct{ *testkit.FakeJob }

// Defer fails with an error whatever it is asked.
func (store deferRefusingStore) Defer(context.Context, string, string) (bool, error) {
	return false, errors.New("the store cannot set anything aside today")
}

// generousDeferStore sets a task aside as often as it is asked, where the
// promise is once.
type generousDeferStore struct{ *testkit.FakeJob }

// Defer answers yes every time.
func (store generousDeferStore) Defer(ctx context.Context, jobID string, taskID string) (bool, error) {
	_, err := store.FakeJob.Defer(ctx, jobID, taskID)
	return true, err
}

// TestTheJobCheckCatchesAStoreThatBreaksTheDeferPromise: the check fails a
// store that never sets a task aside, one that sets aside a task of a job
// that is not there, one that fails every deferral, and one that sets the
// same task aside twice.
func TestTheJobCheckCatchesAStoreThatBreaksTheDeferPromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
	}{
		{"a store that never sets a task aside", stingyDeferStore{workingJobStore()}},
		{"a store that sets aside a task of a job that is not there", forgivingDeferStore{workingJobStore()}},
		{"a store that fails every deferral", deferRefusingStore{workingJobStore()}},
		{"a store that sets the same task aside twice", generousDeferStore{workingJobStore()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckJob(ctx, test.jobs); err == nil {
				t.Fatal("the job check passed, and it was given a store that breaks the defer promise")
			}
		})
	}
}
