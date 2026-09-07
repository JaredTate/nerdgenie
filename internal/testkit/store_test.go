package testkit_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheFakeStoreNumbersEventsInTheOrderTheyArrive(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()

	for _, kind := range []contract.EventKind{contract.EventMessage, contract.EventToolCall, contract.EventReply} {
		if _, err := store.Append(ctx, contract.Event{TaskID: "17", Kind: kind, Body: json.RawMessage(`{}`)}); err != nil {
			t.Fatalf("appending a %s event failed: %v", kind, err)
		}
	}

	events, err := store.ByTask(ctx, "17")
	if err != nil {
		t.Fatalf("reading the task's events failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("the task has %d events, want 3", len(events))
	}
	if store.Count() != 3 {
		t.Errorf("the store counts %d events, want the 3 appended", store.Count())
	}
	for at, event := range events {
		if event.Sequence != int64(at+1) {
			t.Errorf("the event at position %d has sequence %d, want %d", at, event.Sequence, at+1)
		}
	}
}

func TestTheFakeStoreReadsBackByKindByIDAndByRange(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	for range 5 {
		if _, err := store.Append(ctx, contract.Event{TaskID: "17", Kind: contract.EventToolResult}); err != nil {
			t.Fatalf("appending an event failed: %v", err)
		}
	}
	if _, err := store.Append(ctx, contract.Event{TaskID: "18", Kind: contract.EventMessage}); err != nil {
		t.Fatalf("appending an event failed: %v", err)
	}

	results, err := store.ByKind(ctx, contract.EventToolResult)
	if err != nil || len(results) != 5 {
		t.Fatalf("reading by kind gave %d events and error %v, want 5 and no error", len(results), err)
	}
	one, err := store.ByID(ctx, 3)
	if err != nil || one.Sequence != 3 {
		t.Fatalf("reading event 3 gave sequence %d and error %v, want 3 and no error", one.Sequence, err)
	}
	span, err := store.ByRange(ctx, contract.EventRange{From: 2, To: 4})
	if err != nil || len(span) != 3 {
		t.Fatalf("reading the range 2 to 4 gave %d events and error %v, want 3 and no error", len(span), err)
	}
}

func TestTheFakeStoreSaysSoWhenAnEventIsNotThere(t *testing.T) {
	if _, err := testkit.NewFakeStore().ByID(context.Background(), 99); err == nil {
		t.Fatal("reading an event that is not there was reported as a success, want an error naming the number")
	}
}

func TestTheFakeStoreReplaysEveryEventInOrderAndStopsOnAnError(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	for range 4 {
		if _, err := store.Append(ctx, contract.Event{Kind: contract.EventMessage}); err != nil {
			t.Fatalf("appending an event failed: %v", err)
		}
	}

	seen := []int64{}
	if err := store.Replay(ctx, func(event contract.Event) error {
		seen = append(seen, event.Sequence)
		return nil
	}); err != nil {
		t.Fatalf("replaying failed: %v", err)
	}
	if len(seen) != 4 || seen[0] != 1 || seen[3] != 4 {
		t.Errorf("the replay handed back %v, want the four events in order", seen)
	}

	stopped := 0
	err := store.Replay(ctx, func(contract.Event) error {
		stopped++
		return errStopReplay
	})
	if err == nil {
		t.Error("the replay reported no error after the function returned one")
	}
	if stopped != 1 {
		t.Errorf("the replay handed back %d events after the first error, want 1", stopped)
	}
}

func TestTheFakeStoreKeepsTheStoreContract(t *testing.T) {
	if err := testkit.CheckStore(context.Background(), testkit.NewFakeStore()); err != nil {
		t.Fatalf("the fake store does not keep the store contract: %v", err)
	}
}

// errStopReplay is what a replay function returns to stop the replay.
var errStopReplay = replayStopped{}

// replayStopped is the error a test's replay function returns.
type replayStopped struct{}

// Error says what went wrong and what to do about it.
func (replayStopped) Error() string {
	return "the test stopped the replay on purpose, so this error is expected"
}
