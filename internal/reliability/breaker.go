// The crash-loop breaker follows the restart-loop guard in Hermes at
// ~/Code/hermes-agent/gateway/restart_loop_guard.py. Its lesson is that the
// state has to be on disk, because every restart is a fresh process and
// anything held in memory is gone; that the breaker must fail open, because a
// broken breaker that wedges a healthy agent is worse than the loop it was
// guarding against; and that tripping means refusing to replay the work that
// keeps killing the program while carrying on serving the user, so that a
// person is put back in the loop instead of a machine spinning.

package reliability

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// BreakerFileName is the small file under the run folder that remembers the
// unclean starts, because a restart is a new process and memory is gone.
const BreakerFileName = "crash-loop.json"

const (
	// RestartLimit is how many unclean starts inside the window count as a crash
	// loop rather than as an operator restarting the program.
	RestartLimit = 3
	// RestartWindow is how close together unclean starts have to be to belong to
	// the same crash loop.
	RestartWindow = 5 * time.Minute
	// QuietPeriod is how long a tripped breaker stays tripped. After it, the
	// breaker clears itself, so that a machine nobody is watching heals.
	QuietPeriod = 30 * time.Minute
)

// Breaker counts the starts that followed an unclean exit and, past the limit,
// says that the agent must serve the user without starting a task.
type Breaker struct {
	path  string
	clock contract.Clock
	guard sync.Mutex
}

// breakerState is what the file under the run folder holds.
type breakerState struct {
	// Starts are the moments of the unclean starts in this crash loop.
	Starts []time.Time `json:"starts"`
	// TrippedAt is when the breaker tripped, and is the zero time when it has
	// not.
	TrippedAt time.Time `json:"trippedAt"`
	// Announced says the user has already been told about this trip, so that
	// they are told once and not on every restart.
	Announced bool `json:"announced"`
}

// NewBreaker returns the breaker for one home folder.
func NewBreaker(home contract.Home, clock contract.Clock) *Breaker {
	return &Breaker{path: filepath.Join(home.RunFolder(), BreakerFileName), clock: clock}
}

// RecordUncleanStart writes down that the program has just started after an
// unclean exit, and says whether that start has tripped the breaker. Starts
// older than the window belong to a crash loop that is over and are dropped.
func (breaker *Breaker) RecordUncleanStart() (bool, error) {
	breaker.guard.Lock()
	defer breaker.guard.Unlock()

	now := breaker.clock.Now()
	state, _ := breaker.read()
	state.Starts = append(within(state.Starts, now, RestartWindow), now)
	if len(state.Starts) >= RestartLimit && state.TrippedAt.IsZero() {
		state.TrippedAt = now
		state.Announced = false
	}
	if err := writeStateFile(breaker.path, state); err != nil {
		return false, err
	}
	return !state.TrippedAt.IsZero(), nil
}

// Tripped says whether the agent must refuse to start a task. A breaker that
// tripped longer ago than the quiet period clears itself here, which is how a
// machine nobody is watching comes back on its own. A file that cannot be read
// leaves the breaker open and reports why, because a broken breaker must never
// stop a healthy agent from working.
func (breaker *Breaker) Tripped() (bool, error) {
	breaker.guard.Lock()
	defer breaker.guard.Unlock()

	state, err := breaker.read()
	if err != nil {
		return false, err
	}
	if state.TrippedAt.IsZero() {
		return false, nil
	}
	if breaker.clock.Now().Sub(state.TrippedAt) >= QuietPeriod {
		return false, removeStateFile(breaker.path)
	}
	return true, nil
}

// Announce returns the one message the user is sent about this trip, and an
// empty string every time after that and whenever the breaker has not tripped.
func (breaker *Breaker) Announce() (string, error) {
	tripped, err := breaker.Tripped()
	if err != nil || !tripped {
		return "", err
	}

	breaker.guard.Lock()
	defer breaker.guard.Unlock()
	state, err := breaker.read()
	if err != nil || state.TrippedAt.IsZero() || state.Announced {
		return "", err
	}
	state.Announced = true
	if err := writeStateFile(breaker.path, state); err != nil {
		return "", err
	}
	return breaker.message(len(state.Starts)), nil
}

// Clear forgets the crash loop, which is what a clean exit and a healthy hour
// both mean.
func (breaker *Breaker) Clear() error {
	breaker.guard.Lock()
	defer breaker.guard.Unlock()
	return removeStateFile(breaker.path)
}

// message is what the user is told, in the words of somebody explaining what
// happened and what they can do about it.
func (breaker *Breaker) message(starts int) string {
	return fmt.Sprintf(
		"Coeus stopped and started again %d times in %s, which means something it was doing kept killing it. "+
			"It will keep answering you, but it will start no task until %s have passed or you restart it yourself. "+
			"The last task is still in the log, so nothing is lost.",
		starts, RestartWindow, QuietPeriod)
}

// read loads the state, and hands back an empty one when the file is missing or
// damaged so that the caller can carry on and write a good one over it.
func (breaker *Breaker) read() (breakerState, error) {
	state := breakerState{}
	found, err := readStateFile(breaker.path, &state)
	if err != nil || !found {
		return breakerState{}, err
	}
	return state, nil
}

// within keeps the moments that are no older than the span.
func within(moments []time.Time, now time.Time, span time.Duration) []time.Time {
	kept := make([]time.Time, 0, len(moments))
	for _, moment := range moments {
		if now.Sub(moment) <= span {
			kept = append(kept, moment)
		}
	}
	return kept
}
