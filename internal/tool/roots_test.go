package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool"
)

// workRoot makes a folder the file tools may work in, under a user home of its
// own, and returns the check over it together with the two paths.
func workRoot(t *testing.T) (tool.PathCheck, string, string) {
	t.Helper()
	userHome := t.TempDir()
	root := filepath.Join(userHome, contract.WorkFolderName)
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the work folder %s: %v", root, err)
	}
	return tool.NewPathCheck([]string{root}, userHome, ""), root, userHome
}

func TestAPathInsideARootIsAllowed(t *testing.T) {
	check, root, _ := workRoot(t)
	wanted := filepath.Join(root, "notes", "today.md")

	allowed, err := check(wanted)
	if err != nil {
		t.Fatalf("a path inside the root was refused: %v", err)
	}
	if allowed != wanted {
		t.Errorf("the check returned %q, want the path itself, %q", allowed, wanted)
	}
}

func TestAPathOutsideEveryRootIsRefusedAndTheRefusalNamesTheRoots(t *testing.T) {
	check, root, userHome := workRoot(t)
	outside := filepath.Join(userHome, "secrets.txt")

	_, err := check(outside)
	if err == nil {
		t.Fatalf("the path %s outside every root was allowed", outside)
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("the refusal reads %q and does not name the root the tools may work in", err)
	}
}

func TestAPathThatClimbsOutOfARootIsRefused(t *testing.T) {
	check, root, _ := workRoot(t)

	if _, err := check(filepath.Join(root, "..", "elsewhere.txt")); err == nil {
		t.Errorf("a path that climbs out of the root with .. was allowed")
	}
}

func TestALinkOutOfARootIsRefused(t *testing.T) {
	check, root, userHome := workRoot(t)
	outside := filepath.Join(userHome, "outside")
	if err := os.MkdirAll(outside, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder outside the root: %v", err)
	}
	link := filepath.Join(root, "shortcut")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("cannot make the link out of the root: %v", err)
	}

	if _, err := check(filepath.Join(link, "secrets.txt")); err == nil {
		t.Errorf("a path that leaves the root through a link was allowed")
	}
}

func TestThePathsTheSandboxMustNeverReachAreRefusedEvenInsideARoot(t *testing.T) {
	userHome := t.TempDir()
	check := tool.NewPathCheck([]string{userHome}, userHome, "")

	for _, forbidden := range contract.ExcludedFromSandbox(userHome, "") {
		if _, err := check(filepath.Join(forbidden, "anything")); err == nil {
			t.Errorf("the path inside %s was allowed, and it must stay outside the fence", forbidden)
		}
	}
}

func TestAPathThatIsNotAWholePathIsRefused(t *testing.T) {
	check, _, _ := workRoot(t)

	for _, written := range []string{"", "notes/today.md", "   "} {
		if _, err := check(written); err == nil {
			t.Errorf("the path %q was allowed, and a tool has no working directory to read it against", written)
		}
	}
}

func TestWithNoRootsConfiguredEveryPathIsRefused(t *testing.T) {
	check := tool.NewPathCheck(nil, t.TempDir(), "")

	_, err := check("/tmp/anything")
	if err == nil {
		t.Fatalf("a path was allowed when no root is configured at all")
	}
	if !strings.Contains(err.Error(), "sandbox_roots") {
		t.Errorf("the refusal reads %q and does not say which setting to fill in", err)
	}
}
