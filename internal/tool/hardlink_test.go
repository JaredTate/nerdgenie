package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestAHardLinkInsideARootIsRefusedBecauseItsOtherNameMayBeOutside(t *testing.T) {
	check, root, userHome := workRoot(t)
	outside := filepath.Join(userHome, "secrets.txt")
	if err := os.WriteFile(outside, []byte("the password is hunter2\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file outside the root: %v", err)
	}
	inside := filepath.Join(root, "innocent.txt")
	if err := os.Link(outside, inside); err != nil {
		t.Fatalf("cannot make the hard link inside the root: %v", err)
	}

	allowed, err := check(inside)
	if err == nil {
		t.Fatalf("a hard link inside the root was allowed as %q, and it reaches %s", allowed, outside)
	}
	if !strings.Contains(err.Error(), inside) {
		t.Errorf("the refusal reads %q and does not name the file", err)
	}
	if !strings.Contains(err.Error(), "copy") {
		t.Errorf("the refusal reads %q and does not say what to do instead", err)
	}
}

func TestAnOrdinaryFileWithOneNameIsStillAllowed(t *testing.T) {
	check, root, _ := workRoot(t)
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("a note\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file inside the root: %v", err)
	}

	allowed, err := check(path)
	if err != nil {
		t.Fatalf("an ordinary file inside the root was refused: %v", err)
	}
	if allowed != path {
		t.Errorf("the check returned %q, want the path itself, %q", allowed, path)
	}
}

func TestAFolderInsideARootIsAllowedAlthoughEveryFolderHasSeveralNames(t *testing.T) {
	check, root, _ := workRoot(t)
	folder := filepath.Join(root, "notes")
	if err := os.MkdirAll(filepath.Join(folder, "drafts"), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder inside the root: %v", err)
	}

	if _, err := check(folder); err != nil {
		t.Errorf("a folder inside the root was refused, and a folder always has more than one name: %v", err)
	}
}
