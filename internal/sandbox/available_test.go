package sandbox

import (
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
