package context

import (
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The two headings the record is cut between. The printer always writes all four
// headings, always in this order, so cutting on them is exact.
const (
	goalHeading = "## Goal"
	workHeading = "## Work"
	// doneListLabel opens the done list inside the goal. The list is written
	// and marked as the work goes, so it is cut out of the goal and rides in
	// the tail with the plan.
	doneListLabel = "Done when:"
)

// recordParts is one printed record cut into the pieces that change at different
// paces. A provider's prompt cache keeps whatever the last call and this one
// share from the first byte and stops at the first byte that differs, so what
// changes least goes first and what changes on every call goes last.
type recordParts struct {
	// Stable is the goal without its done list, and the rules. They change
	// only when the user corrects the work, so they go into the system prompt
	// above cache boundary C. The done list is not among them: it is written
	// and then marked line by line as the work is proved, and every one of
	// those writes used to rewrite the system prompt and cost the whole
	// conversation under it, a hundred and thirty-five seconds on the local
	// model at sixty thousand tokens.
	Stable string
	// Body is the done list, then the work and the lessons with the list of
	// results taken out. It changes only when the model edits its plan or its
	// lists or proves a done line, so it is the first message below the cache
	// line.
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
// It prints through record.PrintForTheModel rather than record.Print, so an ask
// too long to ride in every prompt arrives as its first quarter and one line
// saying how to read the rest. The stored record keeps the user's words entire.
//
// A record that has not been made yet, which is what the first turn of a task
// holds, splits into nothing at all.
func splitRecord(held contract.Record) recordParts {
	if held.Header.Kind == "" {
		return recordParts{}
	}
	lines := strings.Split(strings.TrimRight(string(record.PrintForTheModel(held)), "\n"), "\n")
	goalAt, workAt := lineAt(lines, goalHeading), lineAt(lines, workHeading)
	if goalAt < 0 || workAt < goalAt {
		return recordParts{Body: strings.Join(lines, "\n")}
	}
	body, results := cutOutTheResults(lines[workAt:])
	stable, doneList := cutOutTheDoneList(lines[goalAt:workAt])
	if doneList != "" {
		body = doneList + "\n\n" + body
	}
	return recordParts{
		Stable:   strings.TrimRight(strings.Join(stable, "\n"), "\n"),
		Body:     strings.TrimRight(body, "\n"),
		Results:  results,
		Standing: strings.TrimRight(strings.Join(lines[:goalAt], "\n"), "\n"),
	}
}

// cutOutTheDoneList takes the done list out of the goal's lines and hands it
// back on its own, label line and all, so that the one part of the goal which
// changes as the work is proved can ride in the tail. A goal with no done list
// yet is handed back whole, with nothing beside it.
func cutOutTheDoneList(goal []string) (rest []string, doneList string) {
	label := lineAt(goal, doneListLabel)
	if label < 0 {
		return goal, ""
	}
	end := label + 1
	for end < len(goal) && strings.HasPrefix(goal[end], resultItemMark) {
		end++
	}
	kept := make([]string, 0, len(goal))
	kept = append(kept, goal[:label]...)
	kept = append(kept, goal[end:]...)
	return kept, strings.Join(goal[label:end], "\n")
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
