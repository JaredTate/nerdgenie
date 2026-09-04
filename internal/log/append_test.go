package log

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aTime is the moment the tests hand to the log, chosen with nanoseconds in it
// so that a timestamp losing precision on the way to the file would show up.
var aTime = time.Date(2026, time.March, 4, 5, 6, 7, 890123456, time.UTC)

func TestAppendGivesEachEventTheNextSequenceNumber(t *testing.T) {
	eventLog := newTestLog(t)
	ctx := context.Background()

	for wanted := int64(1); wanted <= 3; wanted++ {
		sequence, err := eventLog.Append(ctx, contract.Event{
			Occurred: aTime,
			TaskID:   "t1",
			Kind:     contract.EventMessage,
		})
		if err != nil {
			t.Fatalf("appending event %d failed: %v", wanted, err)
		}
		if sequence != wanted {
			t.Errorf("the sequence number is %d, want %d", sequence, wanted)
		}
	}
}

func TestAppendAndByIDKeepEveryField(t *testing.T) {
	eventLog := newTestLog(t)
	ctx := context.Background()
	written := contract.Event{
		Occurred: aTime,
		TaskID:   "t31",
		Kind:     contract.EventToolResult,
		Body:     json.RawMessage(`{"result":"r7","text":"the page said hello"}`),
	}

	sequence, err := eventLog.Append(ctx, written)
	if err != nil {
		t.Fatalf("appending the event failed: %v", err)
	}
	found, err := eventLog.ByID(ctx, sequence)
	if err != nil {
		t.Fatalf("reading back event %d failed: %v", sequence, err)
	}

	if found.Sequence != sequence {
		t.Errorf("the sequence number came back as %d, want %d", found.Sequence, sequence)
	}
	if !found.Occurred.Equal(written.Occurred) {
		t.Errorf("the time came back as %s, want %s", found.Occurred, written.Occurred)
	}
	if found.Occurred.Location() != time.UTC {
		t.Errorf("the time came back in the zone %s, and the log keeps every time in UTC", found.Occurred.Location())
	}
	if found.TaskID != written.TaskID {
		t.Errorf("the task id came back as %q, want %q", found.TaskID, written.TaskID)
	}
	if found.Kind != written.Kind {
		t.Errorf("the kind came back as %q, want %q", found.Kind, written.Kind)
	}
	if string(found.Body) != string(written.Body) {
		t.Errorf("the body came back as %s, want %s", found.Body, written.Body)
	}
}

func TestAppendTurnsATimeInAnotherZoneIntoUTC(t *testing.T) {
	eventLog := newTestLog(t)
	ctx := context.Background()
	elsewhere := time.FixedZone("five hours behind", -5*60*60)
	written := aTime.In(elsewhere)

	sequence, err := eventLog.Append(ctx, contract.Event{
		Occurred: written,
		TaskID:   "t1",
		Kind:     contract.EventMessage,
	})
	if err != nil {
		t.Fatalf("appending the event failed: %v", err)
	}
	found, err := eventLog.ByID(ctx, sequence)
	if err != nil {
		t.Fatalf("reading the event back failed: %v", err)
	}
	if !found.Occurred.Equal(written) {
		t.Errorf("the time came back as %s, want the same moment as %s", found.Occurred, written)
	}
}

func TestAppendKeepsAnEventThatBelongsToNoTask(t *testing.T) {
	eventLog := newTestLog(t)
	ctx := context.Background()

	sequence, err := eventLog.Append(ctx, contract.Event{Occurred: aTime, Kind: contract.EventMessage})
	if err != nil {
		t.Fatalf("appending an event with no task failed, and the contract allows one: %v", err)
	}
	found, err := eventLog.ByID(ctx, sequence)
	if err != nil {
		t.Fatalf("reading the event back failed: %v", err)
	}
	if found.TaskID != "" {
		t.Errorf("the task id came back as %q, want it empty", found.TaskID)
	}
}

func TestAppendKeepsAnEmptyBodyEmpty(t *testing.T) {
	eventLog := newTestLog(t)
	ctx := context.Background()

	sequence, err := eventLog.Append(ctx, contract.Event{Occurred: aTime, TaskID: "t1", Kind: contract.EventReply})
	if err != nil {
		t.Fatalf("appending an event with no body failed: %v", err)
	}
	found, err := eventLog.ByID(ctx, sequence)
	if err != nil {
		t.Fatalf("reading the event back failed: %v", err)
	}
	if found.Body != nil {
		t.Errorf("the body came back as %s, want nothing at all", found.Body)
	}
}

func TestAppendRefusesAKindTheLogDoesNotKnow(t *testing.T) {
	eventLog := newTestLog(t)

	_, err := eventLog.Append(context.Background(), contract.Event{
		Occurred: aTime,
		TaskID:   "t1",
		Kind:     contract.EventKind("weather report"),
	})
	if err == nil {
		t.Fatal("appending an event of an unknown kind returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), "weather report") {
		t.Errorf("the error is %q, and it must name the kind it was given", err)
	}
}

func TestAppendRefusesABodyThatIsNotJSON(t *testing.T) {
	eventLog := newTestLog(t)

	_, err := eventLog.Append(context.Background(), contract.Event{
		Occurred: aTime,
		TaskID:   "t1",
		Kind:     contract.EventMessage,
		Body:     json.RawMessage("{not json at all"),
	})
	if err == nil {
		t.Fatal("appending a body that is not JSON returned no error, and it must refuse")
	}
}

func TestAppendStopsWhenTheContextIsCancelled(t *testing.T) {
	eventLog := newTestLog(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eventLog.Append(cancelled, contract.Event{Occurred: aTime, TaskID: "t1", Kind: contract.EventMessage})
	if err == nil {
		t.Fatal("appending with a cancelled context returned no error, and it must give up")
	}
}

func TestByIDRefusesASequenceNumberBelowOne(t *testing.T) {
	eventLog := newTestLog(t)

	for _, sequence := range []int64{0, -1} {
		if _, err := eventLog.ByID(context.Background(), sequence); err == nil {
			t.Errorf("reading event %d returned no error, and sequence numbers start at one", sequence)
		}
	}
}

func TestByIDSaysSoWhenThereIsNoSuchEvent(t *testing.T) {
	eventLog := newTestLog(t)

	_, err := eventLog.ByID(context.Background(), 4242)
	if err == nil {
		t.Fatal("reading an event that is not there returned no error, and it must name the number")
	}
	if !strings.Contains(err.Error(), "4242") {
		t.Errorf("the error is %q, and it must name the number 4242", err)
	}
}

func TestAppendAndByIDSaySoAfterTheLogIsClosed(t *testing.T) {
	closed, err := Open(context.Background(), filepath.Join(t.TempDir(), "nerdgenie.db"))
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}

	ctx := context.Background()
	if _, err := closed.Append(ctx, contract.Event{Occurred: aTime, TaskID: "t1", Kind: contract.EventMessage}); err == nil {
		t.Error("appending to a closed log returned no error, and it must say the log is closed")
	}
	if _, err := closed.ByID(ctx, 1); err == nil {
		t.Error("reading from a closed log returned no error, and it must say the log is closed")
	}
}
