package search_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/search"
)

func TestASearchThatNamesThePatternSomethingElseStillRuns(t *testing.T) {
	names := []string{"query", "regex", "text", "search"}
	for _, name := range names {
		bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
			output, err := run(t, tool, map[string]any{name: "alpha", "path": root})
			if err != nil {
				t.Errorf("a search with the pattern under %q was refused: %v", name, err)
				return
			}
			if !strings.Contains(output.Text, "alpha.go") {
				t.Errorf("a search with the pattern under %q found %q", name, output.Text)
			}
		})
	}
}

func TestASearchWithNothingToLookForNamesTheFieldToWrite(t *testing.T) {
	tool, root := newTool(t, "")

	_, err := run(t, tool, map[string]any{"path": root, "pattern_to_find": "alpha"})
	if err == nil {
		t.Fatalf("a search with nothing to look for was run")
	}
	if !strings.HasSuffix(err.Error(), `"pattern"`) {
		t.Errorf("the refusal reads %q and does not end with the name of the field to write", err)
	}
}
