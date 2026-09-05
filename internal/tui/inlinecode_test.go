package tui

import (
	"strings"
	"testing"
)

// TestInlineCodeKeepsItsPunctuationClose holds the reply's words together:
// the live screen drew "`add`, `remove`" as "add , remove", because a code
// span and the comma after it were split into words and joined with a blank.
// A word that begins where the last one ended, with no blank between them in
// the writer's text, stays joined to it.
func TestInlineCodeKeepsItsPunctuationClose(t *testing.T) {
	rows := markdownRows("- `inventory.py` — `add`, `remove`, `list_items`, backed by `inventory.json`. Over-remove raises `ValueError` and leaves the file.", 200)
	drawn := []string{}
	for _, line := range rows {
		words := ""
		for _, piece := range line.spans {
			words += piece.text
		}
		drawn = append(drawn, words)
	}
	joined := strings.Join(drawn, "\n")
	for _, wanted := range []string{"inventory.py —", "add, remove, list_items, backed", "inventory.json. Over-remove", "ValueError and"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("the reply is drawn as:\n%s\nwant it to read %q", joined, wanted)
		}
	}
	if strings.Contains(joined, " ,") || strings.Contains(joined, " .") {
		t.Errorf("the reply is drawn as:\n%s\nwith a blank before a comma or a full stop", joined)
	}
}
