package provider

import (
	"context"
	"testing"
	"time"
)

// TestACallGetsNoDeadlineOfItsOwnWhenTheTurnCapIsOff is the user's rule at the
// wire: the shipped caps give a turn no limit, so a call with no deadline from
// its caller gets none here either, rather than a zero one that would end it
// before the request was sent.
func TestACallGetsNoDeadlineOfItsOwnWhenTheTurnCapIsOff(t *testing.T) {
	if limit := callTimeout(); limit != 0 {
		t.Fatalf("the call timeout is %s, and the shipped caps set no time per turn", limit)
	}

	bounded, stop := withCallDeadline(context.Background())
	defer stop()

	if due, has := bounded.Deadline(); has {
		t.Errorf("a call under no turn cap was given a deadline of its own, due %s", due)
	}
	if err := bounded.Err(); err != nil {
		t.Errorf("a call under no turn cap was ended before it began: %v", err)
	}
}

// TestACallKeepsTheDeadlineItsCallerGaveIt proves a deadline the turn brings,
// which is the user's own time_per_turn, is left as it is.
func TestACallKeepsTheDeadlineItsCallerGaveIt(t *testing.T) {
	given, endTheTurn := context.WithTimeout(context.Background(), time.Hour)
	defer endTheTurn()

	bounded, stop := withCallDeadline(given)
	defer stop()

	wanted, _ := given.Deadline()
	if due, has := bounded.Deadline(); !has || !due.Equal(wanted) {
		t.Errorf("the call's deadline is %s (%v), want the caller's own %s", due, has, wanted)
	}
}
