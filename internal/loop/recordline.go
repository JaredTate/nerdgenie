package loop

import (
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// RecordLineSeparator is what stands between where a record stands and what it
// is about, so that a screen can cut the line in two if it wants to.
const RecordLineSeparator = " · "

// MaxRecordLineRunes is the longest a record line may be. It is one line on a
// screen beside everything else the strip draws, so it is short.
const MaxRecordLineRunes = 90

// RecordLineOf writes the one line a screen draws when a task or a job is
// created, started, finished, or failed: "task 3 started · build the game", or
// "job 2 task 4 done · post the weekly note" when the task belongs to a job.
//
// It is one function so that every place that sends a line writes the same
// shape, and so that a test can ask for the line it expects rather than
// matching on a sentence somebody typed twice.
func RecordLineOf(number string, fromJob *contract.TaskToRun, standing string, about string) string {
	what := "task " + number
	if fromJob != nil {
		what = "job " + fromJob.JobID + " task " + number
	}
	if standing == "" {
		standing = "started"
	}
	line := what + " " + standing + RecordLineSeparator + oneLineOfTheAsk(about)
	return cutToRunes(line, MaxRecordLineRunes)
}

// oneLineOfTheAsk folds what the user asked onto one line, because a record line
// is one line and an ask may be a paragraph.
func oneLineOfTheAsk(about string) string {
	folded := strings.Join(strings.Fields(about), " ")
	if folded == "" {
		return "no ask was written down"
	}
	return folded
}

// cutToRunes shortens a line on a character boundary and says it was cut.
func cutToRunes(line string, limit int) string {
	letters := []rune(line)
	if len(letters) <= limit {
		return line
	}
	return string(letters[:limit-1]) + "\u2026"
}
