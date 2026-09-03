package reliability_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// startOfTime is the moment every fake clock in these tests starts from.
var startOfTime = time.Date(2026, time.September, 2, 3, 0, 0, 0, time.UTC)

// waitForSleepers waits until that many callers are asleep on the fake clock, so
// that a test never moves the clock past a goroutine which has not started
// waiting yet. It gives up after a second, because a wait with no limit is how a
// test hangs a whole build.
func waitForSleepers(t *testing.T, clock *testkit.FakeClock, wanted int) {
	t.Helper()
	for range 1000 {
		if clock.Sleepers() >= wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("only %d callers are waiting on the clock after a second, want %d", clock.Sleepers(), wanted)
}

func TestTheTurnDeadlineRunsOutAtFifteenMinutes(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.TurnDeadline(clock, contract.DefaultConfig().Caps)

	if deadline.Expired() {
		t.Fatalf("a turn deadline has already run out at the moment it was made")
	}
	if want := 15 * time.Minute; deadline.Remaining() != want {
		t.Errorf("a fresh turn deadline has %s left, want %s", deadline.Remaining(), want)
	}

	clock.Advance(15 * time.Minute)

	if !deadline.Expired() {
		t.Fatalf("the turn deadline has not run out after fifteen minutes")
	}
	err := deadline.Err()
	if !errors.Is(err, reliability.ErrDeadlineExpired) {
		t.Fatalf("a turn that ran out reports %v, want a deadline error", err)
	}
	if !strings.Contains(err.Error(), "turn") || !strings.Contains(err.Error(), "15m") {
		t.Errorf("the deadline error does not name the work and its limit: %v", err)
	}
}

func TestTheToolDeadlineRunsOutAtSevenMinutes(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.ToolDeadline(clock, contract.DefaultConfig().Caps)

	clock.Advance(7*time.Minute - time.Second)
	if deadline.Expired() {
		t.Errorf("the tool deadline ran out a second early")
	}
	if deadline.Err() != nil {
		t.Errorf("a tool deadline with time left reports %v, want nothing", deadline.Err())
	}

	clock.Advance(time.Second)
	if !deadline.Expired() {
		t.Errorf("the tool deadline has not run out after seven minutes")
	}
	if deadline.Remaining() != 0 {
		t.Errorf("a deadline that has run out has %s left, want none", deadline.Remaining())
	}
}

func TestTheDeadlineSaysWhenItIsDue(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.NewDeadline(clock, "the turn", time.Hour)

	if want := startOfTime.Add(time.Hour); !deadline.Due().Equal(want) {
		t.Errorf("the deadline is due at %s, want %s", deadline.Due(), want)
	}
}

func TestWatchingADeadlineCancelsTheContextWhenTheTimeIsUp(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.NewDeadline(clock, "the turn", time.Minute)

	watched, stop := deadline.Watch(context.Background())
	defer stop()
	waitForSleepers(t, clock, 1)

	select {
	case <-watched.Done():
		t.Fatalf("the watched context was cancelled before the deadline")
	default:
	}

	clock.Advance(time.Minute)

	select {
	case <-watched.Done():
	case <-time.After(time.Second):
		t.Fatalf("the watched context was not cancelled when the deadline ran out")
	}
	if err := context.Cause(watched); !errors.Is(err, reliability.ErrDeadlineExpired) {
		t.Errorf("the watched context was cancelled with %v, want a deadline error", err)
	}
}

func TestStoppingAWatchLeavesTheDeadlineAlone(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.NewDeadline(clock, "the tool", time.Minute)

	watched, stop := deadline.Watch(context.Background())
	waitForSleepers(t, clock, 1)
	stop()

	if err := context.Cause(watched); !errors.Is(err, context.Canceled) {
		t.Errorf("a stopped watch left the context with %v, want it cancelled", err)
	}
	if deadline.Expired() {
		t.Errorf("stopping the watch made the deadline itself run out")
	}
}

func TestAWatchEndsWithTheContextItWasGiven(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.NewDeadline(clock, "the turn", time.Hour)
	parent, cancelParent := context.WithCancel(context.Background())

	watched, stop := deadline.Watch(parent)
	defer stop()
	waitForSleepers(t, clock, 1)
	cancelParent()

	select {
	case <-watched.Done():
	case <-time.After(time.Second):
		t.Fatalf("cancelling the parent context did not end the watched one")
	}
}

func TestADeadlineWithNoTimeAtAllHasAlreadyRunOut(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.NewDeadline(clock, "the turn", 0)

	if !deadline.Expired() {
		t.Errorf("a deadline of no time at all has not run out")
	}

	watched, stop := deadline.Watch(context.Background())
	defer stop()
	select {
	case <-watched.Done():
	case <-time.After(time.Second):
		t.Fatalf("watching a deadline that has already run out never cancelled the context")
	}
}
