package context

import (
	"fmt"
	"strings"
)

// RecentTask is one just-finished or set-down task for the model to see when the
// current record is empty, so that "where are we" has an answer better than "no
// active work". It names the task by its number, carries the ask in the user's
// own words, and gives one line on where the task stood. The caller passes these
// newest first.
type RecentTask struct {
	// Number is the task's number, as in "task 4".
	Number int
	// Ask is the ask in the user's own words.
	Ask string
	// Standing is one line on where the task stood when it was last worked.
	Standing string
}

// The bounds on the recent-work block, so it can never grow the prompt without
// limit however many tasks the caller passes or however long an ask or a
// standing line runs. The block rides in the cached part of the prompt, above
// the record, and an unbounded block up there would cost the record its room.
const (
	// MaxRecentTasks is the most recent tasks the block ever names. The caller
	// passes them newest first, and only this many are kept.
	MaxRecentTasks = 3
	// MaxRecentAskRunes bounds the ask on one line, after it is cut to its first
	// sentence.
	MaxRecentAskRunes = 100
	// MaxRecentStandingRunes bounds the standing on one line.
	MaxRecentStandingRunes = 80
)

// recentWorkHeading opens the recent-work block and says what its lines are. It
// rides in the caching part of the prompt because the list holds still through a
// sitting.
const recentWorkHeading = "**Recent work.** The tasks most recently finished or set down, newest first."

// recentWorkText writes the recent-work block: one line per task in the form
// "task N: ask — standing", at most the three most recent, each line bounded in
// length. It is called only when there is at least one task, so it never returns
// the empty string.
func recentWorkText(tasks []RecentTask) string {
	if len(tasks) > MaxRecentTasks {
		tasks = tasks[:MaxRecentTasks]
	}
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		lines = append(lines, fmt.Sprintf("task %d: %s — %s",
			task.Number,
			cutToRunes(firstSentence(oneLine(task.Ask)), MaxRecentAskRunes),
			cutToRunes(oneLine(task.Standing), MaxRecentStandingRunes)))
	}
	return strings.Join(lines, "\n")
}

// firstSentence keeps an ask down to its first sentence, so one line of recent
// work carries a summary and not a whole paragraph. An ask with no sentence end
// is left whole for the rune cap to bound.
func firstSentence(text string) string {
	if at := strings.IndexAny(text, ".!?"); at >= 0 {
		return text[:at+1]
	}
	return text
}

// oneLine flattens any run of whitespace, newlines and all, into single spaces,
// so a multi-line ask or standing becomes one line and cannot draw a line or a
// heading of its own inside the block.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
