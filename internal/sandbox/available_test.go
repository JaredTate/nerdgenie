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

func TestAvailableOnThisMachineReadsTheRealPathAndTheRealKernel(t *testing.T) {
	fence := &Fence{}

	// The result depends on the machine, so the test asserts only that the
	// answer is one of the two shapes the caller has to handle.
	if err := fence.Available(); err != nil && !strings.Contains(err.Error(), "sandbox") {
		t.Errorf("the reason the sandbox cannot run says %q, and it must say what is missing and what to do", err)
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

	first := fence.canMakeANamespace()
	if !fence.probed {
		t.Fatal("the fence did not remember that it had already asked bwrap")
	}

	// The second answer comes from what was remembered, so it is the same value
	// even though nothing runs the second time.
	if second := fence.canMakeANamespace(); !errors.Is(second, first) && second != first {
		t.Errorf("the second answer is %v and the first was %v, want the one that was remembered", second, first)
	}
}
