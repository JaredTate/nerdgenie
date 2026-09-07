package read_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/codemap"
	"github.com/JaredTate/nerdgenie/internal/tool/read"
)

// aGeneratedMap is the shape codemap.Print writes, with the mark on it.
const aGeneratedMap = "# Repository Map: Game\n\n" + codemap.GeneratedMark + "\n\nThis file is written by Nerd Genie at the end of every task of a job here.\n\n" +
	"## Roots\n\n- `./` - 1 files\n- `src/` - 2 files\n- `test/` - 1 files\n\n" +
	"## Source files\n\n### src/game.js\nTic-tac-toe: the board, the rules and the rendering.\n- `makeBoard()` → Makes an empty board.\n- `winnerOf(board)`\n\n" +
	"### src/server.js\n- no names read\n\n" +
	"## Tests\n\n### test/game.test.js\nThe game's tests.\n- 12 tests\n\n" +
	"## Other files\n\n```text\nindex.html\n```\n"

// TestAGeneratedMapReadWholeGivesItsContentsAndNotItsEveryFunction: on run
// eighteen the model opened REPO_MAP.md whole at the start of every task,
// four thousand tokens to find one file. A generated map read without a
// section answers with its contents, the roots and one line per file, and
// says how to read one file's functions.
func TestAGeneratedMapReadWholeGivesItsContentsAndNotItsEveryFunction(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	aPage(t, root, "REPO_MAP.md", aGeneratedMap)

	output, err := run(t, tool, map[string]any{"path": "REPO_MAP.md"})
	if err != nil {
		t.Fatalf("reading the map failed: %v", err)
	}
	for _, want := range []string{
		"## Roots",
		"- `src/` - 2 files",
		"- src/game.js — Tic-tac-toe: the board, the rules and the rendering.",
		"- src/server.js — no names read",
		"- test/game.test.js — The game's tests. (12 tests)",
		"- index.html",
		"Read a file's functions with `read REPO_MAP.md <path>`.",
	} {
		if !strings.Contains(output.Text, want) {
			t.Errorf("the contents lack %q:\n%s", want, output.Text)
		}
	}
	for _, absent := range []string{"makeBoard", "winnerOf", "written by Nerd Genie"} {
		if strings.Contains(output.Text, absent) {
			t.Errorf("the contents carry %q, which is one file's entry and not the contents:\n%s", absent, output.Text)
		}
	}
}

// TestAHandWrittenMapIsReadWholeAsAnyFileIs: a map without the generated mark
// is the person's own, and is read as any Markdown file is.
func TestAHandWrittenMapIsReadWholeAsAnyFileIs(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	aPage(t, root, "REPO_MAP.md", "# My map\n\nWritten by hand.\n\n## Roots\n\n- everything is in src/\n")

	output, err := run(t, tool, map[string]any{"path": "REPO_MAP.md"})
	if err != nil {
		t.Fatalf("reading a hand-written map failed: %v", err)
	}
	for _, want := range []string{"1: # My map", "3: Written by hand.", "7: - everything is in src/"} {
		if !strings.Contains(output.Text, want) {
			t.Errorf("the hand-written map is not read whole; it lacks %q:\n%s", want, output.Text)
		}
	}
}

// TestOneFilesEntryIsReadBySectionOrByThePathAfterTheMapsName: the section
// field names one file's entry, and so does the path the instructions show,
// "REPO_MAP.md src/game.js", which a model writes as one path.
func TestOneFilesEntryIsReadBySectionOrByThePathAfterTheMapsName(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	aPage(t, root, "REPO_MAP.md", aGeneratedMap)

	for name, fields := range map[string]map[string]any{
		"by section":              {"path": "REPO_MAP.md", "section": "src/game.js"},
		"by the path after a gap": {"path": "REPO_MAP.md src/game.js"},
	} {
		output, err := run(t, tool, fields)
		if err != nil {
			t.Fatalf("%s: reading one file's entry failed: %v", name, err)
		}
		for _, want := range []string{"### src/game.js", "- `makeBoard()` → Makes an empty board.", "- `winnerOf(board)`"} {
			if !strings.Contains(output.Text, want) {
				t.Errorf("%s: the entry lacks %q:\n%s", name, want, output.Text)
			}
		}
		if strings.Contains(output.Text, "server.js") {
			t.Errorf("%s: the entry carries another file's:\n%s", name, output.Text)
		}
	}
	if _, err := run(t, tool, map[string]any{"path": "REPO_MAP.md src/missing.js"}); err == nil || !strings.Contains(err.Error(), "src/game.js") {
		t.Errorf("a file the map has no entry for is not refused with the entries it has: %v", err)
	}
}

// TestTheContentsOfAHugeMapAreBounded: a map of two thousand files gives a
// listing no longer than one read, and says how many files were left off.
func TestTheContentsOfAHugeMapAreBounded(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	var huge strings.Builder
	huge.WriteString("# Repository Map: Big\n\n" + codemap.GeneratedMark + "\n\n## Roots\n\n- `src/` - 900 files\n\n## Source files\n\n")
	for at := 0; at < 900; at++ {
		fmt.Fprintf(&huge, "### src/file%d.js\nFile %d.\n- `f%d()`\n\n", at, at, at)
	}
	aPage(t, root, "REPO_MAP.md", huge.String())

	output, err := run(t, tool, map[string]any{"path": "REPO_MAP.md"})
	if err != nil {
		t.Fatalf("reading a huge map failed: %v", err)
	}
	if lines := strings.Count(output.Text, "\n"); lines > read.MaxLines+2 {
		t.Errorf("the contents run %d lines, over the cap of %d", lines, read.MaxLines)
	}
	if !strings.Contains(output.Text, "more files") {
		t.Errorf("the contents do not say how many files were left off:\n%s", output.Text[len(output.Text)-300:])
	}
}
