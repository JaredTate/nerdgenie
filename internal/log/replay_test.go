package log

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestReplayHandsEveryEventBackInOrder(t *testing.T) {
	eventLog := newTestLog(t)
	sequences := writeEvents(t, eventLog, threeTasks)

	handed := []int64{}
	err := eventLog.Replay(context.Background(), func(event contract.Event) error {
		handed = append(handed, event.Sequence)
		return nil
	})
	if err != nil {
		t.Fatalf("replaying the log failed: %v", err)
	}
	if !sameNumbers(handed, sequences) {
		t.Errorf("the replay handed back %v, want %v", handed, sequences)
	}
}

func TestReplayOnAnEmptyLogHandsBackNothing(t *testing.T) {
	eventLog := newTestLog(t)

	handed := 0
	err := eventLog.Replay(context.Background(), func(contract.Event) error {
		handed++
		return nil
	})
	if err != nil {
		t.Fatalf("replaying an empty log failed: %v", err)
	}
	if handed != 0 {
		t.Errorf("an empty log handed back %d events, want none", handed)
	}
}

func TestReplayStopsAtTheFirstError(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, threeTasks)
	refused := errors.New("the caller had seen enough of the log")

	handed := 0
	err := eventLog.Replay(context.Background(), func(contract.Event) error {
		handed++
		if handed == 2 {
			return refused
		}
		return nil
	})
	if !errors.Is(err, refused) {
		t.Fatalf("the replay returned %v, want the error the function returned", err)
	}
	if handed != 2 {
		t.Errorf("the replay handed back %d events, want it to stop at the second", handed)
	}
}

func TestReplaySaysSoAfterTheLogIsClosed(t *testing.T) {
	closed, err := Open(context.Background(), filepath.Join(t.TempDir(), "nerdgenie.db"))
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}

	err = closed.Replay(context.Background(), func(contract.Event) error { return nil })
	if err == nil {
		t.Error("replaying a closed log returned no error, and it must say the log is closed")
	}
}

func TestReplayStopsWhenTheContextIsCancelled(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, threeTasks)
	cancelling, cancel := context.WithCancel(context.Background())
	defer cancel()

	handed := 0
	err := eventLog.Replay(cancelling, func(contract.Event) error {
		handed++
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("replaying with a cancelled context returned no error, and it must give up")
	}
	if handed != 1 {
		t.Errorf("the replay handed back %d events after the context was cancelled, want 1", handed)
	}
}

func TestTheLogMeetsTheStoreContract(t *testing.T) {
	if err := testkit.CheckStore(context.Background(), newTestLog(t)); err != nil {
		t.Fatalf("the real event log does not keep the promises of contract.Store: %v", err)
	}
}

func TestReplayHandsBackTenThousandEventsInOrderAndReadsStopAtTheMaximum(t *testing.T) {
	eventLog := newTestLog(t)
	ctx := context.Background()
	written := MaxEventsPerRead + 1

	for number := 1; number <= written; number++ {
		if _, err := eventLog.Append(ctx, contract.Event{
			Occurred: aTime,
			TaskID:   "t1",
			Kind:     contract.EventToolResult,
			Body:     json.RawMessage(fmt.Sprintf(`{"step":%d}`, number)),
		}); err != nil {
			t.Fatalf("appending event %d failed: %v", number, err)
		}
	}

	previous := int64(0)
	handed := 0
	err := eventLog.Replay(ctx, func(event contract.Event) error {
		if event.Sequence != previous+1 {
			return fmt.Errorf("event %d came after event %d, and every sequence number is one more than the last", event.Sequence, previous)
		}
		wanted := fmt.Sprintf(`{"step":%d}`, event.Sequence)
		if string(event.Body) != wanted {
			return fmt.Errorf("event %d carries the body %s, want %s", event.Sequence, event.Body, wanted)
		}
		previous = event.Sequence
		handed++
		return nil
	})
	if err != nil {
		t.Fatalf("replaying ten thousand events failed: %v", err)
	}
	if handed != written {
		t.Errorf("the replay handed back %d events, want %d", handed, written)
	}

	found, err := eventLog.ByTask(ctx, "t1")
	if err == nil {
		t.Error("reading a task with more events than one read returns gave no error, and it must say it was cut short")
	}
	if len(found) != MaxEventsPerRead {
		t.Errorf("reading a task with %d events gave back %d of them, want the maximum of %d", written, len(found), MaxEventsPerRead)
	}
}

func TestManyGoroutinesAppendingGetUniqueAndDenseSequenceNumbers(t *testing.T) {
	eventLog := newTestLog(t)
	const writers = 8
	const eachWrites = 25

	seen := make(chan int64, writers*eachWrites)
	group := sync.WaitGroup{}
	for writer := range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range eachWrites {
				sequence, err := eventLog.Append(context.Background(), contract.Event{
					Occurred: aTime,
					TaskID:   fmt.Sprintf("t%d", writer),
					Kind:     contract.EventMessage,
				})
				if err != nil {
					t.Errorf("appending from writer %d failed: %v", writer, err)
					return
				}
				seen <- sequence
			}
		}()
	}
	group.Wait()
	close(seen)

	numbers := []int64{}
	for sequence := range seen {
		numbers = append(numbers, sequence)
	}
	slices.Sort(numbers)
	if len(numbers) != writers*eachWrites {
		t.Fatalf("%d events were appended, want %d", len(numbers), writers*eachWrites)
	}
	for position, sequence := range numbers {
		if sequence != int64(position+1) {
			t.Fatalf("the sequence numbers are %v at position %d, and they must be one to %d with no gaps and no repeats", sequence, position, len(numbers))
		}
	}
}
