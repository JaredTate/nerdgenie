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

// splitRecord cuts a printed record into three pieces, in the order of how often
// each one changes, because that order is what a provider's prompt cache can
// reuse: it keeps whatever the last call and this one share from the first byte,
// and stops at the first byte that differs.
//
// The stable piece is the goal and the rules. It goes into the system prompt
// above cache boundary C, where the provider can reuse it for the whole task.
// The body is the work and the lessons, which only ever grow as results and
// decisions are added to the end. It goes into the first message below the cache
// line. The standing piece is the header: the status line with the budget left,
// and the cost of the last turn. Those two lines are written anew on every
// single call, so they go last of all, after every result, where changing them
// costs the provider nothing.
//
// A record that has not been made yet, which is what the first turn of a task
// holds, splits into nothing at all.
func splitRecord(held contract.Record) (stable string, body string, standing string) {
	if held.Header.Kind == "" {
		return "", "", ""
	}
	lines := strings.Split(strings.TrimRight(string(record.Print(held)), "\n"), "\n")
	goalAt, workAt := lineAt(lines, goalHeading), lineAt(lines, workHeading)
	if goalAt < 0 || workAt < goalAt {
		return "", strings.Join(lines, "\n"), ""
	}
	stable = strings.TrimRight(strings.Join(lines[goalAt:workAt], "\n"), "\n")
	body = strings.TrimRight(strings.Join(lines[workAt:], "\n"), "\n")
	standing = strings.TrimRight(strings.Join(lines[:goalAt], "\n"), "\n")
	return stable, body, standing
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
