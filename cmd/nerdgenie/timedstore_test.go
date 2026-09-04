package main

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theStampingTime is the moment the timed store gives an event that arrives with
// none of its own.
var theStampingTime = time.Date(2026, 9, 4, 16, 19, 0, 0, time.UTC)

func TestTheTimedStoreStampsAnEventThatCameWithNoTime(t *testing.T) {
	under := testkit.NewFakeStore()
	timed := timedEvents(under, testkit.NewFakeClock(theStampingTime))
	ctx := context.Background()

	sequence, err := timed.Append(ctx, contract.Event{TaskID: "1", Kind: contract.EventMessage, Body: []byte("{}")})
	if err != nil {
		t.Fatalf("appending an event with no time failed: %v", err)
	}

	got, err := timed.ByID(ctx, sequence)
	if err != nil {
		t.Fatalf("reading the event back by its number failed: %v", err)
	}
	if !got.Occurred.Equal(theStampingTime) {
		t.Errorf("the event was stamped %s, want the moment it was written, %s", got.Occurred, theStampingTime)
	}
}

func TestTheTimedStoreLeavesAnEventThatBroughtItsOwnTimeAlone(t *testing.T) {
	under := testkit.NewFakeStore()
	timed := timedEvents(under, testkit.NewFakeClock(theStampingTime))
	ctx := context.Background()
	brought := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	sequence, err := timed.Append(ctx, contract.Event{TaskID: "1", Kind: contract.EventMessage, Occurred: brought, Body: []byte("{}")})
	if err != nil {
		t.Fatalf("appending an event that brought its own time failed: %v", err)
	}

	got, err := timed.ByID(ctx, sequence)
	if err != nil {
		t.Fatalf("reading the event back failed: %v", err)
	}
	if !got.Occurred.Equal(brought) {
		t.Errorf("the event was restamped %s, want the time it brought, %s", got.Occurred, brought)
	}
}

func TestTheTimedStorePassesTheReadsStraightThrough(t *testing.T) {
	under := testkit.NewFakeStore()
	timed := timedEvents(under, testkit.NewFakeClock(theStampingTime))
	ctx := context.Background()
	for _, kind := range []contract.EventKind{contract.EventMessage, contract.EventToolCall} {
		if _, err := timed.Append(ctx, contract.Event{TaskID: "7", Kind: kind, Body: []byte("{}")}); err != nil {
			t.Fatalf("appending failed: %v", err)
		}
	}

	byTask, err := timed.ByTask(ctx, "7")
	if err != nil || len(byTask) != 2 {
		t.Fatalf("ByTask returned %d events, err %v, want the two under task 7", len(byTask), err)
	}
	byKind, err := timed.ByKind(ctx, contract.EventMessage)
	if err != nil || len(byKind) != 1 {
		t.Fatalf("ByKind returned %d events, err %v, want the one message", len(byKind), err)
	}
	byRange, err := timed.ByRange(ctx, contract.EventRange{From: 1, To: 2})
	if err != nil || len(byRange) != 2 {
		t.Fatalf("ByRange returned %d events, err %v, want the two in the span", len(byRange), err)
	}
	seen := 0
	if err := timed.Replay(ctx, func(contract.Event) error { seen++; return nil }); err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if seen != 2 {
		t.Errorf("Replay handed over %d events, want the two that were written", seen)
	}
}
