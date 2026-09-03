package search_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/search"
)

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
