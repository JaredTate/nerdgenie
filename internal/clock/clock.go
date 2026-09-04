package clock

import (
	"context"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// system is the machine's own clock.
type system struct{}

// System returns the real clock.
func System() contract.Clock { return system{} }

// Now is the machine's current time.
func (system) Now() time.Time { return time.Now() }

// Sleep waits for the duration, or returns the context's error when the context
// ends first.
func (system) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewTicker starts a real ticker on the interval.
func (system) NewTicker(interval time.Duration) contract.Ticker {
	return ticker{ticker: time.NewTicker(interval)}
}

// ticker wraps the standard library's ticker in the contract's shape.
type ticker struct{ ticker *time.Ticker }

// Ticks is the channel the times arrive on.
func (t ticker) Ticks() <-chan time.Time { return t.ticker.C }

// Stop ends the ticker.
func (t ticker) Stop() { t.ticker.Stop() }
