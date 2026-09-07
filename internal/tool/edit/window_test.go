package edit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/edit"
)

// Half of the whole-file reads on runs eighteen and twenty came right after
// an edit of the same file: the result said only how many bytes the file
// held, so the model read the file again to see its own change. The result
// now shows the changed lines with three lines of context each side,
// numbered the way read numbers them, so the next read is not needed.

func anEditOf(t *testing.T, before string, old string, replacement string) string {
	t.Helper()
	tool, root, _ := newTool(t)
	path := filepath.Join(root, "code.js")
	if err := os.WriteFile(path, []byte(before), contract.DataFileMode); err != nil {
		t.Fatal(err)
	}
	output, err := run(t, tool, map[string]any{"path": path, "old": old, "new": replacement})
	if err != nil {
		t.Fatalf("the edit failed: %v", err)
	}
	return output.Text
}

func TestAnEditShowsTheChangedLinesWithContext(t *testing.T) {
	lines := []string{}
	for at := 1; at <= 12; at++ {
		lines = append(lines, "line "+strings.Repeat("x", at))
	}
	before := strings.Join(lines, "\n") + "\n"
	text := anEditOf(t, before, "line xxxxxx\n", "line six\nline six and a half\n")
	want := "lines 3 to 10 now read:\n3: line xxx\n4: line xxxx\n5: line xxxxx\n6: line six\n7: line six and a half\n8: line xxxxxxx\n9: line xxxxxxxx\n10: line xxxxxxxxx\n"
	if !strings.HasSuffix(text, want) {
		t.Errorf("the edit's result reads:\n%s\nwant it to end with:\n%s", text, want)
	}
	if !strings.HasPrefix(text, "edited ") {
		t.Errorf("the result no longer opens with the edited line: %q", strings.SplitN(text, "\n", 2)[0])
	}
}

func TestAnEditAtTheTopOrTheBottomShowsWhatIsThere(t *testing.T) {
	text := anEditOf(t, "a\nb\nc\n", "a\n", "A\n")
	if !strings.HasSuffix(text, "lines 1 to 3 now read:\n1: A\n2: b\n3: c\n") {
		t.Errorf("an edit at the top reads:\n%s", text)
	}
	text = anEditOf(t, "a\nb\nc\n", "c\n", "C\n")
	if !strings.HasSuffix(text, "lines 1 to 3 now read:\n1: a\n2: b\n3: C\n") {
		t.Errorf("an edit at the bottom reads:\n%s", text)
	}
}

func TestADeletionShowsTheLinesAroundTheGap(t *testing.T) {
	text := anEditOf(t, "a\nb\nc\nd\ne\nf\ng\nh\n", "d\ne\n", "")
	if !strings.HasSuffix(text, "lines 1 to 6 now read:\n1: a\n2: b\n3: c\n4: f\n5: g\n6: h\n") {
		t.Errorf("a deletion's result reads:\n%s", text)
	}
}

func TestALongReplacementIsCutAndSaysHowMuchMore(t *testing.T) {
	var many []string
	for at := 0; at < edit.MaxLinesShown+20; at++ {
		many = append(many, "new")
	}
	text := anEditOf(t, "a\nb\nc\n", "b\n", strings.Join(many, "\n")+"\n")
	shown := strings.Count(text, "\n") - strings.Count(strings.SplitN(text, "lines ", 2)[0], "\n")
	if shown > edit.MaxLinesShown+3 || !strings.Contains(text, "more lines; read the file for the rest") {
		t.Errorf("a long replacement's result shows %d lines and reads:\n%s", shown, text[:200])
	}
}
