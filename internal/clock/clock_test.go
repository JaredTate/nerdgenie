package clock_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/contract"
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
