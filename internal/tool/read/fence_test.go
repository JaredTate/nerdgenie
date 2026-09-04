package read_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
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

func TestASymbolicLinkOutOfTheRootIsRefusedByTheCheckTheProgramShips(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	secret := aSecretOutside(t, root)
	link := filepath.Join(root, "notes.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("cannot make the symbolic link inside the root: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": link})
	if err == nil {
		t.Fatalf("a symbolic link out of the root was read and returned %q", output.Text)
	}
	if strings.Contains(output.Text, "hunter2") {
		t.Errorf("the read handed back what the file outside the root holds")
	}
}

func TestAHardLinkOutOfTheRootIsRefusedByTheCheckTheProgramShips(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	secret := aSecretOutside(t, root)
	link := filepath.Join(root, "innocent.txt")
	if err := os.Link(secret, link); err != nil {
		t.Fatalf("cannot make the hard link inside the root: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": link})
	if err == nil {
		t.Fatalf("a hard link out of the root was read and returned %q", output.Text)
	}
	if strings.Contains(output.Text, "hunter2") {
		t.Errorf("the read handed back what the file outside the root holds")
	}
}
