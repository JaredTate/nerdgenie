// The drain marker follows the drain control in Hermes at
// ~/Code/hermes-agent/gateway/drain_control.py. There is no control channel
// into a running agent, so the way to tell it to stop taking new work is a file
// it looks at. Two lessons from that file are kept. The marker carries the
// identity of this boot, so that a marker left behind by a machine that has
// since restarted cannot park the agent forever. And the marker expires, so
// that a writer which crashed before cancelling cannot do the same. Both checks
// are lenient: only a definite mismatch or a definite expiry is ignored, and a
// marker nobody can read still means stop, because quiescing by mistake is
// cheap and carrying on by mistake is not.

package reliability

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// DrainFileName is the marker under the run folder that says the loop should
// finish the task it has and take no new one.
const DrainFileName = "drain.json"

// DrainExpiry is how long a marker is honoured. A drain is asked for by
// something that is about to restart the program, so a marker still there half
// an hour later belongs to a writer that never came back.
const DrainExpiry = 30 * time.Minute

// bootIDFile is where Linux publishes an identifier that changes on every boot.
const bootIDFile = "/proc/sys/kernel/random/boot_id"

// plainDrainReason is what a drain is called when the marker gives no reason of
// its own, which is the one thing every drain has in common. It reads as the
// second half of a sentence, as every reason written into the marker does.
const plainDrainReason = "it is being stopped"

// Drain is the marker that tells the loop to finish the running task and take
// no new one, which is how the updater stops the agent without cutting a task
// in half.
type Drain struct {
	path   string
	clock  contract.Clock
	bootID string
}

// drainMarker is what the marker file holds.
type drainMarker struct {
	// BootID is the boot that asked for the drain, so that a restart ends it.
	BootID string `json:"bootID"`
	// RequestedAt is when it was asked for, so that it can run out.
	RequestedAt time.Time `json:"requestedAt"`
	// Reason is for whoever finds the file and wonders why the agent is idle.
	Reason string `json:"reason"`
}

// NewDrain returns the drain marker for one home folder.
func NewDrain(home contract.Home, clock contract.Clock) *Drain {
	return &Drain{path: filepath.Join(home.RunFolder(), DrainFileName), clock: clock, bootID: BootID()}
}

// Request writes the marker, with a reason for whoever finds it.
func (drain *Drain) Request(reason string) error {
	return writeStateFile(drain.path, drainMarker{
		BootID:      drain.bootID,
		RequestedAt: drain.clock.Now(),
		Reason:      reason,
	})
}

// Cancel takes the marker away, and says nothing when there was none.
func (drain *Drain) Cancel() error {
	return removeStateFile(drain.path)
}

// Requested says whether the loop should take no new task. A marker that is
// there but cannot be read counts as a drain, because a marker nobody can read
// has to mean stop.
func (drain *Drain) Requested() bool {
	on, _ := drain.state()
	return on
}

// Why is the reason the drain was asked for, for the caller to put in front of
// whoever asked for a task, and is empty when no drain is on. A marker with no
// reason in it, and a marker nobody can read, both answer with the plain reason
// every drain has, because the caller needs one sentence either way.
func (drain *Drain) Why() string {
	_, why := drain.state()
	return why
}

// state answers both questions from one read of the marker: whether a drain is
// on, and the reason it was asked for.
func (drain *Drain) state() (bool, string) {
	marker := drainMarker{}
	found, err := readStateFile(drain.path, &marker)
	if err != nil {
		if drainFileIsThere(drain.path) {
			return true, plainDrainReason
		}
		return false, ""
	}
	if !found {
		return false, ""
	}
	if marker.BootID != "" && drain.bootID != "" && marker.BootID != drain.bootID {
		return false, ""
	}
	if !marker.RequestedAt.IsZero() && drain.clock.Now().Sub(marker.RequestedAt) > DrainExpiry {
		return false, ""
	}
	if marker.Reason == "" {
		return true, plainDrainReason
	}
	return true, marker.Reason
}

// drainFileIsThere says whether the marker exists at all, which is what decides
// a drain when the file cannot be understood.
func drainFileIsThere(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// BootID is an identifier that changes every time the machine boots, so that a
// marker written before a restart can be told from one written after it. It is
// empty on a machine that does not publish one, and then a marker is honoured
// whatever boot wrote it.
func BootID() string {
	written, err := os.ReadFile(bootIDFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(written))
}
