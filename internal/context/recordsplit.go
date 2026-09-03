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

// splitRecord cuts a printed record into the half that rarely changes and the
// half that changes every turn.
//
// The stable half is the goal and the rules. It goes into the system prompt
// above cache boundary C, where the provider can reuse it for the whole task.
// The live half is the header, the work, and the lessons. The header goes there
// because the budget line and the cost line are written anew every turn, and
// nothing above the cache line may move.
//
// A record that has not been made yet, which is what the first turn of a task
// holds, splits into nothing at all.
func splitRecord(held contract.Record) (stable string, live string) {
	if held.Header.Kind == "" {
		return "", ""
	}
	lines := strings.Split(strings.TrimRight(string(record.Print(held)), "\n"), "\n")
	goalAt, workAt := lineAt(lines, goalHeading), lineAt(lines, workHeading)
	if goalAt < 0 || workAt < goalAt {
		return "", strings.Join(lines, "\n")
	}
	stable = strings.TrimRight(strings.Join(lines[goalAt:workAt], "\n"), "\n")
	live = strings.TrimRight(strings.Join(lines[:goalAt], "\n"), "\n") + "\n\n" +
		strings.Join(lines[workAt:], "\n")
	return stable, live
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
