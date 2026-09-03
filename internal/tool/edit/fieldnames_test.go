package edit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// threeLines puts a file of three lines in the folder the tool may change, and
// returns its path and what it holds.
func threeLines(t *testing.T, root string) (string, string) {
	t.Helper()
	held := "the first line\nthe second line\nthe third line\n"
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte(held), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	return path, held
}

func TestAnEditWithNoNewTextIsRefusedAndTheLineIsLeftAlone(t *testing.T) {
	tool, root, _ := newTool(t)
	path, held := threeLines(t, root)

	output, err := run(t, tool, map[string]any{"path": path, "old": "the second line\n"})
	if err == nil {
		t.Fatalf("an edit with no new text deleted the line and said %q", output.Text)
	}
	if !strings.Contains(err.Error(), "new") {
		t.Errorf("the refusal reads %q and does not name the field that is missing", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("cannot read the file back: %v", readErr)
	}
	if string(after) != held {
		t.Errorf("the file now holds %q, and a refused edit must leave it as it was", after)
	}
}

func TestAnEditThatUsesTheNamesTheOtherAgentsUseStillWorks(t *testing.T) {
	pairs := []struct{ old, new string }{
		{"old_string", "new_string"},
		{"old_text", "new_text"},
		{"search", "replace"},
		{"oldString", "newString"},
	}
	for _, pair := range pairs {
		tool, root, _ := newTool(t)
		path, _ := threeLines(t, root)

		_, err := run(t, tool, map[string]any{"path": path, pair.old: "the second line", pair.new: "a better line"})
		if err != nil {
			t.Errorf("an edit written as %s and %s was refused: %v", pair.old, pair.new, err)
			continue
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("cannot read the file back: %v", readErr)
		}
		if !strings.Contains(string(after), "a better line") {
			t.Errorf("an edit written as %s and %s left %q in the file", pair.old, pair.new, after)
		}
	}
}

func TestAnEditThatQuotesNothingToLookForNamesTheFieldItNeeds(t *testing.T) {
	tool, root, _ := newTool(t)
	path, _ := threeLines(t, root)

	_, err := run(t, tool, map[string]any{"path": path, "content": "a better line"})
	if err == nil {
		t.Fatalf("an edit that quotes nothing to look for was made")
	}
	if !strings.HasSuffix(err.Error(), `"old"`) {
		t.Errorf("the refusal reads %q and does not end with the name of the field to write", err)
	}
}

func TestAnEditThatDeletesALineOnPurposeIsStillAllowed(t *testing.T) {
	tool, root, _ := newTool(t)
	path, _ := threeLines(t, root)

	if _, err := run(t, tool, map[string]any{"path": path, "old": "the second line\n", "new": ""}); err != nil {
		t.Fatalf("an edit that replaces a line with nothing was refused: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file back: %v", err)
	}
	if strings.Contains(string(after), "the second line") {
		t.Errorf("the line the edit deleted on purpose is still there: %q", after)
	}
}
