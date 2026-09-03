// Stopping and starting the unit follows ZeroClaw's service handling at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/service/mod.rs, which drives a
// user unit through "systemctl --user" and never writes a unit file at update
// time. OpenClaw's updater at ~/Code/openclaw/src/cli/update-cli/ is the example
// avoided here: it edits the service definition during an update, and a
// permission check on the file it does not own is what stopped an update dead.
// Coeus writes the unit once at install and the updater only moves a link.

package update

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// The bounds on talking to the service manager.
const (
	// serviceWait is how long an ordinary systemctl call may take.
	serviceWait = 30 * time.Second
	// maxServiceOutput is how much of what systemctl said is kept for the
	// message, so that a manager gone mad cannot fill memory.
	maxServiceOutput = 4096
)

// service is the systemd user unit the agent runs under, and the four things
// the updater does to it.
type service struct {
	// unit is the name of the unit, which is "coeus.service".
	unit string
	// stopWait is how long the stop may take, because stopping waits for the
	// agent to finish the task it has.
	stopWait time.Duration
}

// stop asks the service manager to stop the agent and waits for it to go. The
// agent has the drain marker by then, so this is the moment its running task
// ends, and the wait is bounded by how long the marker is honoured.
func (unit service) stop(ctx context.Context) error {
	_, err := unit.run(ctx, unit.stopWait, "stop", unit.unit)
	return err
}

// start asks the service manager to start the agent again.
func (unit service) start(ctx context.Context) error {
	_, err := unit.run(ctx, serviceWait, "start", unit.unit)
	return err
}

// active says whether the unit is up and answering.
//
// The unit is a notify unit, and internal/reliability sends READY=1 only when
// the readiness check first answers, so systemd calls the unit active at exactly
// the moment /readyz would. That is why the updater asks the service manager
// rather than opening a port of its own. A manager that says anything else,
// including a refusal, means the unit is not up yet.
func (unit service) active(ctx context.Context) bool {
	said, err := unit.run(ctx, serviceWait, "is-active", unit.unit)
	return err == nil && strings.TrimSpace(said) == "active"
}

// run calls systemctl for the user's own units and hands back what it said.
func (unit service) run(ctx context.Context, wait time.Duration, arguments ...string) (string, error) {
	within, stop := context.WithTimeout(ctx, wait)
	defer stop()

	called := append([]string{"--user"}, arguments...)
	said, err := exec.CommandContext(within, "systemctl", called...).CombinedOutput()
	if len(said) > maxServiceOutput {
		said = said[:maxServiceOutput]
	}
	if err == nil {
		return string(said), nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return string(said), fmt.Errorf("systemctl is not on this machine, so Coeus cannot restart itself; install the service with \"coeus install\" on a machine that runs systemd: %w", err)
	}
	return string(said), fmt.Errorf("\"systemctl %s\" did not work and said %q, so look at \"systemctl --user status %s\": %w",
		strings.Join(called, " "), strings.TrimSpace(string(said)), unit.unit, err)
}
