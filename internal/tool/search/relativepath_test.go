package search_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/search"
)

func TestASearchOfAShortPathLooksUnderTheFolderTheAgentWorksIn(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, _ string) {
		output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": "notes"})
		if err != nil {
			t.Fatalf("the short path every model writes was refused: %v", err)
		}
		if !strings.Contains(output.Text, "alpha.md") {
			t.Errorf("the search came back with %q, want the matches under the folder the agent works in", output.Text)
		}
	})
}

func TestASearchOfAShortPathThatClimbsOutOfTheFolderIsRefused(t *testing.T) {
	tool, root := newTool(t, "")

	_, err := run(t, tool, map[string]any{"pattern": "hunter2", "path": filepath.Join("..", "..")})
	if err == nil {
		t.Fatalf("a short path that climbs out of the folder the agent works in was searched")
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("the refusal reads %q and never says the folder a short path is taken from, which is %s", err, root)
	}
}

func TestASearchOfAPathUnderTheHomeMarkLooksWhereTheMarkPoints(t *testing.T) {
	tool, root := newTool(t, "")
	underTheHome := filepath.Join("~", filepath.Base(root), "notes")

	output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": underTheHome})
	if err != nil {
		t.Fatalf("the path %q, which a model writes whenever it has seen a shell, was refused: %v", underTheHome, err)
	}
	if !strings.Contains(output.Text, "alpha.md") {
		t.Errorf("the search came back with %q, want the matches under the folder the home mark points at", output.Text)
	}
}

func TestTheSearchToolTellsTheModelWhereAShortPathIsTakenFrom(t *testing.T) {
	tool, _ := newTool(t, "")

	said := tool.Spec().Description
	for _, field := range tool.Spec().Fields {
		if field.Name == "path" {
			said += " " + field.Description
		}
	}
	if !strings.Contains(said, "folder the agent works in") {
		t.Errorf("the model is told %q, which never says where a path that does not start at the root is taken from", said)
	}
}
