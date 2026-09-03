package sandbox

import (
	"errors"
	"strings"
	"testing"
)

// theKernelsModulesWithLandlock is what /sys/kernel/security/lsm reads like on a
// machine where Landlock is switched on.
const theKernelsModulesWithLandlock = "lockdown,capability,landlock,yama,apparmor,ima,evm"

func TestAvailabilitySaysNothingIsMissingWhenBothPartsAreThere(t *testing.T) {
	if err := checkAvailability(true, theKernelsModulesWithLandlock); err != nil {
		t.Fatalf("the sandbox reported itself unavailable with bwrap installed and Landlock switched on: %v", err)
	}
}

func TestAvailabilityNamesBwrapWhenItIsNotOnThePath(t *testing.T) {
	err := checkAvailability(false, theKernelsModulesWithLandlock)
	if err == nil {
		t.Fatal("the sandbox reported itself available with no bwrap on the PATH")
	}
	if !strings.Contains(err.Error(), "bwrap") || !strings.Contains(err.Error(), "bubblewrap") {
		t.Errorf("the reason says %q, and it must name bwrap and say to install bubblewrap", err)
	}
}

func TestAvailabilityNamesLandlockWhenTheKernelDoesNotReportIt(t *testing.T) {
	err := checkAvailability(true, "lockdown,capability,yama,apparmor")
	if err == nil {
		t.Fatal("the sandbox reported itself available on a kernel with no Landlock")
	}
	if !strings.Contains(err.Error(), "landlock") {
		t.Errorf("the reason says %q, and it must name Landlock", err)
	}
}

func TestAvailabilityNamesBothPartsWhenBothAreMissing(t *testing.T) {
	err := checkAvailability(false, "")
	if err == nil {
		t.Fatal("the sandbox reported itself available with neither part present")
	}
	if !strings.Contains(err.Error(), "bwrap") || !strings.Contains(err.Error(), "landlock") {
		t.Errorf("the reason says %q, and it must name both missing parts", err)
	}
}

func TestAvailabilityIsNotFooledByAModuleNameThatMerelyContainsLandlock(t *testing.T) {
	if err := checkAvailability(true, "capability,notlandlockatall"); err == nil {
		t.Fatal("a module list with no landlock entry of its own was read as Landlock being switched on")
	}
}

func TestTheReasonANamespaceWasRefusedNamesTheFixAPersonHasToApply(t *testing.T) {
	err := namespaceRefusedError("bwrap: setting up uid map: Permission denied", errors.New("bwrap quit with exit status 1"))

	if err == nil {
		t.Fatal("a refused namespace came back as no error at all")
	}
	said := err.Error()
	for _, wanted := range []string{"apparmor", "userns", "bwrap"} {
		if !strings.Contains(strings.ToLower(said), wanted) {
			t.Errorf("the reason says %q, and it must name %q so that a person knows what to change", said, wanted)
		}
	}
	if !strings.Contains(said, "uid map") {
		t.Errorf("the reason says %q, and it must carry what bwrap itself said", said)
	}
}

func TestTheNamespaceProbeIsRunOnceAndItsAnswerRemembered(t *testing.T) {
	fence, _ := aFenceForTesting(t)
	asked := 0
	refused := errors.New("bwrap cannot make a user namespace on this machine")
	fence.probe = func() error {
		asked++
		return refused
	}

	first := fence.canMakeANamespace()
	second := fence.canMakeANamespace()

	// Counting is the whole point. Asking costs a process, the answer does not
	// change while the agent is running, and a test that only compares the two
	// answers stays green when the remembering is taken out.
	if asked != 1 {
		t.Errorf("the fence asked bwrap %d times, want once", asked)
	}
	if !errors.Is(first, refused) || !errors.Is(second, refused) {
		t.Errorf("the answers are %v and %v, want the remembered %v both times", first, second, refused)
	}
}
