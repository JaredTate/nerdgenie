package testkit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// FakeStore is the event log kept in memory: append-only, numbered in the order
// events arrive, and readable the four ways the contract names.
type FakeStore struct {
	guard  sync.Mutex
	events []contract.Event
}

// NewFakeStore returns an empty log.
func NewFakeStore() *FakeStore {
	return &FakeStore{}
}

// Append writes one event and gives it the next sequence number.
func (store *FakeStore) Append(_ context.Context, event contract.Event) (int64, error) {
	store.guard.Lock()
	defer store.guard.Unlock()
	event.Sequence = int64(len(store.events) + 1)
	if event.Occurred.IsZero() {
		event.Occurred = time.Unix(int64(event.Sequence), 0).UTC()
	}
	store.events = append(store.events, event)
	return event.Sequence, nil
}

// ByTask returns every event of one task or job, in order.
func (store *FakeStore) ByTask(_ context.Context, taskID string) ([]contract.Event, error) {
	return store.matching(func(event contract.Event) bool { return event.TaskID == taskID }), nil
}

// ByKind returns every event of one kind, in order.
func (store *FakeStore) ByKind(_ context.Context, kind contract.EventKind) ([]contract.Event, error) {
	return store.matching(func(event contract.Event) bool { return event.Kind == kind }), nil
}

// ByID returns one event by its sequence number.
func (store *FakeStore) ByID(_ context.Context, sequence int64) (contract.Event, error) {
	store.guard.Lock()
	defer store.guard.Unlock()
	if sequence < 1 || sequence > int64(len(store.events)) {
		return contract.Event{}, fmt.Errorf("there is no event numbered %d in the log, which holds %d events", sequence, len(store.events))
	}
	return store.events[sequence-1], nil
}

// ByRange returns every event in a span of sequence numbers.
func (store *FakeStore) ByRange(_ context.Context, span contract.EventRange) ([]contract.Event, error) {
	return store.matching(func(event contract.Event) bool {
		return event.Sequence >= span.From && event.Sequence <= span.To
	}), nil
}

// Replay hands every event to the function in order, and stops at the first
// error the function returns.
func (store *FakeStore) Replay(_ context.Context, hand func(event contract.Event) error) error {
	for _, event := range store.snapshot() {
		if err := hand(event); err != nil {
			return fmt.Errorf("the replay stopped at event %d: %w", event.Sequence, err)
		}
	}
	return nil
}

// Count is how many events the log holds, which a test uses to assert that
// something was written.
func (store *FakeStore) Count() int {
	store.guard.Lock()
	defer store.guard.Unlock()
	return len(store.events)
}

// matching returns every event the test function accepts, in order.
func (store *FakeStore) matching(wanted func(event contract.Event) bool) []contract.Event {
	found := []contract.Event{}
	for _, event := range store.snapshot() {
		if wanted(event) {
			found = append(found, event)
		}
	}
	return found
}

// snapshot returns a copy of the log, so that a reader never holds the lock
// while somebody else is appending.
func (store *FakeStore) snapshot() []contract.Event {
	store.guard.Lock()
	defer store.guard.Unlock()
	copied := make([]contract.Event, len(store.events))
	copy(copied, store.events)
	return copied
}
