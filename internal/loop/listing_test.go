package loop_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheTasksListingReadsTheLogOnce is the gate review's fifteenth finding.
// The listing asked the log for every checkpoint to find the task numbers and
// then loaded each of those tasks in full, which reads that task's whole
// checkpoint history again, so listing fifty tasks read the log fifty-one times
// over for fifty lines of text.
func TestTheTasksListingReadsTheLogOnce(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["the notes are read"]}`)),
		answerStep("The notes name two accounts. Which one should I use?"),
		callStep("Nothing is read yet. I will read the brand file.",
			callFor("c2", "read", `{"path":"brand.md"}`),
			taskCall("c2t", `{"why":"the user wants the brand file read","doneWhen":["the brand file is read"]}`)),
		answerStep("The brand file is short. Shall I go on?"),
	}, scriptedTool("read", "the notes", "the brand file"))
	built.ask(t, "read the notes")
	built.ask(t, "read the brand file")

	counted := &countingStore{inner: built.store}
	options := built.options()
	options.Store = counted
	listing, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the counting store: %v", err)
	}

	printed, err := listing.TasksCommand().Run(t.Context(), "", contract.CommandContext{})
	if err != nil {
		t.Fatalf("the tasks command could not list the tasks: %v", err)
	}

	if counted.byTask != 0 {
		t.Errorf("the listing read one task's whole history %d times, and a listing reads the newest checkpoint of each",
			counted.byTask)
	}
	if counted.byKind != 1 {
		t.Errorf("the listing read the log %d times, and once is enough for every task's newest checkpoint", counted.byKind)
	}
	lines := strings.Split(strings.TrimSpace(printed), "\n")
	if len(lines) != 2 {
		t.Fatalf("the listing printed %d lines for two tasks: %q", len(lines), printed)
	}
	if !strings.HasPrefix(lines[0], "task 2") || !strings.Contains(lines[0], "read the brand file") {
		t.Errorf("the newest line reads %q, want task 2 with its ask and where it stands", lines[0])
	}
	if !strings.Contains(lines[1], string(contract.StatusWaiting)) {
		t.Errorf("the older line reads %q, and it says where that task stands", lines[1])
	}
}

// countingStore is the event log with a count of how many times it was read,
// which is how a test sees a listing reading the log over and over.
type countingStore struct {
	inner  *testkit.FakeStore
	byTask int
	byKind int
}

// Append writes one event.
func (store *countingStore) Append(ctx context.Context, event contract.Event) (int64, error) {
	return store.inner.Append(ctx, event)
}

// ByTask returns one task's events and counts the read.
func (store *countingStore) ByTask(ctx context.Context, taskID string) ([]contract.Event, error) {
	store.byTask++
	return store.inner.ByTask(ctx, taskID)
}

// ByKind returns every event of one kind and counts the read.
func (store *countingStore) ByKind(ctx context.Context, kind contract.EventKind) ([]contract.Event, error) {
	store.byKind++
	return store.inner.ByKind(ctx, kind)
}

// ByID returns one event.
func (store *countingStore) ByID(ctx context.Context, sequence int64) (contract.Event, error) {
	return store.inner.ByID(ctx, sequence)
}

// ByRange returns the events of one span.
func (store *countingStore) ByRange(ctx context.Context, span contract.EventRange) ([]contract.Event, error) {
	return store.inner.ByRange(ctx, span)
}

// Replay hands every event to the function given.
func (store *countingStore) Replay(ctx context.Context, hand func(event contract.Event) error) error {
	return store.inner.Replay(ctx, hand)
}
