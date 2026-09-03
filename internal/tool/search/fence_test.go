package search_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/search"
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

func TestASearchThroughASymbolicLinkOutOfTheRootIsRefused(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		secret := aSecretOutside(t, root)
		link := filepath.Join(root, "elsewhere")
		if err := os.Symlink(filepath.Dir(secret), link); err != nil {
			t.Fatalf("cannot make the symbolic link inside the root: %v", err)
		}

		output, err := run(t, tool, map[string]any{"pattern": "hunter2", "path": link})
		if err == nil {
			t.Fatalf("a search through a symbolic link out of the root was run and returned %q", output.Text)
		}
		if strings.Contains(output.Text, "hunter2") {
			t.Errorf("the search handed back a line from outside the root")
		}
	})
}

func TestASearchThroughAHardLinkOutOfTheRootIsRefused(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		secret := aSecretOutside(t, root)
		link := filepath.Join(root, "innocent.txt")
		if err := os.Link(secret, link); err != nil {
			t.Fatalf("cannot make the hard link inside the root: %v", err)
		}

		output, err := run(t, tool, map[string]any{"pattern": "hunter2", "path": link})
		if err == nil {
			t.Fatalf("a search through a hard link out of the root was run and returned %q", output.Text)
		}
		if strings.Contains(output.Text, "hunter2") {
			t.Errorf("the search handed back a line from outside the root")
		}
	})
}
