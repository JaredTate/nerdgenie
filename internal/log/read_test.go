package log

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// writeEvents appends one event per kind and task it is given, and returns the
// sequence numbers in the order they were written.
func writeEvents(t *testing.T, eventLog *Log, written []contract.Event) []int64 {
	t.Helper()
	sequences := []int64{}
	for _, event := range written {
		event.Occurred = aTime
		sequence, err := eventLog.Append(context.Background(), event)
		if err != nil {
			t.Fatalf("appending the event for task %q failed: %v", event.TaskID, err)
		}
		sequences = append(sequences, sequence)
	}
	return sequences
}

// sequencesOf lists the sequence numbers of the events, so that a test can
// compare a whole result in one line.
func sequencesOf(events []contract.Event) []int64 {
	numbers := []int64{}
	for _, event := range events {
		numbers = append(numbers, event.Sequence)
	}
	return numbers
}

// sameNumbers says whether two lists of sequence numbers match, in order.
func sameNumbers(left []int64, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for position, number := range left {
		if right[position] != number {
			return false
		}
	}
	return true
}

// threeTasks is the little log every read-shape test starts from: two events for
// one task with a third task's event between them, so that a read that ignores
// its condition or its order is caught.
var threeTasks = []contract.Event{
	{TaskID: "t1", Kind: contract.EventMessage},
	{TaskID: "t2", Kind: contract.EventToolCall},
	{TaskID: "t1", Kind: contract.EventToolResult},
}

func TestByTaskReturnsOneTasksEventsInOrder(t *testing.T) {
	eventLog := newTestLog(t)
	sequences := writeEvents(t, eventLog, threeTasks)

	found, err := eventLog.ByTask(context.Background(), "t1")
	if err != nil {
		t.Fatalf("reading the events of task t1 failed: %v", err)
	}
	wanted := []int64{sequences[0], sequences[2]}
	if !sameNumbers(sequencesOf(found), wanted) {
		t.Errorf("the events came back as %v, want %v", sequencesOf(found), wanted)
	}
}

func TestByTaskReturnsNothingForATaskWithNoEvents(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, threeTasks)

	found, err := eventLog.ByTask(context.Background(), "t99")
	if err != nil {
		t.Fatalf("reading the events of a task with none failed: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a task with no events gave back %d of them, want none", len(found))
	}
}

func TestByTaskRefusesAnEmptyTaskID(t *testing.T) {
	eventLog := newTestLog(t)

	if _, err := eventLog.ByTask(context.Background(), ""); err == nil {
		t.Error("reading the events of a task with no id returned no error, and it must ask for the id")
	}
}

func TestByKindReturnsOneKindsEventsInOrder(t *testing.T) {
	eventLog := newTestLog(t)
	sequences := writeEvents(t, eventLog, []contract.Event{
		{TaskID: "t1", Kind: contract.EventMessage},
		{TaskID: "t1", Kind: contract.EventToolCall},
		{TaskID: "t2", Kind: contract.EventMessage},
	})

	found, err := eventLog.ByKind(context.Background(), contract.EventMessage)
	if err != nil {
		t.Fatalf("reading the events of one kind failed: %v", err)
	}
	wanted := []int64{sequences[0], sequences[2]}
	if !sameNumbers(sequencesOf(found), wanted) {
		t.Errorf("the events came back as %v, want %v", sequencesOf(found), wanted)
	}
}

func TestByKindReturnsNothingWhenNoEventOfThatKindWasWritten(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, threeTasks)

	found, err := eventLog.ByKind(context.Background(), contract.EventCheckpoint)
	if err != nil {
		t.Fatalf("reading a kind nothing was written for failed: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a kind with no events gave back %d of them, want none", len(found))
	}
}

func TestByKindRefusesAKindTheLogDoesNotKnow(t *testing.T) {
	eventLog := newTestLog(t)

	_, err := eventLog.ByKind(context.Background(), contract.EventKind("weather report"))
	if err == nil {
		t.Fatal("reading a kind the log does not know returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), "weather report") {
		t.Errorf("the error is %q, and it must name the kind it was given", err)
	}
}

func TestByRangeReturnsTheEventsInTheSpan(t *testing.T) {
	eventLog := newTestLog(t)
	sequences := writeEvents(t, eventLog, threeTasks)

	found, err := eventLog.ByRange(context.Background(), contract.EventRange{From: sequences[0], To: sequences[1]})
	if err != nil {
		t.Fatalf("reading a span of sequence numbers failed: %v", err)
	}
	wanted := []int64{sequences[0], sequences[1]}
	if !sameNumbers(sequencesOf(found), wanted) {
		t.Errorf("the events came back as %v, want %v", sequencesOf(found), wanted)
	}
}

func TestByRangeReturnsNothingForASpanPastTheEnd(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, threeTasks)

	found, err := eventLog.ByRange(context.Background(), contract.EventRange{From: 900, To: 1000})
	if err != nil {
		t.Fatalf("reading a span past the end of the log failed: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a span past the end gave back %d events, want none", len(found))
	}
}

func TestByRangeRefusesASpanThatDoesNotStartAtOneOrAbove(t *testing.T) {
	eventLog := newTestLog(t)

	for _, span := range []contract.EventRange{{From: -5, To: 3}, {From: 0, To: 3}} {
		if _, err := eventLog.ByRange(context.Background(), span); err == nil {
			t.Errorf("reading the span %d to %d returned no error, and sequence numbers start at one", span.From, span.To)
		}
	}
}

func TestByRangeRefusesASpanThatEndsBeforeItStarts(t *testing.T) {
	eventLog := newTestLog(t)

	_, err := eventLog.ByRange(context.Background(), contract.EventRange{From: 9, To: 4})
	if err == nil {
		t.Fatal("reading a span that ends before it starts returned no error, and it must refuse")
	}
}

func TestAReadCutShortByItsLimitHandsBackWhatItReadAndSaysSo(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, []contract.Event{
		{TaskID: "t1", Kind: contract.EventMessage},
		{TaskID: "t1", Kind: contract.EventMessage},
		{TaskID: "t1", Kind: contract.EventMessage},
	})

	found, err := eventLog.events(context.Background(), "WHERE task_id = ?", 2, "t1")
	if err == nil {
		t.Fatal("a read cut short by its limit returned no error, and it must say it was cut short")
	}
	if len(found) != 2 {
		t.Errorf("a limit of two gave back %d events, want 2", len(found))
	}
	if !sameNumbers(sequencesOf(found), []int64{1, 2}) {
		t.Errorf("a limit of two gave back %v, want the first two", sequencesOf(found))
	}
}

func TestBoundedLimitNeverGoesPastThePackageMaximum(t *testing.T) {
	cases := []struct {
		asked  int
		wanted int
	}{
		{asked: 1, wanted: 1},
		{asked: 50, wanted: 50},
		{asked: MaxEventsPerRead, wanted: MaxEventsPerRead},
		{asked: MaxEventsPerRead + 1, wanted: MaxEventsPerRead},
		{asked: 0, wanted: MaxEventsPerRead},
		{asked: -7, wanted: MaxEventsPerRead},
	}
	for _, one := range cases {
		if bounded := boundedLimit(one.asked); bounded != one.wanted {
			t.Errorf("a limit of %d became %d, want %d", one.asked, bounded, one.wanted)
		}
	}
}

func TestBoundedLimitNeverAsksForNothing(t *testing.T) {
	lowerTheCap(t, 0)

	for _, asked := range []int{-7, 0, 1, 500} {
		if bounded := boundedLimit(asked); bounded != 1 {
			t.Errorf("with a cap of nothing a limit of %d became %d, want 1, because a read that asks for no rows can hand nothing back to look at", asked, bounded)
		}
	}
}

func TestTheListReadsSaySoAfterTheLogIsClosed(t *testing.T) {
	closed, err := Open(context.Background(), filepath.Join(t.TempDir(), "coeus.db"))
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}

	ctx := context.Background()
	if _, err := closed.ByTask(ctx, "t1"); err == nil {
		t.Error("reading by task from a closed log returned no error, and it must say the log is closed")
	}
	if _, err := closed.ByKind(ctx, contract.EventMessage); err == nil {
		t.Error("reading by kind from a closed log returned no error, and it must say the log is closed")
	}
	if _, err := closed.ByRange(ctx, contract.EventRange{From: 1, To: 3}); err == nil {
		t.Error("reading a span from a closed log returned no error, and it must say the log is closed")
	}
}

func TestAReadStopsWhenTheContextIsCancelled(t *testing.T) {
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, threeTasks)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := eventLog.ByTask(cancelled, "t1"); err == nil {
		t.Error("reading with a cancelled context returned no error, and it must give up")
	}
}
