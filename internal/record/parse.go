package record

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The two caps on the text a record may be read from. A record is one to three
// thousand tokens by design, so anything past these is not a record Coeus wrote.
const (
	// MaxRecordBytes is the most text Parse will look at.
	MaxRecordBytes = 1 << 20
	// MaxRecordLines is the most lines Parse will look at.
	MaxRecordLines = 20000
)

// headingOrder is the order the four parts are always written in, because
// everything above the cache line must not move between one turn and the next.
var headingOrder = []string{headingGoal, headingRules, headingWork, headingLessons}

// Parse reads the text form of a record back into the contract types. It is the
// only reader of that form, as Print is the only writer, and it refuses anything
// the rules of a record forbid rather than quietly repairing it.
func Parse(text []byte) (contract.Record, error) {
	if len(text) > MaxRecordBytes {
		return contract.Record{}, fmt.Errorf("this text is %d bytes and a record is at most %d, so it is not a record Coeus wrote",
			len(text), MaxRecordBytes)
	}
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	if len(lines) > MaxRecordLines {
		return contract.Record{}, fmt.Errorf("this text is %d lines and a record is at most %d, so it is not a record Coeus wrote",
			len(lines), MaxRecordLines)
	}

	reading := &reader{}
	for index, line := range lines {
		reading.line = index + 1
		if err := reading.readLine(line); err != nil {
			return contract.Record{}, err
		}
	}
	if reading.headings != len(headingOrder) {
		return contract.Record{}, fmt.Errorf("this record has %d of its four parts, so check that it runs from %q to %q",
			reading.headings, headingGoal, headingLessons)
	}
	if reading.record.Goal.Ask == "" {
		return contract.Record{}, fmt.Errorf("this record names no ask, and a record opens with the user's message, as in %q",
			`Ask: "post a tweet"`)
	}
	return reading.record, nil
}

// reader carries the record being built and where the scanner has got to, so
// that every rule can be a short method instead of one long function.
type reader struct {
	record      contract.Record
	headings    int
	list        string
	line        int
	sawBudget   bool
	sawProgress bool
}

// fail returns an error naming the line and what to do about it.
func (reading *reader) fail(what string, parts ...any) error {
	return fmt.Errorf("line %d of the record: %s", reading.line, fmt.Sprintf(what, parts...))
}

// failRule returns an error naming the line, what to do, and the rule that was
// broken, so that a caller can tell one rule from another with errors.Is.
func (reading *reader) failRule(rule error, what string, parts ...any) error {
	return fmt.Errorf("line %d of the record: %s: %w", reading.line, fmt.Sprintf(what, parts...), rule)
}

// readLine reads one line of the text, whichever of the five kinds it is.
func (reading *reader) readLine(text string) error {
	switch {
	case reading.line == 1:
		return reading.readHeader(text)
	case reading.line == 2 && reading.record.Header.Kind == contract.RecordTask:
		return reading.readCostLine(text)
	case strings.TrimSpace(text) == "":
		reading.list = ""
		return nil
	case strings.HasPrefix(text, "## "):
		return reading.openSection(text)
	case strings.HasPrefix(text, itemMark):
		return reading.readItem(strings.TrimPrefix(text, itemMark))
	default:
		return reading.openList(text)
	}
}

// readHeader reads the first line: the kind, the number, where the ask came
// from, and either the budget or the progress.
func (reading *reader) readHeader(text string) error {
	rest, found := strings.CutPrefix(text, "# ")
	if !found {
		return reading.fail("a record opens with %q and its kind, and this line does not", "# ")
	}
	fields := strings.Split(rest, headerGap)
	kindText, id, split := strings.Cut(fields[0], " ")
	// The number must really be a number: a job's reports are labelled from it,
	// as "j4.2" is from job four, and a job labelled anything else would write
	// reports that nothing could read back.
	if _, isNumber := readCount(id); !split || !isNumber {
		return reading.fail("the first field is %q, and it must be the kind and the number, as in %q", fields[0], "task 17")
	}
	kind := contract.RecordKind(kindText)
	if kind != contract.RecordTask && kind != contract.RecordJob {
		return reading.fail("the kind is %q, and a record is either a task or a job", kindText)
	}
	if len(fields) < 2 || !knownStatus(contract.RecordStatus(fields[1])) {
		return reading.fail("the second field must say where the record stands: running, waiting, stopped, failed, or done")
	}
	reading.record.Header.Kind = kind
	reading.record.Header.ID = unfoldText(id)
	reading.record.Header.Status = contract.RecordStatus(fields[1])
	for _, field := range fields[2:] {
		if err := reading.readHeaderField(field); err != nil {
			return err
		}
	}
	return reading.checkHeaderIsWhole()
}

// readHeaderField reads one of the fields after the kind and the status.
func (reading *reader) readHeaderField(field string) error {
	header := &reading.record.Header
	if origin, found := strings.CutPrefix(field, "from "); found {
		header.Origin = unfoldText(origin)
		return nil
	}
	if header.Kind == contract.RecordTask {
		return reading.readBudgetField(field)
	}
	if due, found := strings.CutPrefix(field, "next: "); found {
		header.NextDue = unfoldText(due)
		return nil
	}
	return reading.readProgressField(field)
}

// readBudgetField reads how much of a task's budget is left.
func (reading *reader) readBudgetField(field string) error {
	rounds, minutes := 0, 0
	if _, err := fmt.Sscanf(field, "budget left: %d rounds, %d minutes", &rounds, &minutes); err != nil {
		return reading.fail("the header field %q is one nobody knows, so write the budget as %q", field, "budget left: 86 rounds, 51 minutes")
	}
	if rounds < 0 || minutes < 0 || fmt.Sprintf("budget left: %d rounds, %d minutes", rounds, minutes) != field {
		return reading.fail("the budget field %q does not read as a whole number of rounds and minutes left", field)
	}
	reading.record.Header.RoundsLeft, reading.record.Header.MinutesLeft = rounds, minutes
	reading.sawBudget = true
	return nil
}

// readProgressField reads how many of a job's tasks are finished.
func (reading *reader) readProgressField(field string) error {
	done, total := 0, 0
	if _, err := fmt.Sscanf(field, "%d of %d tasks done", &done, &total); err != nil {
		return reading.fail("the header field %q is one nobody knows, so write the progress as %q", field, "3 of 12 tasks done")
	}
	if done < 0 || total < 0 || fmt.Sprintf("%d of %d tasks done", done, total) != field {
		return reading.fail("the progress field %q does not read as a count of finished tasks out of the whole", field)
	}
	reading.record.Header.TasksDone, reading.record.Header.TasksTotal = done, total
	reading.sawProgress = true
	return nil
}

// checkHeaderIsWhole says no to a header that is missing the one field its kind
// of record must carry.
func (reading *reader) checkHeaderIsWhole() error {
	if reading.record.Header.Kind == contract.RecordTask && !reading.sawBudget {
		return reading.fail("this task header carries no budget, so add %q to it", "budget left: 86 rounds, 51 minutes")
	}
	if reading.record.Header.Kind == contract.RecordJob && !reading.sawProgress {
		return reading.fail("this job header carries no progress, so add %q to it", "3 of 12 tasks done")
	}
	return nil
}

// readCostLine reads what the last turn cost, which is the second line of every
// task record and is written by the harness alone.
func (reading *reader) readCostLine(text string) error {
	rest, found := strings.CutPrefix(text, "this turn: ")
	if !found {
		return reading.fail("the second line of a task is what the turn cost, so write it as %q",
			"this turn: 6.1k tokens in, 5.2k of them cached, 0.4k out")
	}
	parts := strings.Split(rest, ", ")
	if len(parts) != 3 {
		return reading.fail("the cost line has %d parts and it must have three: in, cached, and out", len(parts))
	}
	cost := contract.CostLine{}
	input, readInput := readThousands(parts[0], " tokens in")
	cached, readCached := readThousands(parts[1], " of them cached")
	output, readOutput := readThousands(parts[2], " out")
	if !readInput || !readCached || !readOutput {
		return reading.fail("the cost line %q does not read as three counts in thousands of tokens, such as %q", text, "6.1k")
	}
	cost = contract.CostLine{InputTokens: input, CachedInputTokens: cached, OutputTokens: output}
	if printCostLine(cost) != text {
		return reading.fail("the cost line %q is not written the way a record writes one, so compare it with %q", text, printCostLine(cost))
	}
	reading.record.Header.Cost = cost
	return nil
}

// openSection opens one of the four parts, and refuses one that is out of order,
// because the goal and the rules must always come first.
func (reading *reader) openSection(text string) error {
	if reading.headings >= len(headingOrder) || text != headingOrder[reading.headings] {
		return reading.fail("this heading is %q, and a record has %s in that order",
			text, strings.Join(headingOrder, ", "))
	}
	reading.headings++
	reading.list = ""
	return nil
}

// openList opens the list a run of items belongs to, or reads the one-line piece
// of the goal that carries its text on the same line.
func (reading *reader) openList(text string) error {
	switch reading.headings {
	case 1:
		return reading.openGoalList(text)
	case 2:
		return reading.openOneOfTheLabels(text, labelCorrections, labelStopWhen)
	case 3:
		return reading.openWorkList(text)
	case 4:
		return reading.openOneOfTheLabels(text, labelDecisions, labelFailures)
	default:
		return reading.fail("this line sits above the first heading, and a record opens with %q", headingGoal)
	}
}

// openOneOfTheLabels opens whichever of the labels this line is, and refuses the rest.
func (reading *reader) openOneOfTheLabels(text string, labels ...string) error {
	for _, label := range labels {
		if text == label {
			reading.list = label
			return nil
		}
	}
	return reading.fail("the label %q is one nobody knows, so use one of: %s", text, strings.Join(labels, ", "))
}

// openGoalList reads the ask, the why, or the head of the done list.
func (reading *reader) openGoalList(text string) error {
	if ask, found := strings.CutPrefix(text, labelAsk); found {
		words, inQuotes := unquote(ask)
		if !inQuotes || words == "" {
			return reading.fail("the ask must be the user's message in quotes, as in %q", `Ask: "post a tweet"`)
		}
		reading.record.Goal.Ask = words
		reading.list = ""
		return nil
	}
	if why, found := strings.CutPrefix(text, labelWhy); found {
		if why == "" {
			return reading.fail("the why line is empty, so say in one line why the user wants this")
		}
		reading.record.Goal.Why = unfoldText(why)
		reading.list = ""
		return nil
	}
	return reading.openOneOfTheLabels(text, labelDoneWhen)
}

// openWorkList reads the head of the situation, the plan or the task list, or the
// results, whichever this kind of record has.
func (reading *reader) openWorkList(text string) error {
	if reading.record.Header.Kind == contract.RecordJob {
		return reading.openOneOfTheLabels(text, labelSituation, labelTasks, labelReports)
	}
	return reading.openOneOfTheLabels(text, labelSituation, labelPlan, labelResults)
}

// knownStatus says whether the word is one of the five places a record can stand.
func knownStatus(status contract.RecordStatus) bool {
	switch status {
	case contract.StatusRunning, contract.StatusWaiting, contract.StatusStopped, contract.StatusFailed, contract.StatusDone:
		return true
	default:
		return false
	}
}
