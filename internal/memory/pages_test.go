package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aPagedLog is an event log that answers a list read the way internal/log does:
// it hands back the events it read and, when it filled its cap, an error saying
// to read the rest in pages with ByRange. It is how a task with more events than
// one read returns can be put under test without writing ten thousand rows
// first.
type aPagedLog struct {
	// events is everything the log holds, numbered from one.
	events []contract.Event
	// perRead is how many events one list read hands back before it is cut
	// short, which is what internal/log calls its cap.
	perRead int
	// listFailure is the error every list read fails with when it is set, so
	// that a read that really went wrong can be told apart from one that was cut
	// short.
	listFailure error
	// rangeFailure is the error every read of a span fails with when it is set,
	// which is what a log that breaks part way through a task looks like.
	rangeFailure error
	// spans is every span of numbers that has been asked for, in order, which is
	// how a test sees where a reader started.
	spans []contract.EventRange
	// replays is how many times the whole log has been handed out event by
	// event, which is the work a reader must not do again on every open.
	replays int
}

// Append adds one event and gives it the next number.
func (paged *aPagedLog) Append(_ context.Context, event contract.Event) (int64, error) {
	event.Sequence = int64(len(paged.events) + 1)
	paged.events = append(paged.events, event)
	return event.Sequence, nil
}

// ByTask returns the events of one task, cut short at the cap.
func (paged *aPagedLog) ByTask(_ context.Context, taskID string) ([]contract.Event, error) {
	if paged.listFailure != nil {
		return paged.page(func(event contract.Event) bool { return event.TaskID == taskID }), paged.listFailure
	}
	return paged.read(func(event contract.Event) bool { return event.TaskID == taskID })
}

// ByKind returns the events of one kind, cut short at the cap.
func (paged *aPagedLog) ByKind(_ context.Context, kind contract.EventKind) ([]contract.Event, error) {
	return paged.read(func(event contract.Event) bool { return event.Kind == kind })
}

// ByID returns one event by its number.
func (paged *aPagedLog) ByID(_ context.Context, sequence int64) (contract.Event, error) {
	if sequence < 1 || sequence > int64(len(paged.events)) {
		return contract.Event{}, fmt.Errorf("there is no event numbered %d in this log", sequence)
	}
	return paged.events[sequence-1], nil
}

// ByRange returns the events in a span of numbers, cut short at the cap.
func (paged *aPagedLog) ByRange(_ context.Context, span contract.EventRange) ([]contract.Event, error) {
	paged.spans = append(paged.spans, span)
	if paged.rangeFailure != nil {
		return nil, paged.rangeFailure
	}
	return paged.read(func(event contract.Event) bool {
		return event.Sequence >= span.From && event.Sequence <= span.To
	})
}

// Replay hands every event to a function in order.
func (paged *aPagedLog) Replay(_ context.Context, hand func(event contract.Event) error) error {
	paged.replays++
	for _, event := range paged.events {
		if err := hand(event); err != nil {
			return err
		}
	}
	return nil
}

// read returns one page of the events a test function accepts, together with the
// error internal/log gives when a read fills its cap.
func (paged *aPagedLog) read(wanted func(event contract.Event) bool) ([]contract.Event, error) {
	found := paged.page(wanted)
	if len(found) < paged.perRead {
		return found, nil
	}
	return found, fmt.Errorf("this read of the log stopped at %d events, which is all one read returns, "+
		"so read the rest in pages with ByRange starting after event %d",
		len(found), found[len(found)-1].Sequence)
}

// page returns at most one read's worth of the events a test function accepts.
func (paged *aPagedLog) page(wanted func(event contract.Event) bool) []contract.Event {
	found := []contract.Event{}
	for _, event := range paged.events {
		if !wanted(event) {
			continue
		}
		found = append(found, event)
		if len(found) >= paged.perRead {
			break
		}
	}
	return found
}

// aLongTaskIn writes one file change per number into a log under the same task,
// so that the task has more events than one read of the log returns.
func aLongTaskIn(t *testing.T, paged *aPagedLog, events int) {
	t.Helper()
	for number := 1; number <= events; number++ {
		body, err := json.Marshal(contract.FileChangeBody{
			Path: fmt.Sprintf("/home/jared/nerdgenie/blog/part-%d.md", number), Existed: true,
		})
		if err != nil {
			t.Fatalf("cannot write the file change body: %v", err)
		}
		if _, err := paged.Append(context.Background(), contract.Event{
			TaskID: theLongTask, Kind: contract.EventFileChange, Body: body,
		}); err != nil {
			t.Fatalf("cannot write event number %d: %v", number, err)
		}
	}
}

// theLongTask is the task the paging tests capture.
const theLongTask = "17"

// aMemoryReading builds a memory on a real database that reads the log a test
// hands it.
func aMemoryReading(t *testing.T, paged *aPagedLog) *Memory {
	t.Helper()
	remembering := aMemoryOn(t, aDatabase(t))
	remembering.eventLog = paged
	return remembering
}

func TestCaptureReadsPastTheFirstPageOfALongTask(t *testing.T) {
	ctx := context.Background()
	paged := &aPagedLog{perRead: 4}
	aLongTaskIn(t, paged, 11)
	remembering := aMemoryReading(t, paged)

	if err := remembering.Capture(ctx, theLongTask); err != nil {
		t.Fatalf("cannot capture the long task: %v", err)
	}
	for number := 1; number <= 11; number++ {
		id := capturedFactID(theLongTask, int64(number), false)
		if _, _, err := factRow(ctx, remembering.database, id); err != nil {
			t.Errorf("event number %d of the long task was not captured, and a capture that stops at "+
				"the first page of a read quietly forgets everything the task did after it: %v", number, err)
		}
	}
}

func TestCaptureFailsWhenTheEventLogReallyCannotBeRead(t *testing.T) {
	ctx := context.Background()
	paged := &aPagedLog{perRead: 4, listFailure: errors.New("the log file is no longer there")}
	aLongTaskIn(t, paged, 11)
	remembering := aMemoryReading(t, paged)

	if err := remembering.Capture(ctx, theLongTask); err == nil {
		t.Error("capturing a task whose events could not be read returned no error, and a read that " +
			"failed must never be taken for a task that did nothing")
	}
}

func TestCaptureFailsWhenALaterPageCannotBeRead(t *testing.T) {
	ctx := context.Background()
	paged := &aPagedLog{perRead: 4, rangeFailure: errors.New("the log file is no longer there")}
	aLongTaskIn(t, paged, 11)
	remembering := aMemoryReading(t, paged)

	if err := remembering.Capture(ctx, theLongTask); err == nil {
		t.Error("capturing a task whose later pages could not be read returned no error")
	}
}
