package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// securityModulesFile is where the kernel lists the security modules it is
// running. Landlock has to be one of them, or the ruleset the helper builds is
// refused.
const securityModulesFile = "/sys/kernel/security/lsm"

// landlockModuleName is the word this package looks for in that list.
const landlockModuleName = "landlock"

// namespaceProbeTimeout is how long bwrap gets to build an empty fence and quit
// before the sandbox gives up on it. The work is a fraction of a second; ten
// seconds is only there so that a wedged machine cannot hang the agent.
const namespaceProbeTimeout = 10 * time.Second

// Available returns nil when this machine can run a fence, and otherwise an
// error naming what is missing and what to do about it. The shell tool calls
// this and turns itself off when it returns an error.
//
// There are three things to be sure of, and the third can only be found out by
// trying: bwrap on the PATH, Landlock in the kernel, and a bwrap that is allowed
// to make a user namespace. Ubuntu ships with AppArmor refusing that last one to
// an unconfined program, so an installed bwrap is not the same as a working one.
func (fence *Fence) Available() error {
	_, lookupErr := exec.LookPath(bubblewrapProgram)

	// A kernel with no Landlock has no such file, and os.ReadFile hands back
	// nothing along with the error, which reads as a list with no landlock in it
	// and produces the right answer either way.
	modules, _ := os.ReadFile(securityModulesFile)
	if err := checkAvailability(lookupErr == nil, string(modules)); err != nil {
		return err
	}
	return fence.canMakeANamespace()
}

// canMakeANamespace asks the fence's prober to build an empty fence and
// remembers the answer, because the question costs a process to ask and the
// answer does not change while the agent is running.
func (fence *Fence) canMakeANamespace() error {
	fence.probeGuard.Lock()
	defer fence.probeGuard.Unlock()

	if !fence.probed {
		fence.probed = true
		fence.probeReason = fence.probe()
	}
	return fence.probeReason
}

// probeForANamespace runs bwrap once on a command that does nothing at all. It
// is the cheapest fence that still needs every part a real one needs.
func probeForANamespace() error {
	ctx, stopWaiting := context.WithTimeout(context.Background(), namespaceProbeTimeout)
	defer stopWaiting()

	probe := exec.CommandContext(ctx, bubblewrapProgram, "--unshare-user", "--ro-bind", "/", "/", "/bin/true")
	probe.Env = []string{}
	said, err := probe.CombinedOutput()
	if err == nil {
		return nil
	}
	return namespaceRefusedError(string(said), err)
}

// namespaceRefusedError says that bwrap is installed but cannot fence anything,
// and names the one fix that nearly always applies.
func namespaceRefusedError(said string, err error) error {
	return fmt.Errorf("the sandbox cannot run on this machine: %s is installed but cannot make a user namespace, which on "+
		"Ubuntu means AppArmor is refusing it, so as root write a profile for /usr/bin/bwrap that allows userns or set "+
		"kernel.apparmor_restrict_unprivileged_userns to 0; %s said %q: %w",
		bubblewrapProgram, bubblewrapProgram, strings.TrimSpace(said), err)
}

// checkAvailability holds the whole rule, so that a test can ask it about a
// machine it is not running on: bwrap has to be on the PATH and the kernel has
// to say it is running Landlock.
func checkAvailability(bubblewrapFound bool, securityModules string) error {
	missing := []string{}
	if !bubblewrapFound {
		missing = append(missing, "bwrap is not on the PATH, so install the bubblewrap package")
	}
	if !listsLandlock(securityModules) {
		missing = append(missing, "the kernel does not report landlock in "+securityModulesFile+
			", so boot with lsm=landlock among the security modules")
	}
	if len(missing) == 0 {
		return nil
	}
	return errors.New("the sandbox cannot run on this machine: " + strings.Join(missing, ", and "))
}

// listsLandlock says whether the kernel's list of security modules holds
// Landlock. The list is comma-separated, and the whole name has to match, so
// that a module merely spelled with those letters inside it is not mistaken for
// the real one.
func listsLandlock(securityModules string) bool {
	names := strings.Split(strings.TrimSpace(securityModules), ",")
	for index, name := range names {
		names[index] = strings.TrimSpace(name)
	}
	return slices.Contains(names, landlockModuleName)
}
