package write_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// aSecretOutside writes a file beside the folder the agent may work in, and
// returns its path, so that a test can point a link at it.
func aSecretOutside(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(root), "secrets.txt")
	if err := os.WriteFile(path, []byte("the password is hunter2\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file outside the root: %v", err)
	}
	return path
}

// stillHolds fails the test when the file outside the root was changed.
func stillHolds(t *testing.T, path string, held string) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file outside the root: %v", err)
	}
	if string(after) != held {
		t.Errorf("the file outside the root now holds %q, and nothing may write through the fence", after)
	}
}

func TestAWriteThroughASymbolicLinkOutOfTheRootIsRefused(t *testing.T) {
	tool, root, _ := newTool(t)
	secret := aSecretOutside(t, root)
	link := filepath.Join(root, "notes.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("cannot make the symbolic link inside the root: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"path": link, "content": "written through the fence\n"}); err == nil {
		t.Errorf("a write through a symbolic link out of the root was made")
	}
	stillHolds(t, secret, "the password is hunter2\n")
}

func TestAWriteThroughAHardLinkOutOfTheRootIsRefused(t *testing.T) {
	tool, root, _ := newTool(t)
	secret := aSecretOutside(t, root)
	link := filepath.Join(root, "innocent.txt")
	if err := os.Link(secret, link); err != nil {
		t.Fatalf("cannot make the hard link inside the root: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"path": link, "content": "written through the fence\n"}); err == nil {
		t.Errorf("a write through a hard link out of the root was made")
	}
	stillHolds(t, secret, "the password is hunter2\n")
}
