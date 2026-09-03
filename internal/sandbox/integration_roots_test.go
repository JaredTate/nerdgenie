//go:build integration

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestARootThatIsALinkIsFencedWhereTheLinkLeads(t *testing.T) {
	userHome, work := aTemporaryUserHome(t)
	link := filepath.Join(userHome, "link-to-work")
	if err := os.Symlink(work, link); err != nil {
		t.Fatalf("cannot make the link for the test: %v", err)
	}
	fence := aRealFenceAround(t, userHome, link, theToolOutputCap)

	// bwrap binds the folder a link leads to, and Landlock hangs its rule on the
	// same folder, so the fence has to be built from where the link leads. A
	// fence built from the link itself would let a command write only under a
	// path that is not the one the configuration named.
	plan, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true"})
	if err != nil {
		t.Fatalf("planning a command inside the fence failed: %v", err)
	}
	if !slices.Contains(buildArguments(plan), work) {
		t.Errorf("the command line never names %q, and that is the folder the root leads to", work)
	}

	note := filepath.Join(work, "note.txt")
	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", "echo hello > " + note},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("writing into the folder the root links to reported %d and said %q", result.ExitCode, result.StandardError)
	}
	if written, err := os.ReadFile(note); err != nil || string(written) != "hello\n" {
		t.Errorf("the file the sandboxed command wrote holds %q with the error %v, want %q", written, err, "hello\n")
	}
}

func TestARootThatIsALinkToSomethingThatMustStayOutsideIsRefused(t *testing.T) {
	userHome, work := aTemporaryUserHome(t)
	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this test binary on disk: %v", err)
	}
	targets := map[string]string{
		"the-home-directory": userHome,
		"the-ssh-folder":     filepath.Join(userHome, ".ssh"),
		"the-agents-home":    filepath.Join(userHome, contract.HomeFolderName),
	}

	for what, target := range targets {
		link := filepath.Join(work, "link-to-"+what)
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("cannot make the link for the test: %v", err)
		}
		_, err := New(Settings{Roots: []string{link}, UserHome: userHome, HelperProgram: thisProgram})
		if err == nil {
			t.Errorf("a fence was built around %q, which is a link leading to %q, and that must stay outside the fence", link, target)
		}
	}
}

func TestALinkInsideARootStillCannotReachTheSshFolder(t *testing.T) {
	fence, userHome, work := aRealFence(t, theToolOutputCap)
	escape := filepath.Join(work, "escape")
	if err := os.Symlink(filepath.Join(userHome, ".ssh"), escape); err != nil {
		t.Fatalf("cannot make the link for the test: %v", err)
	}

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/cat",
		Arguments: []string{filepath.Join(escape, "id_fixture")},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed before the fence could refuse it: %v", err)
	}

	if result.ExitCode == 0 {
		t.Errorf("a sandboxed command followed a link out of its root and got %q", result.StandardOutput)
	}
	if strings.Contains(string(result.StandardOutput), "not a key") {
		t.Error("the fixture key came back out of the fence through a link inside the root")
	}
}
