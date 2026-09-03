package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// aFenceForTesting builds a fence whose one root is a work folder beside the
// agent's own home, with a helper program that is really on disk.
func aFenceForTesting(t *testing.T) (*Fence, string) {
	t.Helper()
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	helper := filepath.Join(userHome, "work", "coeus")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("cannot write the stand-in helper program: %v", err)
	}

	fence, err := New(Settings{Roots: []string{work}, UserHome: userHome, HelperProgram: helper})
	if err != nil {
		t.Fatalf("cannot build a fence for the test: %v", err)
	}
	return fence, work
}

func TestNewFenceKeepsTheCleanedRootsAndFillsInTheDefaults(t *testing.T) {
	fence, work := aFenceForTesting(t)

	if !slices.Equal(fence.roots, []string{work}) {
		t.Errorf("the fence holds the roots %v, want %v", fence.roots, []string{work})
	}
	if fence.outputCap != contract.DefaultConfig().Caps.ToolOutputBytes {
		t.Errorf("the output cap is %d, want the tool output cap of %d", fence.outputCap, contract.DefaultConfig().Caps.ToolOutputBytes)
	}
	if len(fence.systemFolders) == 0 {
		t.Error("the fence found no system folders to bind read-only, and every Linux machine has some")
	}
	for _, folder := range fence.systemFolders {
		if _, err := os.Stat(folder); err != nil {
			t.Errorf("the fence would bind %q, which is not on this machine: %v", folder, err)
		}
	}
}

func TestNewFenceRefusesARootThatMustStayOutside(t *testing.T) {
	userHome := tempUserHome(t)

	_, err := New(Settings{Roots: []string{filepath.Join(userHome, ".ssh")}, UserHome: userHome})
	if err == nil {
		t.Fatal("a fence was built around the SSH folder, and the SSH keys must stay outside")
	}
	if !strings.Contains(err.Error(), ".ssh") {
		t.Errorf("the refusal says %q, and it must name the folder it refused", err)
	}
}

func TestNewFenceRefusesARootThatHoldsTheAgentsHomeWhereCoeusHomeMovedIt(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	agentHome := filepath.Join(work, "agenthome")
	if err := os.MkdirAll(agentHome, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the moved agent home for the test: %v", err)
	}

	_, err := New(Settings{Roots: []string{work}, UserHome: userHome, AgentHome: agentHome, HelperProgram: "/bin/sh"})
	if err == nil {
		t.Fatalf("a fence was built around %q, and COEUS_HOME put the vault and the browser profile inside it", work)
	}
	if !strings.Contains(err.Error(), agentHome) {
		t.Errorf("the refusal says %q, and it must name the folder it would have put inside the fence", err)
	}
}

func TestNewFenceKeepsAnOutputCapTheCallerAsksFor(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")

	fence, err := New(Settings{Roots: []string{work}, UserHome: userHome, OutputCap: 64, HelperProgram: "/bin/sh"})
	if err != nil {
		t.Fatalf("cannot build the fence: %v", err)
	}
	if fence.outputCap != 64 {
		t.Errorf("the output cap is %d, want the 64 the caller asked for", fence.outputCap)
	}
}

func TestNewFenceRefusesAHelperProgramThatIsNotThere(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")

	_, err := New(Settings{Roots: []string{work}, UserHome: userHome, HelperProgram: filepath.Join(work, "no-such-program")})
	if err == nil {
		t.Fatal("a fence was built around a helper program that is not on disk")
	}
}

func TestNewFenceRefusesAHelperProgramThatIsNotAFullPath(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")

	if _, err := New(Settings{Roots: []string{work}, UserHome: userHome, HelperProgram: "coeus"}); err == nil {
		t.Fatal("a fence was built around a helper named without a full path, and the fence has its own PATH")
	}
}

func TestNewFenceFallsBackToThisProgramAsTheHelper(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")

	fence, err := New(Settings{Roots: []string{work}, UserHome: userHome})
	if err != nil {
		t.Fatalf("cannot build the fence: %v", err)
	}
	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this program on disk: %v", err)
	}
	if fence.helperProgram != thisProgram {
		t.Errorf("the helper is %q, want this program at %q", fence.helperProgram, thisProgram)
	}
}
