package testkit

import (
	"context"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// maxTicksPerAdvance caps how many times one ticker fires when a test moves the
// clock a long way, so that advancing by a year cannot fill memory.
const maxTicksPerAdvance = 1000

// FakeClock is a clock that only moves when a test moves it, so that no test
// ever waits on the real one.
type FakeClock struct {
	guard    sync.Mutex
	now      time.Time
	sleepers []*sleeper
	tickers  []*fakeTicker
}

// sleeper is one caller waiting inside Sleep.
type sleeper struct {
	wakeAt time.Time
	woken  chan struct{}
}

// NewFakeClock returns a clock reading the time given.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now returns the time the test has moved the clock to.
func (clock *FakeClock) Now() time.Time {
	clock.guard.Lock()
	defer clock.guard.Unlock()
	return clock.now
}

// Sleep waits until the test moves the clock past the duration, or until the
// context is cancelled, whichever comes first.
func (clock *FakeClock) Sleep(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	waiting := &sleeper{woken: make(chan struct{})}

	clock.guard.Lock()
	waiting.wakeAt = clock.now.Add(duration)
	clock.sleepers = append(clock.sleepers, waiting)
	clock.guard.Unlock()

	select {
	case <-waiting.woken:
		return nil
	case <-ctx.Done():
		clock.forget(waiting)
		return ctx.Err()
	}
}

// NewTicker starts a ticker that fires once for every interval the test moves
// the clock past.
func (clock *FakeClock) NewTicker(interval time.Duration) contract.Ticker {
	if interval <= 0 {
		interval = time.Nanosecond
	}
	clock.guard.Lock()
	defer clock.guard.Unlock()
	ticking := &fakeTicker{
		interval: interval,
		nextAt:   clock.now.Add(interval),
		ticks:    make(chan time.Time, maxTicksPerAdvance),
	}
	clock.tickers = append(clock.tickers, ticking)
	return ticking
}

// Advance moves the clock on, wakes every sleeper whose time has come, and fires
// every ticker once per interval it passed.
func (clock *FakeClock) Advance(duration time.Duration) {
	clock.guard.Lock()
	clock.now = clock.now.Add(duration)
	now := clock.now

	stillWaiting := clock.sleepers[:0]
	for _, waiting := range clock.sleepers {
		if waiting.wakeAt.After(now) {
			stillWaiting = append(stillWaiting, waiting)
			continue
		}
		close(waiting.woken)
	}
	clock.sleepers = stillWaiting

	tickers := make([]*fakeTicker, len(clock.tickers))
	copy(tickers, clock.tickers)
	clock.guard.Unlock()

	for _, ticking := range tickers {
		ticking.fireUpTo(now)
	}
}

// Sleepers is how many callers are waiting inside Sleep, which lets a test wait
// for a goroutine it started before moving the clock.
func (clock *FakeClock) Sleepers() int {
	clock.guard.Lock()
	defer clock.guard.Unlock()
	return len(clock.sleepers)
}

// forget drops a sleeper that gave up because its context was cancelled.
func (clock *FakeClock) forget(gone *sleeper) {
	clock.guard.Lock()
	defer clock.guard.Unlock()
	kept := clock.sleepers[:0]
	for _, waiting := range clock.sleepers {
		if waiting != gone {
			kept = append(kept, waiting)
		}
	}
	clock.sleepers = kept
}

// fakeTicker fires on a fake clock's intervals.
type fakeTicker struct {
	guard    sync.Mutex
	interval time.Duration
	nextAt   time.Time
	ticks    chan time.Time
	stopped  bool
}

// Ticks is the channel the times arrive on.
func (ticking *fakeTicker) Ticks() <-chan time.Time {
	return ticking.ticks
}

// Stop ends the ticker, and a stopped ticker never fires again.
func (ticking *fakeTicker) Stop() {
	ticking.guard.Lock()
	defer ticking.guard.Unlock()
	ticking.stopped = true
}

// fireUpTo sends one time for every interval between the ticker's next moment
// and now, up to the cap.
func (ticking *fakeTicker) fireUpTo(now time.Time) {
	ticking.guard.Lock()
	defer ticking.guard.Unlock()
	if ticking.stopped {
		return
	}
	for range maxTicksPerAdvance {
		if ticking.nextAt.After(now) {
			return
		}
		select {
		case ticking.ticks <- ticking.nextAt:
		default:
			return
		}
		ticking.nextAt = ticking.nextAt.Add(ticking.interval)
	}
}
