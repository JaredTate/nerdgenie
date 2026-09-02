package log

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// lowerTheCap lowers the number of events one read returns for the length of one
// test, so that what a read does at the cap can be proved without writing ten
// thousand rows first.
func lowerTheCap(t *testing.T, howMany int) {
	t.Helper()
	restore := eventsPerReadCap
	t.Cleanup(func() { eventsPerReadCap = restore })
	eventsPerReadCap = howMany
}

// fiveOfOneKind is five events of one task and one kind, so that all three list
// reads find the same five and each can be cut short by the same cap.
var fiveOfOneKind = []contract.Event{
	{TaskID: "t1", Kind: contract.EventMessage},
	{TaskID: "t1", Kind: contract.EventMessage},
	{TaskID: "t1", Kind: contract.EventMessage},
	{TaskID: "t1", Kind: contract.EventMessage},
	{TaskID: "t1", Kind: contract.EventMessage},
}

func TestEveryListReadSaysSoWhenItFillsTheCap(t *testing.T) {
	lowerTheCap(t, 2)
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, fiveOfOneKind)
	ctx := context.Background()

	reads := []struct {
		name string
		run  func() ([]contract.Event, error)
	}{
		{name: "by task", run: func() ([]contract.Event, error) { return eventLog.ByTask(ctx, "t1") }},
		{name: "by kind", run: func() ([]contract.Event, error) { return eventLog.ByKind(ctx, contract.EventMessage) }},
		{name: "by span", run: func() ([]contract.Event, error) {
			return eventLog.ByRange(ctx, contract.EventRange{From: 1, To: 5})
		}},
	}
	for _, one := range reads {
		found, err := one.run()
		if err == nil {
			t.Errorf("reading %s stopped at the cap without saying so, and a caller must never be handed part of an answer quietly", one.name)
		}
		if !sameNumbers(sequencesOf(found), []int64{1, 2}) {
			t.Errorf("reading %s gave back %v, want the first two events the cap allows", one.name, sequencesOf(found))
		}
	}
}

func TestTheErrorAtTheCapSaysHowToReadTheRest(t *testing.T) {
	lowerTheCap(t, 2)
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, fiveOfOneKind)
	ctx := context.Background()

	found, err := eventLog.ByTask(ctx, "t1")
	if err == nil {
		t.Fatal("reading a task with more events than the cap allows returned no error, and it must say it was cut short")
	}
	if !strings.Contains(err.Error(), "ByRange") {
		t.Errorf("the error is %q, and it must say to read the rest in pages with ByRange", err)
	}
	if !strings.Contains(err.Error(), "event 2") {
		t.Errorf("the error is %q, and it must name the last event it handed back", err)
	}

	next, err := eventLog.ByRange(ctx, contract.EventRange{From: found[len(found)-1].Sequence + 1, To: 4})
	if err != nil {
		t.Fatalf("reading the next page failed, and the error must tell a caller how to carry on: %v", err)
	}
	if !sameNumbers(sequencesOf(next), []int64{3, 4}) {
		t.Errorf("the next page holds %v, want events three and four", sequencesOf(next))
	}
}

func TestAReadThatExactlyFillsTheCapDoesNotComplain(t *testing.T) {
	lowerTheCap(t, 5)
	eventLog := newTestLog(t)
	writeEvents(t, eventLog, fiveOfOneKind)

	found, err := eventLog.ByTask(context.Background(), "t1")
	if err != nil {
		t.Fatalf("reading a task whose events exactly fill the cap failed, and nothing was left out: %v", err)
	}
	if len(found) != 5 {
		t.Errorf("the read gave back %d events, want all 5", len(found))
	}
}
