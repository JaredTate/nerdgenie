package search_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool"
	"github.com/JaredTate/coeus/internal/tool/search"
)

// theTree is the small tree of files every test in this file searches.
var theTree = map[string]string{
	"alpha.go":            "package main\n\nfunc alpha() int {\n\treturn 1\n}\n",
	"beta.go":             "package main\n\nfunc beta() int {\n\treturn alpha() + 1\n}\n",
	"notes/alpha.md":      "# Alpha\n\nThe alpha note.\n",
	"notes/unrelated.txt": "nothing to see here\n",
}

// newTool builds the search tool over a tree of fixture files and returns the
// tool and the folder holding them. Ripgrep names the program to search with,
// and a path that is not there makes the tool use its own slow search.
func newTool(t *testing.T, ripgrep string) (*search.Tool, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder the agent may work in: %v", err)
	}
	for name, held := range theTree {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(held), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}
	allowed := tool.NewPathCheck([]string{root}, filepath.Dir(root), "")
	return search.New(search.Settings{Allowed: allowed, Ripgrep: ripgrep}), root
}

// bothWays runs one check with ripgrep and again with the tool's own search, so
// that the two always agree.
func bothWays(t *testing.T, check func(t *testing.T, tool *search.Tool, root string)) {
	t.Helper()
	for _, way := range []struct {
		name    string
		ripgrep string
	}{
		{"through ripgrep", "/usr/bin/rg"},
		{"through its own search", filepath.Join(t.TempDir(), "no-ripgrep-here")},
	} {
		t.Run(way.name, func(t *testing.T) {
			tool, root := newTool(t, way.ripgrep)
			check(t, tool, root)
		})
	}
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *search.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t, "")
	spec := tool.Spec()

	if spec.Name != contract.ToolSearch {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolSearch)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "pattern,path" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a search by pattern and path", names)
	}
}

func TestAPatternFindsTheLinesAndTheFilesItMatches(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": root})
		if err != nil {
			t.Fatalf("searching failed: %v", err)
		}
		testkit.Golden(t, "alpha.txt", []byte(strings.ReplaceAll(output.Text, root, "/root")))
	})
}

func TestAPatternThatMatchesNothingSaysSo(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		output, err := run(t, tool, map[string]any{"pattern": "omega", "path": root})
		if err != nil {
			t.Fatalf("searching failed: %v", err)
		}
		if !strings.Contains(output.Text, "nothing") {
			t.Errorf("a search that found nothing said %q", output.Text)
		}
	})
}

func TestANamePatternThatIsNoRegularExpressionStillFindsFiles(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		output, err := run(t, tool, map[string]any{"pattern": "*.md", "path": root})
		if err != nil {
			t.Fatalf("searching by a name pattern failed: %v", err)
		}
		if !strings.Contains(output.Text, "alpha.md") {
			t.Errorf("the search found %q, want the one file whose name matches", output.Text)
		}
		if !strings.Contains(output.Text, "name pattern") {
			t.Errorf("the search read the pattern as a name pattern without saying so: %q", output.Text)
		}
	})
}

func TestASearchIsCappedAtFiftyRows(t *testing.T) {
	tool, root := newTool(t, "/usr/bin/rg")
	crowded := filepath.Join(root, "crowded")
	if err := os.MkdirAll(crowded, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the crowded folder: %v", err)
	}
	for at := range search.MaxRows + 20 {
		name := filepath.Join(crowded, fmt.Sprintf("file-%03d.txt", at))
		if err := os.WriteFile(name, []byte("a line holding gamma\n"), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}

	output, err := run(t, tool, map[string]any{"pattern": "gamma", "path": crowded})
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	rows := strings.Count(strings.TrimSpace(output.Text), "\n") + 1
	if rows > search.MaxRows+2 {
		t.Errorf("the search returned %d rows, and the cap is %d", rows, search.MaxRows)
	}
	if !strings.Contains(output.Text, "narrow") {
		t.Errorf("the search stopped at the cap without saying what to do: %q", output.Text)
	}
}

func TestASearchOutsideTheRootsIsRefused(t *testing.T) {
	tool, _ := newTool(t, "")

	if _, err := run(t, tool, map[string]any{"pattern": "root", "path": "/etc"}); err == nil {
		t.Errorf("a search outside every root was run")
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, root := newTool(t, "")

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	if _, err := run(t, tool, map[string]any{"path": root}); err == nil {
		t.Errorf("a search with no pattern was run")
	}
	if _, err := run(t, tool, map[string]any{"pattern": strings.Repeat("a", search.MaxPatternRunes+1), "path": root}); err == nil {
		t.Errorf("a pattern longer than the cap was searched for")
	}
}
