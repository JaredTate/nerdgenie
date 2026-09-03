package main

import (
	"context"

	"github.com/JaredTate/coeus/internal/contract"
)

// timedStore is the event log with one rule added: an event written with no time
// on it is given the time it was written.
//
// It is here rather than in a writer because there are several writers and only
// one log. The record keeper, which saves a checkpoint and a tool result on
// every change, has no clock of its own, and a live run found every checkpoint
// and every tool result in the log dated the year one, which makes the log
// useless for saying when anything happened. Rather than hand a clock to each
// writer in turn and wait for the next one to forget, the log itself stamps
// whatever arrives without a time.
type timedStore struct {
	// under is the real log.
	under contract.Store
	// clock is where the time an event was written is read.
	clock contract.Clock
}

// timedEvents wraps one store so that every event it takes carries a time.
func timedEvents(under contract.Store, clock contract.Clock) contract.Store {
	return timedStore{under: under, clock: clock}
}

// Append writes one event, giving it the time it was written when it came with
// none of its own.
func (timed timedStore) Append(ctx context.Context, event contract.Event) (int64, error) {
	if event.Occurred.IsZero() {
		event.Occurred = timed.clock.Now()
	}
	return timed.under.Append(ctx, event)
}

// ByTask returns every event of one task or job, in order.
func (timed timedStore) ByTask(ctx context.Context, taskID string) ([]contract.Event, error) {
	return timed.under.ByTask(ctx, taskID)
}

// ByKind returns every event of one kind, in order.
func (timed timedStore) ByKind(ctx context.Context, kind contract.EventKind) ([]contract.Event, error) {
	return timed.under.ByKind(ctx, kind)
}

// ByID returns one event by its sequence number.
func (timed timedStore) ByID(ctx context.Context, sequence int64) (contract.Event, error) {
	return timed.under.ByID(ctx, sequence)
}

// ByRange returns every event in a span of sequence numbers.
func (timed timedStore) ByRange(ctx context.Context, span contract.EventRange) ([]contract.Event, error) {
	return timed.under.ByRange(ctx, span)
}

// Replay hands every event to a function in order.
func (timed timedStore) Replay(ctx context.Context, hand func(event contract.Event) error) error {
	return timed.under.Replay(ctx, hand)
}
