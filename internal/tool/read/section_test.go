package read_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/read"
)

const thePage = `# Tater Tots Tetris Architecture

## Engine

The board and the pieces live in src/engine.js.

## Hazards

The states are NORMAL and DRAGON_WARNING.
The config lives in src/config.js.

## Effects

Three of them.
`

func aPage(t *testing.T, root string, name string, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(text), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write %s: %v", name, err)
	}
	return path
}

// TestReadsASectionOfAMarkdownFileByHeading: a path and a heading return that
// section only, with the file's own line numbers, so the model reads two
// hundred words of a page instead of the page.
func TestReadsASectionOfAMarkdownFileByHeading(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := aPage(t, root, "ARCHITECTURE.md", thePage)

	output, err := run(t, tool, map[string]any{"path": path, "section": "hazards"})
	if err != nil {
		t.Fatalf("reading a section failed: %v", err)
	}
	for _, want := range []string{"7: ## Hazards", "9: The states are NORMAL and DRAGON_WARNING.", "10: The config lives in src/config.js."} {
		if !strings.Contains(output.Text, want) {
			t.Errorf("the section does not carry %q:\n%s", want, output.Text)
		}
	}
	for _, absent := range []string{"src/engine.js", "Three of them", "## Effects"} {
		if strings.Contains(output.Text, absent) {
			t.Errorf("the section carries %q from another section:\n%s", absent, output.Text)
		}
	}
}

// TestAnUnknownHeadingListsTheHeadings: a heading the file does not have is
// refused with the headings it does have, so the next call is right.
func TestAnUnknownHeadingListsTheHeadings(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := aPage(t, root, "ARCHITECTURE.md", thePage)

	_, err := run(t, tool, map[string]any{"path": path, "section": "Rendering"})
	if err == nil {
		t.Fatalf("a heading the file does not have was read")
	}
	for _, want := range []string{"Rendering", "Engine", "Hazards", "Effects"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// TestASectionIsBoundedLikeAFile: a section longer than one read holds is cut
// where a file would be, with the same line saying how to read on.
func TestASectionIsBoundedLikeAFile(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	var long strings.Builder
	long.WriteString("# Big\n\n## Long\n\n")
	for at := 1; at <= read.MaxLines+50; at++ {
		fmt.Fprintf(&long, "line %d of the long section\n", at)
	}
	long.WriteString("\n## After\n\nshort\n")
	path := aPage(t, root, "BIG.md", long.String())

	output, err := run(t, tool, map[string]any{"path": path, "section": "Long"})
	if err != nil {
		t.Fatalf("reading a long section failed: %v", err)
	}
	if !strings.Contains(output.Text, "... there are more lines; read on with an offset of") {
		t.Errorf("a section over the line cap does not say how to read on:\n%s", output.Text[len(output.Text)-200:])
	}
	if strings.Contains(output.Text, "## After") {
		t.Errorf("the read ran past the section's end")
	}
}

// TestASectionOfAFileThatIsNotMarkdownIsRefused: a section makes sense of a
// Markdown file only, and the refusal says to read the file plain.
func TestASectionOfAFileThatIsNotMarkdownIsRefused(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := aPage(t, root, "engine.js", "// # not a heading\nexport const x = 1;\n")

	_, err := run(t, tool, map[string]any{"path": path, "section": "not a heading"})
	if err == nil || !strings.Contains(err.Error(), "without a section") {
		t.Fatalf("a section of a JavaScript file was not refused with a way out: %v", err)
	}
}

// TestTheDescriptionSaysASectionCanBeRead pins the one sentence the model
// reads about sections.
func TestTheDescriptionSaysASectionCanBeRead(t *testing.T) {
	tool, _ := newTool(t, nil, nil)
	if !strings.Contains(tool.Spec().Description, "A Markdown file and a section heading return that section only.") {
		t.Errorf("the description does not say a section can be read: %q", tool.Spec().Description)
	}
	found := false
	for _, field := range tool.Spec().Fields {
		if field.Name == "section" {
			found = true
		}
	}
	if !found {
		t.Errorf("the specification has no section field")
	}
}

func FuzzSectionRequest(f *testing.F) {
	f.Add("Hazards", thePage)
	f.Add("", "")
	f.Add("## Engine", "# a\n## Engine\ntext\n")
	f.Fuzz(func(t *testing.T, section string, page string) {
		tool, root := newTool(t, nil, nil)
		path := aPage(t, root, "PAGE.md", page)
		written, err := json.Marshal(map[string]any{"path": path, "section": section})
		if err != nil {
			t.Fatalf("cannot write the input: %v", err)
		}
		output, err := tool.Run(t.Context(), written)
		if err == nil && len(output.Text) > read.MaxBytes+400 {
			t.Errorf("a section read returned %d bytes, over the cap", len(output.Text))
		}
	})
}
