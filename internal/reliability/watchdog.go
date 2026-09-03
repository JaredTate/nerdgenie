package reliability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"

	"github.com/JaredTate/coeus/internal/contract"
)

// Watchdog tells the service manager that the program is up and still alive.
// The unit written by "coeus install" is of type notify with a watchdog line, so
// systemd waits for READY=1 before it calls the service started, and starts the
// program again when the feed stops.
type Watchdog struct {
	clock    contract.Clock
	interval time.Duration
	notify   func(state string) (bool, error)
}

// NewWatchdog reads the interval the service manager announced. Outside systemd
// there is no interval and no socket, and everything here does nothing, which is
// what lets the same code run from a terminal.
func NewWatchdog(clock contract.Clock) (*Watchdog, error) {
	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil {
		return nil, fmt.Errorf("the watchdog interval the service manager announced could not be read, so check the WatchdogSec line in the unit: %w", err)
	}
	return &Watchdog{
		clock:    clock,
		interval: interval,
		notify:   func(state string) (bool, error) { return daemon.SdNotify(false, state) },
	}, nil
}

// Interval is how long the service manager waits to hear from Coeus, and is
// zero when there is no watchdog.
func (watchdog *Watchdog) Interval() time.Duration { return watchdog.interval }

// Ready says that the program is up and answering, which is what the readiness
// check reports and what a unit of type notify waits for before it calls the
// service started.
func (watchdog *Watchdog) Ready() error {
	if _, err := watchdog.notify(daemon.SdNotifyReady); err != nil {
		return fmt.Errorf("the service manager could not be told that Coeus is ready, so check the NOTIFY_SOCKET the unit passes: %w", err)
	}
	return nil
}

// Feed tells the service manager that the program is alive, at half the interval
// it announced, until the context ends or the program says it is not healthy.
// Stopping the feed is deliberate: a program whose crash-loop breaker has
// tripped is left to systemd to restart, because nothing inside it is going to
// put it right.
func (watchdog *Watchdog) Feed(ctx context.Context, healthy func() bool) error {
	if healthy == nil {
		return errors.New("the watchdog feed needs a way of asking whether the program is healthy, so pass the function that answers it")
	}
	if watchdog.interval <= 0 {
		return nil
	}

	ticker := watchdog.clock.NewTicker(watchdog.interval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.Ticks():
			if !healthy() {
				return nil
			}
			if _, err := watchdog.notify(daemon.SdNotifyWatchdog); err != nil {
				return fmt.Errorf("the service manager could not be told that Coeus is alive, so it will start the program again: %w", err)
			}
		}
	}
}
