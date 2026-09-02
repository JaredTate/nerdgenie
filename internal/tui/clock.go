package tui

import (
	"context"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// systemClock is the machine's own clock. It is the only place in this package
// that reads the real time, and every test puts the fake clock from
// internal/testkit here instead.
type systemClock struct{}

// Now is the time this machine believes it is.
func (systemClock) Now() time.Time {
	return time.Now()
}

// Sleep waits for the duration, or gives up early when the context is cancelled.
func (systemClock) Sleep(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewTicker starts a ticker that fires on the interval until it is stopped.
func (systemClock) NewTicker(interval time.Duration) contract.Ticker {
	if interval <= 0 {
		interval = time.Nanosecond
	}
	return &systemTicker{ticker: time.NewTicker(interval)}
}

// systemTicker is the machine's own ticker, wrapped so that it fits the shape
// internal/contract names.
type systemTicker struct {
	ticker *time.Ticker
}

// Ticks is the channel the times arrive on.
func (ticking *systemTicker) Ticks() <-chan time.Time {
	return ticking.ticker.C
}

// Stop ends the ticker and releases what it holds.
func (ticking *systemTicker) Stop() {
	ticking.ticker.Stop()
}
