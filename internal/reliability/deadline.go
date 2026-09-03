// The one bounded-execution primitive follows the deadline layer in Hermes at
// ~/Code/hermes-agent/agent/deadline.py, which was written after a backlog of
// hangs traced to six site-local timeout mechanisms that each bounded one thing
// and drifted apart. There is one type here, and both the fifteen-minute turn
// limit and the seven-minute tool limit are made from it, so the two limits are
// the same code. The wait is driven by contract.Clock rather than by the
// machine's own timers, so a test moves time instead of waiting for it.

package reliability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// ErrDeadlineExpired is what every deadline in Coeus reports when the work ran
// out of time. It is our own limit and never the model provider's, so a caller
// can tell the two apart.
var ErrDeadlineExpired = errors.New("the work ran out of the time it was given")

// Deadline is one bounded span of work: what the work is called, how long it
// was given, and the moment it runs out.
type Deadline struct {
	clock contract.Clock
	label string
	limit time.Duration
	due   time.Time
}

// NewDeadline gives a span of work a name and a limit, starting now. A limit of
// no time at all has already run out, because a cap of zero is a mistake in the
// configuration rather than a licence to run forever.
func NewDeadline(clock contract.Clock, label string, limit time.Duration) Deadline {
	if limit < 0 {
		limit = 0
	}
	return Deadline{clock: clock, label: label, limit: limit, due: clock.Now().Add(limit)}
}

// TurnDeadline is the limit on one whole turn, which the caps put at fifteen
// minutes.
func TurnDeadline(clock contract.Clock, caps contract.Caps) Deadline {
	return NewDeadline(clock, "the turn", caps.TimePerTurn)
}

// ToolDeadline is the limit on one tool call, which the caps put at seven
// minutes.
func ToolDeadline(clock contract.Clock, caps contract.Caps) Deadline {
	return NewDeadline(clock, "the tool call", caps.TimePerTool)
}

// Due is the moment the work runs out of time.
func (deadline Deadline) Due() time.Time { return deadline.due }

// Remaining is how much time is left, and is zero once the deadline has passed.
func (deadline Deadline) Remaining() time.Duration {
	left := deadline.due.Sub(deadline.clock.Now())
	if left < 0 {
		return 0
	}
	return left
}

// Expired says whether the work has run out of time.
func (deadline Deadline) Expired() bool {
	return !deadline.clock.Now().Before(deadline.due)
}

// Err says why the work must stop, naming the work and the limit it was given,
// or nothing while there is time left.
func (deadline Deadline) Err() error {
	if !deadline.Expired() {
		return nil
	}
	return fmt.Errorf("%s was given %s and has used all of it, so stop and report where it got to: %w",
		deadline.label, deadline.limit, ErrDeadlineExpired)
}

// Watch returns a context that is cancelled when the deadline runs out, and the
// function to stop watching. The cause on the cancelled context is the
// deadline's own error, so a caller can tell our limit from the provider's.
// Stopping waits for the watcher to finish, so a caller that has stopped
// watching leaves no goroutine behind.
func (deadline Deadline) Watch(parent context.Context) (context.Context, context.CancelFunc) {
	bounded, cancel := context.WithCancelCause(parent)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		remaining := deadline.Remaining()
		if remaining <= 0 {
			cancel(deadline.Err())
			return
		}
		if err := deadline.clock.Sleep(bounded, remaining); err != nil {
			return
		}
		cancel(deadline.Err())
	}()

	return bounded, func() {
		cancel(context.Canceled)
		<-finished
	}
}
