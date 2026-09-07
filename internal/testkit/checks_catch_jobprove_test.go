package testkit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// forgetfulDoneList is a store that drops the done list a job was made with.
type forgetfulDoneList struct{ *testkit.FakeJob }

// Create makes the job without its done lines.
func (store forgetfulDoneList) Create(ctx context.Context, wanted contract.NewJob) (string, error) {
	wanted.DoneWhen = nil
	return store.FakeJob.Create(ctx, wanted)
}

// deafToMarks is a store whose marks change nothing.
type deafToMarks struct{ *testkit.FakeJob }

// ProveDoneLine answers yes and marks nothing.
func (store deafToMarks) ProveDoneLine(context.Context, string, int, string) error { return nil }

// tooEagerToClose is a store that closes a job whose done lines are unproved.
type tooEagerToClose struct{ *testkit.FakeJob }

// FinishTask finishes the task and closes the job whatever its done list says.
func (store tooEagerToClose) FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	reportID, err := store.FakeJob.FinishTask(ctx, jobID, taskID, report, failed)
	if err != nil {
		return "", err
	}
	for number := 1; number <= 2; number++ {
		if err := store.FakeJob.ProveDoneLine(ctx, jobID, number, reportID); err != nil {
			return "", err
		}
	}
	return reportID, nil
}

// blindToBadLines is a store that marks a line that is not there without a word.
type blindToBadLines struct{ *testkit.FakeJob }

// ProveDoneLine says nothing about a line past the list's end.
func (store blindToBadLines) ProveDoneLine(ctx context.Context, jobID string, number int, resultID string) error {
	if number > 2 {
		return nil
	}
	return store.FakeJob.ProveDoneLine(ctx, jobID, number, resultID)
}

// stickyMarks is a store that cannot unmark a line.
type stickyMarks struct{ *testkit.FakeJob }

// ProveDoneLine keeps a mark whatever it is told.
func (store stickyMarks) ProveDoneLine(ctx context.Context, jobID string, number int, resultID string) error {
	if resultID == "" {
		return nil
	}
	return store.FakeJob.ProveDoneLine(ctx, jobID, number, resultID)
}

// neverCloses is a store that marks every line and never closes the job.
type neverCloses struct{ *testkit.FakeJob }

// Load reads the job back as running whatever its lines say.
func (store neverCloses) Load(ctx context.Context, jobID string) (contract.Record, error) {
	held, err := store.FakeJob.Load(ctx, jobID)
	held.Header.Status = contract.StatusRunning
	return held, err
}

// TestTheDoneListCheckCatchesAStoreThatDropsMarksOrCloses proves the check is
// not a phantom: a store that loses the done list, one whose marks change
// nothing, and one that closes on unproved lines each fail it, with the fault
// named.
func TestTheDoneListCheckCatchesAStoreThatDropsMarksOrCloses(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name  string
		store contract.Job
		fault string
	}{
		{"a store that drops the done list", forgetfulDoneList{testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))}, "done lines"},
		{"a store whose marks change nothing", deafToMarks{testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))}, "returned no error"},
		{"a store that closes on unproved lines", tooEagerToClose{testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))}, "still running"},
		{"a store that marks a line past the end", blindToBadLines{testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))}, "two-line list"},
		{"a store that cannot unmark", stickyMarks{testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))}, "still marked"},
		{"a store that never closes", neverCloses{testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))}, "want done"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := testkit.CheckJobProvesDoneLines(ctx, test.store)
			if err == nil {
				t.Fatalf("%s passed the check", test.name)
			}
			if !strings.Contains(err.Error(), test.fault) {
				t.Errorf("%s failed with %q, want the fault named with %q", test.name, err, test.fault)
			}
		})
	}
}
