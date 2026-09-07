package testkit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The timing check exists to keep the fake and the real store from drifting
// on when a job and its tasks started and finished. Each store below breaks
// one promise, and the check must name it.

// storeWithNoMoments keeps a job but says nothing about time.
type storeWithNoMoments struct{ *testkit.FakeJob }

func (store storeWithNoMoments) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	_, err := store.FakeJob.Timing(ctx, jobID)
	return contract.JobTiming{}, err
}

// storeThatKnowsEveryJob answers a timing for a job that is not there.
type storeThatKnowsEveryJob struct{ *testkit.FakeJob }

func (store storeThatKnowsEveryJob) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, _ := store.FakeJob.Timing(ctx, jobID)
	return timing, nil
}

// storeThatForgetsAFinish keeps every start and drops every finish.
type storeThatForgetsAFinish struct{ *testkit.FakeJob }

func (store storeThatForgetsAFinish) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	for taskID, held := range timing.Tasks {
		timing.Tasks[taskID] = contract.TaskTiming{Started: held.Started}
	}
	return timing, err
}

// storeThatFinishesTheJobEarly says the job is finished as soon as it is made.
type storeThatFinishesTheJobEarly struct{ *testkit.FakeJob }

func (store storeThatFinishesTheJobEarly) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	if timing.Finished.IsZero() {
		timing.Finished = timing.Started
	}
	return timing, err
}

// storeThatStartsEveryTaskAtOnce gives a waiting task the job's own start.
type storeThatStartsEveryTaskAtOnce struct{ *testkit.FakeJob }

func (store storeThatStartsEveryTaskAtOnce) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	for _, task := range store.FakeJob.Tasks(jobID) {
		if _, there := timing.Tasks[task.TaskID]; !there {
			timing.Tasks[task.TaskID] = contract.TaskTiming{Started: timing.Started}
		}
	}
	return timing, err
}

// storeThatMovesTheStart moves the job's start to its finish once it closes.
type storeThatMovesTheStart struct{ *testkit.FakeJob }

func (store storeThatMovesTheStart) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	if !timing.Finished.IsZero() {
		timing.Started = timing.Finished
	}
	return timing, err
}

// storeThatNeverClosesTheJob keeps the job open after its last task.
type storeThatNeverClosesTheJob struct{ *testkit.FakeJob }

func (store storeThatNeverClosesTheJob) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	timing.Finished = time.Time{}
	return timing, err
}

// storeThatFinishesTheLastTaskLate says the last task finished a minute after
// its report was taken.
type storeThatFinishesTheLastTaskLate struct{ *testkit.FakeJob }

func (store storeThatFinishesTheLastTaskLate) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	if !timing.Finished.IsZero() {
		for taskID, held := range timing.Tasks {
			if held.Finished.Equal(timing.Finished) {
				timing.Tasks[taskID] = contract.TaskTiming{Started: held.Started, Finished: held.Finished.Add(time.Minute)}
			}
		}
	}
	return timing, err
}

// storeThatHandsOutNothing never has a task to hand out.
type storeThatHandsOutNothing struct{ *testkit.FakeJob }

func (store storeThatHandsOutNothing) NextTask(context.Context, time.Time) (contract.TaskToRun, bool, error) {
	return contract.TaskToRun{}, false, nil
}

// errTheStoreIsBrokenOnPurpose is the one error every failing store below answers with.
var errTheStoreIsBrokenOnPurpose = errors.New("the store is broken on purpose")

// storeThatCannotCreate refuses every job.
type storeThatCannotCreate struct{ *testkit.FakeJob }

func (storeThatCannotCreate) Create(context.Context, contract.NewJob) (string, error) {
	return "", errTheStoreIsBrokenOnPurpose
}

// storeThatCannotAddATask refuses every task.
type storeThatCannotAddATask struct{ *testkit.FakeJob }

func (storeThatCannotAddATask) AddTask(context.Context, contract.NewTask) (string, error) {
	return "", errTheStoreIsBrokenOnPurpose
}

// storeThatCannotHandOut fails when asked for the next task.
type storeThatCannotHandOut struct{ *testkit.FakeJob }

func (storeThatCannotHandOut) NextTask(context.Context, time.Time) (contract.TaskToRun, bool, error) {
	return contract.TaskToRun{}, false, errTheStoreIsBrokenOnPurpose
}

// storeThatCannotFinish fails when a report is brought back.
type storeThatCannotFinish struct{ *testkit.FakeJob }

func (storeThatCannotFinish) FinishTask(context.Context, string, string, string, bool) (string, error) {
	return "", errTheStoreIsBrokenOnPurpose
}

// storeThatCannotTellTheTime fails on the timing of a job that is there.
type storeThatCannotTellTheTime struct{ *testkit.FakeJob }

func (store storeThatCannotTellTheTime) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	if _, err := store.FakeJob.Timing(ctx, jobID); err != nil {
		return contract.JobTiming{}, err
	}
	return contract.JobTiming{}, errTheStoreIsBrokenOnPurpose
}

// storeThatCannotFinishTheLastTask fails on the second report only.
type storeThatCannotFinishTheLastTask struct {
	*testkit.FakeJob
	reports int
}

func (store *storeThatCannotFinishTheLastTask) FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	store.reports++
	if store.reports > 1 {
		return "", errTheStoreIsBrokenOnPurpose
	}
	return store.FakeJob.FinishTask(ctx, jobID, taskID, report, failed)
}

// storeThatLosesTheTimeAtTheEnd fails on the timing once the job is closed.
type storeThatLosesTheTimeAtTheEnd struct{ *testkit.FakeJob }

func (store storeThatLosesTheTimeAtTheEnd) Timing(ctx context.Context, jobID string) (contract.JobTiming, error) {
	timing, err := store.FakeJob.Timing(ctx, jobID)
	if err == nil && !timing.Finished.IsZero() {
		return contract.JobTiming{}, errTheStoreIsBrokenOnPurpose
	}
	return timing, err
}

func TestTheTimingCheckCatchesAStoreThatBreaksAPromise(t *testing.T) {
	for _, test := range []struct {
		name   string
		broken func(fake *testkit.FakeJob) contract.Job
		words  string
	}{
		{"no moments", func(fake *testkit.FakeJob) contract.Job { return storeWithNoMoments{fake} }, "the job started at"},
		{"knows every job", func(fake *testkit.FakeJob) contract.Job { return storeThatKnowsEveryJob{fake} }, "not there returned no error"},
		{"forgets a finish", func(fake *testkit.FakeJob) contract.Job { return storeThatForgetsAFinish{fake} }, "the first task's timing reads"},
		{"finishes the job early", func(fake *testkit.FakeJob) contract.Job { return storeThatFinishesTheJobEarly{fake} }, "with a task still to run"},
		{"starts every task at once", func(fake *testkit.FakeJob) contract.Job { return storeThatStartsEveryTaskAtOnce{fake} }, "before it was handed out"},
		{"moves the start", func(fake *testkit.FakeJob) contract.Job { return storeThatMovesTheStart{fake} }, "the job's start moved"},
		{"never closes the job", func(fake *testkit.FakeJob) contract.Job { return storeThatNeverClosesTheJob{fake} }, "the job finished at"},
		{"finishes the last task late", func(fake *testkit.FakeJob) contract.Job { return storeThatFinishesTheLastTaskLate{fake} }, "the second task's timing reads"},
		{"hands out nothing", func(fake *testkit.FakeJob) contract.Job { return storeThatHandsOutNothing{fake} }, "the next task is"},
		{"cannot create", func(fake *testkit.FakeJob) contract.Job { return storeThatCannotCreate{fake} }, "creating the timed job failed"},
		{"cannot add a task", func(fake *testkit.FakeJob) contract.Job { return storeThatCannotAddATask{fake} }, "adding the first timed task failed"},
		{"cannot hand out", func(fake *testkit.FakeJob) contract.Job { return storeThatCannotHandOut{fake} }, "asking for the next task failed"},
		{"cannot finish", func(fake *testkit.FakeJob) contract.Job { return storeThatCannotFinish{fake} }, "finishing the first timed task failed"},
		{"cannot tell the time", func(fake *testkit.FakeJob) contract.Job { return storeThatCannotTellTheTime{fake} }, "reading the timing after the first task failed"},
		{"cannot finish the last task", func(fake *testkit.FakeJob) contract.Job { return &storeThatCannotFinishTheLastTask{FakeJob: fake} }, "finishing the second timed task failed"},
		{"loses the time at the end", func(fake *testkit.FakeJob) contract.Job { return storeThatLosesTheTimeAtTheEnd{fake} }, "reading the timing after the last task failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := testkit.NewFakeClock(time.Unix(1700000000, 0).UTC())
			err := testkit.CheckJobTiming(context.Background(), test.broken(testkit.NewFakeJob(clock)), clock)
			if err == nil {
				t.Fatalf("the timing check passed a store that %s", test.name)
			}
			if !strings.Contains(err.Error(), test.words) {
				t.Errorf("the timing check said %q, want it to say %q", err.Error(), test.words)
			}
		})
	}
}
