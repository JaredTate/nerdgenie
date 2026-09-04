package search_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/search"
)

// allowedUnder is the check a tool in these tests is given: a path is allowed
// when it is inside the one folder the test works in.
func allowedUnder(root string) func(path string) (string, error) {
	return func(path string) (string, error) {
		if !strings.HasPrefix(filepath.Clean(path), root) {
			return "", fmt.Errorf("the path %s is outside the folder the agent may work in, which is %s", path, root)
		}
		return filepath.Clean(path), nil
	}
}

func TestASearchWithNoFolderLooksInTheFolderTheAgentWorksIn(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		inTheRoot := search.New(search.Settings{Allowed: allowedUnder(root), DefaultFolder: root})

		output, err := inTheRoot.Run(t.Context(), []byte(`{"pattern":"alpha"}`))
		if err != nil {
			t.Fatalf("a search with a pattern and no folder was refused: %v", err)
		}
		if !strings.Contains(output.Text, "alpha.go") {
			t.Errorf("a search with no folder found %q, want the matches under the folder the agent works in", output.Text)
		}
	})
}

func TestASearchWithNoFolderAndNoFolderToFallBackOnSaysSo(t *testing.T) {
	tool, root := newTool(t, "")
	_ = root

	_, err := tool.Run(t.Context(), []byte(`{"pattern":"alpha"}`))
	if err == nil {
		t.Fatalf("a search with nowhere to look was run")
	}
	if !strings.Contains(err.Error(), "folder") {
		t.Errorf("the refusal reads %q and does not say a folder is what is missing", err)
	}
}
