package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// securityModulesFile is where the kernel lists the security modules it is
// running. Landlock has to be one of them, or the ruleset the helper builds is
// refused.
const securityModulesFile = "/sys/kernel/security/lsm"

// landlockModuleName is the word this package looks for in that list.
const landlockModuleName = "landlock"

// Available returns nil when this machine can run a fence, and otherwise an
// error naming what is missing and what to install. The shell tool calls this
// and turns itself off when it returns an error.
func (fence *Fence) Available() error {
	_, lookupErr := exec.LookPath(bubblewrapProgram)

	// A kernel with no Landlock has no such file, and os.ReadFile hands back
	// nothing along with the error, which reads as a list with no landlock in it
	// and produces the right answer either way.
	modules, _ := os.ReadFile(securityModulesFile)
	return checkAvailability(lookupErr == nil, string(modules))
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
