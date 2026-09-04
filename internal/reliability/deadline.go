// The one bounded-execution primitive follows the deadline layer in Hermes at
// ~/Code/hermes-agent/agent/deadline.py, which was written after a backlog of
// hangs traced to six site-local timeout mechanisms that each bounded one thing
// and drifted apart. There is one type here, and both the turn limit, when the
// user sets one, and the seven-minute tool limit are made from it, so the two
// limits are the same code. The wait is driven by contract.Clock rather than by
// the machine's own timers, so a test moves time instead of waiting for it.

package reliability

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// noLimit is what a deadline that is off says it has left: the longest span Go
// can hold, which is more time than any program will see.
const noLimit = time.Duration(math.MaxInt64)

// ErrDeadlineExpired is what every deadline in Nerd Genie reports when the work ran
// out of time. It is our own limit and never the model provider's, so a caller
// can tell the two apart.
var ErrDeadlineExpired = errors.New("the work ran out of the time it was given")

// Deadline is one bounded span of work: what the work is called, how long it
// was given, and the moment it runs out. A deadline may also be off, which is
// a span of work with no limit on it at all.
type Deadline struct {
	clock contract.Clock
	label string
	limit time.Duration
	due   time.Time
	off   bool
}

// NewDeadline gives a span of work a name and a limit, starting now. A limit of
// no time at all has already run out, because a cap of zero on work that must
// end, such as a tool call, is a mistake in the configuration rather than a
// licence to run forever; work that may run with no limit uses NoDeadline.
func NewDeadline(clock contract.Clock, label string, limit time.Duration) Deadline {
	if limit < 0 {
		limit = 0
	}
	return Deadline{clock: clock, label: label, limit: limit, due: clock.Now().Add(limit)}
}

// NoDeadline gives a span of work a name and no limit: it never runs out, and
// watching it only follows the context it was given. It is what a turn gets
// unless the user set time_per_turn.
func NoDeadline(clock contract.Clock, label string) Deadline {
	return Deadline{clock: clock, label: label, off: true}
}

// TurnDeadline is the limit on one whole turn. It is off unless the user set
// time_per_turn, because Nerd Genie puts no cap on its own work unless asked to.
func TurnDeadline(clock contract.Clock, caps contract.Caps) Deadline {
	if caps.TimePerTurn <= 0 {
		return NoDeadline(clock, "the turn")
	}
	return NewDeadline(clock, "the turn", caps.TimePerTurn)
}

// ToolDeadline is the limit on one tool call, which the caps put at seven
// minutes. It is never off: a hung command has to be killed.
func ToolDeadline(clock contract.Clock, caps contract.Caps) Deadline {
	return NewDeadline(clock, "the tool call", caps.TimePerTool)
}

// Off says whether this deadline is off, which is a span of work with no
// limit on it.
func (deadline Deadline) Off() bool { return deadline.off }

// Due is the moment the work runs out of time, and is the zero time on a
// deadline that is off.
func (deadline Deadline) Due() time.Time { return deadline.due }

// Remaining is how much time is left: zero once the deadline has passed, and
// the longest span Go can hold on a deadline that is off.
func (deadline Deadline) Remaining() time.Duration {
	if deadline.off {
		return noLimit
	}
	left := deadline.due.Sub(deadline.clock.Now())
	if left < 0 {
		return 0
	}
	return left
}

// Expired says whether the work has run out of time, which a deadline that is
// off never does.
func (deadline Deadline) Expired() bool {
	return !deadline.off && !deadline.clock.Now().Before(deadline.due)
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
	if deadline.off {
		// There is no moment to wait for, so there is nothing to watch: the
		// context follows its parent and the stop function alone ends it.
		return bounded, func() { cancel(context.Canceled) }
	}
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
