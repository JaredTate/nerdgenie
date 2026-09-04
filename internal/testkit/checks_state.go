package testkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A check function takes the interface rather than the fake, and asserts the
// properties the contract promises. A unit test calls it on the fake and a live
// test calls it on the real thing, so the fake cannot drift from what it stands
// in for. Every check leaves whatever it was given the way it found it, apart
// from what it wrote itself.

// CheckClock asserts what every clock promises: time never goes backwards, a
// sleep of nothing returns at once, a cancelled sleep gives up, and a stopped
// ticker can be stopped twice without complaint.
func CheckClock(clock contract.Clock) error {
	first := clock.Now()
	if clock.Now().Before(first) {
		return errors.New("the clock went backwards between two readings, and time must never go backwards")
	}
	if err := clock.Sleep(context.Background(), 0); err != nil {
		return fmt.Errorf("sleeping for no time at all failed: %w", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := clock.Sleep(cancelled, time.Hour); err == nil {
		return errors.New("sleeping with a cancelled context returned no error, and it must return the context's error")
	}

	ticker := clock.NewTicker(time.Hour)
	ticker.Stop()
	ticker.Stop()
	return nil
}

// CheckStore asserts what every event log promises: sequence numbers grow, every
// read shape finds what was written, an event that is not there is an error, and
// a replay hands events back in order.
func CheckStore(ctx context.Context, store contract.Store) error {
	taskID := "contract-check"
	written := []int64{}
	for range 3 {
		sequence, err := store.Append(ctx, contract.Event{TaskID: taskID, Kind: contract.EventMessage})
		if err != nil {
			return fmt.Errorf("appending an event failed: %w", err)
		}
		written = append(written, sequence)
	}
	if written[0] >= written[1] || written[1] >= written[2] {
		return fmt.Errorf("the sequence numbers came back as %v, and they must grow", written)
	}

	if err := checkStoreReads(ctx, store, taskID, written); err != nil {
		return err
	}
	if _, err := store.ByID(ctx, written[2]+10000); err == nil {
		return errors.New("reading an event that is not there returned no error, and it must name the number")
	}
	return nil
}

// checkStoreReads asserts the four read shapes and the replay.
func checkStoreReads(ctx context.Context, store contract.Store, taskID string, written []int64) error {
	byTask, err := store.ByTask(ctx, taskID)
	if err != nil || len(byTask) != len(written) {
		return fmt.Errorf("reading by task gave %d events and error %v, want %d", len(byTask), err, len(written))
	}
	byKind, err := store.ByKind(ctx, contract.EventMessage)
	if err != nil || len(byKind) < len(written) {
		return fmt.Errorf("reading by kind gave %d events and error %v, want at least %d", len(byKind), err, len(written))
	}
	one, err := store.ByID(ctx, written[1])
	if err != nil || one.Sequence != written[1] {
		return fmt.Errorf("reading event %d gave sequence %d and error %v", written[1], one.Sequence, err)
	}
	span, err := store.ByRange(ctx, contract.EventRange{From: written[0], To: written[2]})
	if err != nil || len(span) < len(written) {
		return fmt.Errorf("reading the range gave %d events and error %v, want at least %d", len(span), err, len(written))
	}

	previous := int64(0)
	err = store.Replay(ctx, func(event contract.Event) error {
		if event.Sequence <= previous {
			return fmt.Errorf("the replay handed back event %d after event %d, and it must be in order", event.Sequence, previous)
		}
		previous = event.Sequence
		return nil
	})
	if err != nil {
		return fmt.Errorf("replaying the log failed: %w", err)
	}
	return nil
}

// CheckMemory asserts what every memory promises: a saved fact can be read back
// by its id, a fact that is not there is an error, the hint is never longer than
// three lines, and a fact with no text is refused.
func CheckMemory(ctx context.Context, memory contract.Memory) error {
	fact := contract.Fact{
		ID:       "contract-check-1",
		Text:     "the contract check wrote this fact",
		Source:   "internal/testkit",
		Recorded: time.Unix(1, 0).UTC(),
	}
	if err := memory.Save(ctx, []contract.Fact{fact}); err != nil {
		return fmt.Errorf("saving a fact failed: %w", err)
	}
	found, err := memory.Get(ctx, fact.ID)
	if err != nil {
		return fmt.Errorf("reading back the fact that was just saved failed: %w", err)
	}
	if found.Text != fact.Text {
		return fmt.Errorf("the fact came back as %q, want %q", found.Text, fact.Text)
	}
	if _, err := memory.Get(ctx, "no-such-fact"); err == nil {
		return errors.New("reading a fact that is not there returned no error, and it must name the id")
	}

	hint, err := memory.Hint(ctx, "contract check")
	if err != nil {
		return fmt.Errorf("asking for a hint failed: %w", err)
	}
	if len(hint) > contract.MemoryHintLines {
		return fmt.Errorf("the hint is %d lines, and the cap is %d", len(hint), contract.MemoryHintLines)
	}
	if err := memory.Save(ctx, []contract.Fact{{ID: "empty"}}); err == nil {
		return errors.New("saving a fact with no text returned no error, and a fact must say something")
	}
	return nil
}
