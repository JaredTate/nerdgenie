package edit

import (
	"fmt"
	"strings"
)

// The result of an edit shows the lines it changed, with a little context,
// numbered the way the read tool numbers them, so the model sees its own
// change without reading the file again. On runs eighteen and twenty half
// of the whole-file reads came right after an edit of the same file.

const (
	// ContextLines is how many unchanged lines are shown on each side.
	ContextLines = 3
	// MaxLinesShown is the most lines the window holds; a longer change is
	// cut and the result says how much more there is.
	MaxLinesShown = 40
)

// changedLines is the window of the file after the edit around what changed:
// the first and last differing lines, found by the lines the two versions
// share at the top and the bottom, widened by the context, bounded.
func changedLines(before string, after string) string {
	old := strings.Split(strings.TrimSuffix(before, "\n"), "\n")
	now := strings.Split(strings.TrimSuffix(after, "\n"), "\n")
	top := 0
	for top < len(old) && top < len(now) && old[top] == now[top] {
		top++
	}
	bottom := 0
	for bottom < len(old)-top && bottom < len(now)-top && old[len(old)-1-bottom] == now[len(now)-1-bottom] {
		bottom++
	}
	// The changed lines of the new text run from top to len(now)-bottom; a
	// pure deletion has none, and the window is the context around the gap.
	first, last := top-ContextLines, len(now)-bottom+ContextLines
	if first < 0 {
		first = 0
	}
	if last > len(now) {
		last = len(now)
	}
	if first >= last {
		return ""
	}
	cut := 0
	if last-first > MaxLinesShown {
		cut = last - first - MaxLinesShown
		last = first + MaxLinesShown
	}
	var out strings.Builder
	fmt.Fprintf(&out, "lines %d to %d now read:\n", first+1, last)
	for at := first; at < last; at++ {
		fmt.Fprintf(&out, "%d: %s\n", at+1, now[at])
	}
	if cut > 0 {
		fmt.Fprintf(&out, "... and %d more lines; read the file for the rest\n", cut)
	}
	return out.String()
}
