package replay

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// comparableTaskID is the number both records wear while they are compared. A
// replay writes into a throwaway log and so is given a number of its own, which
// says nothing about whether the code still behaves the way it did.
const comparableTaskID = "0"

// comparable is the record with the three things a replay cannot repeat taken
// out of it: the task's number, how much of the budget was left, and what the
// turn cost. The first is an accident of which log the run was written into.
// The other two are measurements of the run itself, and a replay spends
// different tokens than the run it repeats, so holding them against it would
// make every replay fail for a reason nobody can fix.
func comparable(held contract.Record) contract.Record {
	held.Header.ID = comparableTaskID
	held.Header.RoundsLeft = 0
	held.Header.MinutesLeft = 0
	held.Header.Cost = contract.CostLine{}
	return held
}

// doneCheckSays is what the done-check says about a record, and is empty when
// the record may close.
func doneCheckSays(held contract.Record) string {
	if err := record.DoneCheck(comparable(held)); err != nil {
		return err.Error()
	}
	return ""
}

// differenceBetween names the first line where the recorded record and the
// replayed one part company, and is empty when they read the same. A record
// with more lines than the other differs on the line where the shorter one
// stopped, because a line that is not there reads as nothing.
func differenceBetween(was contract.Record, now contract.Record) string {
	recorded := strings.Split(string(record.Print(comparable(was))), "\n")
	replayed := strings.Split(string(record.Print(comparable(now))), "\n")
	for at := range max(len(recorded), len(replayed)) {
		if lineAt(recorded, at) != lineAt(replayed, at) {
			return fmt.Sprintf("line %d of the record: the replay wrote %q and the recording has %q",
				at+1, lineAt(replayed, at), lineAt(recorded, at))
		}
	}
	return ""
}

// lineAt is one line of a printed record, and is empty past the end of it.
func lineAt(lines []string, at int) string {
	if at >= len(lines) {
		return ""
	}
	return lines[at]
}

// writeReport says in plain words what the replay found, which is what the
// replay subcommand prints and what a failing test shows the reader.
func writeReport(result Result) string {
	lines := []string{headline(result)}
	if result.Difference != "" {
		lines = append(lines, "  the first step that differed: "+result.Difference)
	}
	lines = append(lines, "  "+statusLine(result), "  "+doneCheckLine(result),
		fmt.Sprintf("  %d recorded %s played", result.Rounds, roundOrRounds(result.Rounds)))
	return strings.Join(lines, "\n")
}

// headline is the first line of the report, which says whether the replay ended
// where the recording ended.
func headline(result Result) string {
	if result.Passed {
		return fmt.Sprintf("task %s: the replay reproduced the recording", result.TaskID)
	}
	return fmt.Sprintf("task %s: the replay did not reproduce the recording", result.TaskID)
}

// statusLine says where each of the two runs ended.
func statusLine(result Result) string {
	if result.Status == result.WasStatus {
		return fmt.Sprintf("the task ended %s, the same as the recording", result.Status)
	}
	return fmt.Sprintf("the task ended %s and the recording ended %s", result.Status, result.WasStatus)
}

// doneCheckLine says what the done-check made of each of the two records.
func doneCheckLine(result Result) string {
	switch {
	case result.DoneCheck == "" && result.WasDoneCheck == "":
		return "the done-check passes, the same as the recording"
	case result.DoneCheck == result.WasDoneCheck:
		return "the done-check says what it said on the recording: " + result.DoneCheck
	case result.DoneCheck == "":
		return "the done-check passes, and on the recording it said: " + result.WasDoneCheck
	case result.WasDoneCheck == "":
		return "the done-check says " + result.DoneCheck + ", and on the recording it passed"
	default:
		return fmt.Sprintf("the done-check says %s, and on the recording it said %s",
			result.DoneCheck, result.WasDoneCheck)
	}
}

// roundOrRounds is the word for a count of rounds, so that a report about one
// round does not say "1 rounds".
func roundOrRounds(rounds int) string {
	if rounds == 1 {
		return "round"
	}
	return "rounds"
}
