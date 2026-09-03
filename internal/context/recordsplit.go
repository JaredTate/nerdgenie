package context

import (
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// The two headings the record is cut between. The printer always writes all four
// headings, always in this order, so cutting on them is exact.
const (
	goalHeading = "## Goal"
	workHeading = "## Work"
)

// recordParts is one printed record cut into the pieces that change at different
// paces. A provider's prompt cache keeps whatever the last call and this one
// share from the first byte and stops at the first byte that differs, so what
// changes least goes first and what changes on every call goes last.
type recordParts struct {
	// Stable is the goal and the rules. They change only when the user corrects
	// the work, so they go into the system prompt above cache boundary C.
	Stable string
	// Body is the work and the lessons with the list of results taken out. It
	// changes only when the model edits its plan or its lists, so it is the
	// first message below the cache line.
	Body string
	// Results is the list of results, label line and all. It grows by a line
	// every round, so it goes at the tail, where what grows costs only itself.
	Results string
	// Standing is the header: the status line with the budget left, and the cost
	// of the last call. Those two lines are written anew on every single call,
	// so they are the very last thing the model reads.
	Standing string
}

// splitRecord cuts a printed record into those four pieces.
//
// A record that has not been made yet, which is what the first turn of a task
// holds, splits into nothing at all.
func splitRecord(held contract.Record) recordParts {
	if held.Header.Kind == "" {
		return recordParts{}
	}
	lines := strings.Split(strings.TrimRight(string(record.Print(held)), "\n"), "\n")
	goalAt, workAt := lineAt(lines, goalHeading), lineAt(lines, workHeading)
	if goalAt < 0 || workAt < goalAt {
		return recordParts{Body: strings.Join(lines, "\n")}
	}
	body, results := cutOutTheResults(lines[workAt:])
	return recordParts{
		Stable:   strings.TrimRight(strings.Join(lines[goalAt:workAt], "\n"), "\n"),
		Body:     strings.TrimRight(body, "\n"),
		Results:  results,
		Standing: strings.TrimRight(strings.Join(lines[:goalAt], "\n"), "\n"),
	}
}

// cutOutTheResults takes the list of results out of the work and hands it back
// on its own, label line and all, so that the one part of the record which grows
// every round can go at the tail of the prompt instead of in the middle of it.
// Work with no list yet is handed back whole, with nothing beside it.
func cutOutTheResults(lines []string) (rest string, results string) {
	label := theResultLabel(lines)
	if label < 0 {
		return strings.Join(lines, "\n"), ""
	}
	end := label + 1
	for end < len(lines) && strings.HasPrefix(lines[end], resultItemMark) {
		end++
	}
	if end == label+1 {
		return strings.Join(lines, "\n"), ""
	}
	kept := make([]string, 0, len(lines))
	kept = append(kept, lines[:label]...)
	kept = append(kept, lines[end:]...)
	return strings.Join(kept, "\n"), strings.Join(lines[label:end], "\n")
}

// lineAt is where a heading sits among the lines, or minus one when the printer
// wrote no such line.
func lineAt(lines []string, heading string) int {
	for at, line := range lines {
		if line == heading {
			return at
		}
	}
	return -1
}
