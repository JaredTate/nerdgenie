package clock_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestTheSystemClockReadsTheMachinesTime(t *testing.T) {
	var real contract.Clock = clock.System()
	before := time.Now()
	now := real.Now()
	if now.Before(before) || now.After(before.Add(time.Second)) {
		t.Errorf("the clock said %v, want within a second after %v", now, before)
	}
}

func TestTheSystemClockSleepsAndStopsEarlyWhenTheContextEnds(t *testing.T) {
	real := clock.System()
	start := time.Now()
	if err := real.Sleep(context.Background(), 20*time.Millisecond); err != nil {
		t.Fatalf("a short sleep failed: %v", err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Error("the sleep returned before its duration had passed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start = time.Now()
	if err := real.Sleep(ctx, time.Minute); err == nil {
		t.Error("a sleep on a cancelled context returned no error, want the context's error")
	}
	if time.Since(start) > time.Second {
		t.Error("a sleep on a cancelled context waited instead of returning at once")
	}
}

func TestTheSystemClockTickerTicksAndStops(t *testing.T) {
	ticker := clock.System().NewTicker(5 * time.Millisecond)
	select {
	case <-ticker.Ticks():
	case <-time.After(time.Second):
		t.Fatal("the ticker did not tick within a second")
	}
	ticker.Stop()
}

// TestWordsSayHowLongSomethingTookTheWayAPersonWouldRead holds the one way a
// span of time is written where the harness reports what a task or a job
// took: seconds under a minute, minutes and seconds under an hour, hours and
// minutes after that, and never a negative span.
func TestWordsSayHowLongSomethingTookTheWayAPersonWouldRead(t *testing.T) {
	for _, shape := range []struct {
		span time.Duration
		want string
	}{
		{0, "0s"},
		{42 * time.Second, "42s"},
		{3 * time.Minute, "3m"},
		{3*time.Minute + 12*time.Second, "3m 12s"},
		{59*time.Minute + 59*time.Second, "59m 59s"},
		{time.Hour, "1h"},
		{time.Hour + 5*time.Minute + 30*time.Second, "1h 5m"},
		{26*time.Hour + 1*time.Minute, "26h 1m"},
		{-5 * time.Second, "0s"},
	} {
		if got := clock.Words(shape.span); got != shape.want {
			t.Errorf("Words(%v) reads %q, want %q", shape.span, got, shape.want)
		}
	}
}
