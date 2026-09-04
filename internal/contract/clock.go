package contract

import (
	"context"
	"time"
)

// Ticker fires on an interval until it is stopped. The fake in internal/testkit
// fires only when a test advances time, so no test ever waits on a real clock.
type Ticker interface {
	// Ticks is the channel the times arrive on.
	Ticks() <-chan time.Time
	// Stop ends the ticker and releases what it holds.
	Stop()
}

// Clock is where every part of Nerd Genie reads the time, so that a test can control
// it. Nothing calls time.Now directly.
type Clock interface {
	// Now is the current time.
	Now() time.Time
	// Sleep waits for the duration, or returns early with the context's error
	// when the context is cancelled first.
	Sleep(ctx context.Context, duration time.Duration) error
	// NewTicker starts a ticker on the interval.
	NewTicker(interval time.Duration) Ticker
}
