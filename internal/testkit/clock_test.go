package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheFakeClockOnlyMovesWhenTheTestMovesIt(t *testing.T) {
	start := time.Date(2026, time.September, 2, 9, 0, 0, 0, time.UTC)
	clock := testkit.NewFakeClock(start)

	if !clock.Now().Equal(start) {
		t.Errorf("the clock reads %s, want %s", clock.Now(), start)
	}
	clock.Advance(90 * time.Minute)
	if want := start.Add(90 * time.Minute); !clock.Now().Equal(want) {
		t.Errorf("after moving it on, the clock reads %s, want %s", clock.Now(), want)
	}
}

func TestASleeperWakesWhenTheTestMovesTheClockPastIt(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	woke := make(chan error, 1)

	go func() { woke <- clock.Sleep(context.Background(), time.Minute) }()
	waitForSleepers(t, clock, 1)

	clock.Advance(time.Minute)
	select {
	case err := <-woke:
		if err != nil {
			t.Errorf("the sleeper woke with an error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the sleeper did not wake after the clock moved past its deadline")
	}
}

func TestASleeperGivesUpWhenItsContextIsCancelled(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	ctx, cancel := context.WithCancel(context.Background())
	woke := make(chan error, 1)

	go func() { woke <- clock.Sleep(ctx, time.Hour) }()
	waitForSleepers(t, clock, 1)
	cancel()

	select {
	case err := <-woke:
		if err == nil {
			t.Error("the sleeper reported no error, want the context's error")
		}
	case <-time.After(time.Second):
		t.Fatal("the sleeper did not give up after its context was cancelled")
	}
}

func TestASleepOfNothingReturnsAtOnce(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())

	if err := clock.Sleep(context.Background(), 0); err != nil {
		t.Errorf("sleeping for no time at all failed: %v", err)
	}
}

func TestATickerFiresOncePerIntervalTheTestMovesPast(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	ticker := clock.NewTicker(time.Minute)
	defer ticker.Stop()

	clock.Advance(3 * time.Minute)

	fired := 0
	for range 3 {
		select {
		case <-ticker.Ticks():
			fired++
		case <-time.After(time.Second):
			t.Fatalf("the ticker fired %d times after three intervals, want 3", fired)
		}
	}
	if fired != 3 {
		t.Errorf("the ticker fired %d times, want 3", fired)
	}
}

func TestAStoppedTickerNeverFiresAgain(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	ticker := clock.NewTicker(time.Minute)
	ticker.Stop()

	clock.Advance(5 * time.Minute)

	select {
	case <-ticker.Ticks():
		t.Error("a stopped ticker fired, and it should never fire again")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTheFakeClockKeepsTheClockContract(t *testing.T) {
	if err := testkit.CheckClock(testkit.NewFakeClock(time.Unix(0, 0).UTC())); err != nil {
		t.Fatalf("the fake clock does not keep the clock contract: %v", err)
	}
}

// waitForSleepers waits until the clock has the number of sleepers the test is
// about to wake, so that the test never races the goroutine it started.
func waitForSleepers(t *testing.T, clock *testkit.FakeClock, wanted int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if clock.Sleepers() >= wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the clock has %d sleepers after a second of waiting, want %d", clock.Sleepers(), wanted)
}
