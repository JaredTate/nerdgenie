// The turn lease follows the per-session lease in Hermes at
// ~/Code/hermes-agent/gateway/turn_lease.py. Two of its lessons are kept here.
// A release is checked against the exact lease that was handed out, so that a
// late unwind cannot free a newer turn's lease. And a waiter that runs out of
// time is refused rather than let through, because a turn that runs beside the
// one holding the lease writes the same record twice, which is the damage the
// lease exists to prevent.

package reliability

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// LeaseWait is how long a second turn on the same session waits for the turn in
// front of it before it is refused.
const LeaseWait = 5 * time.Second

// leaseStep is how often a waiting turn looks again. It is short enough that a
// turn which finishes quickly is not made to wait, and long enough that the
// waiting costs nothing.
const leaseStep = 25 * time.Millisecond

// MaxLeases is how many sessions may hold a turn at once. It is a bound rather
// than a policy: a machine with this many turns running at once is in trouble
// already, and refusing the next one says so.
const MaxLeases = 512

// ErrTurnInProgress means a turn is already running for that session and the
// wait for it ran out. The caller must refuse the new turn and tell the user
// rather than run it beside the one that holds the lease.
var ErrTurnInProgress = errors.New("a turn is already running for this session")

// Leases hands out one turn at a time for each session.
type Leases struct {
	clock contract.Clock
	guard sync.Mutex
	held  map[string]*Lease
}

// Lease is one held turn. Release gives it back, and only this exact lease can.
type Lease struct {
	leases  *Leases
	session string
}

// NewLeases returns an empty registry that reads the time from the clock given.
func NewLeases(clock contract.Clock) *Leases {
	return &Leases{clock: clock, held: map[string]*Lease{}}
}

// Acquire takes the turn lease for a session, waiting up to LeaseWait for a
// turn that is already running. It returns ErrTurnInProgress when the wait runs
// out, and the context's own error when the caller gives up first.
func (leases *Leases) Acquire(ctx context.Context, session string) (*Lease, error) {
	if session == "" {
		return nil, errors.New("a turn lease needs the name of the session it is for, so pass the session the message arrived on")
	}
	startedWaiting := leases.clock.Now()

	// The loop is bounded twice over: by the wait below and by the number of
	// looks it can take inside that wait, so that no clock can make it spin.
	for range int(LeaseWait/leaseStep) + 2 {
		lease, err := leases.take(session)
		if err != nil {
			return nil, err
		}
		if lease != nil {
			return lease, nil
		}
		waited := leases.clock.Now().Sub(startedWaiting)
		if waited >= LeaseWait {
			break
		}
		if err := leases.clock.Sleep(ctx, min(leaseStep, LeaseWait-waited)); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w on %q, so it waited %s and gave up: send the message again in a moment",
		ErrTurnInProgress, session, LeaseWait)
}

// Held is how many sessions are running a turn right now.
func (leases *Leases) Held() int {
	leases.guard.Lock()
	defer leases.guard.Unlock()
	return len(leases.held)
}

// Release gives the lease back, and does nothing when it has already been given
// back or when a newer turn now holds the session.
func (lease *Lease) Release() {
	if lease == nil || lease.leases == nil {
		return
	}
	leases := lease.leases
	leases.guard.Lock()
	defer leases.guard.Unlock()
	if leases.held[lease.session] == lease {
		delete(leases.held, lease.session)
	}
}

// Session is the session the lease is held for.
func (lease *Lease) Session() string { return lease.session }

// take gives out the lease when the session is free, nothing when it is held,
// and an error when the registry is full.
func (leases *Leases) take(session string) (*Lease, error) {
	leases.guard.Lock()
	defer leases.guard.Unlock()

	if _, taken := leases.held[session]; taken {
		return nil, nil
	}
	if len(leases.held) >= MaxLeases {
		return nil, fmt.Errorf("%d turns are already running, which is all Coeus runs at once, so this turn was not started: wait for one to finish",
			len(leases.held))
	}
	lease := &Lease{leases: leases, session: session}
	leases.held[session] = lease
	return lease, nil
}
