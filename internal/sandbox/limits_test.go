package sandbox

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestTheBoundsOnOneSandboxedCommandAreTheNumbersPinnedHere(t *testing.T) {
	// These numbers are what stands between one line of shell and a wedged
	// machine, so a change to any of them is a change to this test as well.
	for _, bound := range []struct {
		name  string
		found int
		want  int
	}{
		{"how many processes one command may start", MostProcesses, 512},
		{"how large the fence's own temporary folder is", TemporaryFolderBytes, 100 << 20},
	} {
		if bound.found != bound.want {
			t.Errorf("%s is %d, want %d", bound.name, bound.found, bound.want)
		}
	}
}

func TestEveryBoundIsAskedOfTheKernelWithTheSameSoftAndHardLimit(t *testing.T) {
	asked := map[int]syscall.Rlimit{}
	err := setResourceLimits(func(resource int, limit *syscall.Rlimit) error {
		asked[resource] = *limit
		return nil
	})
	if err != nil {
		t.Fatalf("setting the bounds failed: %v", err)
	}

	want := map[int]uint64{processCountResource: MostProcesses}
	for resource, value := range want {
		limit, found := asked[resource]
		if !found {
			t.Errorf("resource %d was never bounded, and a command with no bound on it can take the whole machine down", resource)
			continue
		}
		if limit.Cur != value || limit.Max != value {
			t.Errorf("resource %d was bounded at %d soft and %d hard, want %d for both", resource, limit.Cur, limit.Max, value)
		}
	}
	if len(asked) != len(want) {
		t.Errorf("%d bounds were set, want the %d pinned here", len(asked), len(want))
	}
}

func TestABoundTheKernelRefusesIsReportedWithTheBoundThatFailed(t *testing.T) {
	err := setResourceLimits(func(int, *syscall.Rlimit) error { return errors.New("the kernel said no") })

	if err == nil {
		t.Fatal("a kernel that refused a bound was reported as a success, and the command would run unbounded")
	}
	if !strings.Contains(err.Error(), "how many processes it may start") {
		t.Errorf("the failure says %q, which does not name the bound that could not be set", err)
	}
}

func TestTheCommandLineGivesTheTemporaryFolderASizeItCannotGrowPast(t *testing.T) {
	arguments := buildArguments(theFixturePlan())

	temporaryFolder := slices.Index(arguments, "--tmpfs")
	if temporaryFolder < 2 {
		t.Fatalf("the command line does not make a fresh temporary folder at all: %v", arguments)
	}
	if arguments[temporaryFolder-2] != "--size" {
		t.Fatalf("the command line asks for a temporary folder with no size in front of it, and a tmpfs with no size is half this machine's memory: %v", arguments)
	}
	if arguments[temporaryFolder-1] != strconv.Itoa(TemporaryFolderBytes) {
		t.Errorf("the temporary folder is asked for at %s bytes, want %d", arguments[temporaryFolder-1], TemporaryFolderBytes)
	}
}

// TestTheFenceLeavesAddressSpaceAloneBecauseNodeAndGoReserveIt pins the one
// bound that is deliberately not set: a four-gigabyte address-space bound made
// Node's test runner abort inside the fence on the first live run, because its
// engine reserves far more virtual space than it uses.
func TestTheFenceLeavesAddressSpaceAloneBecauseNodeAndGoReserveIt(t *testing.T) {
	for _, bound := range theBoundsOnOneCommand {
		if bound.resource == syscall.RLIMIT_AS {
			t.Errorf("the fence bounds address space at %d, and Node and Go reserve more than any sane bound allows", bound.value)
		}
	}
}
