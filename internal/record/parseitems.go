package record

import (
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// arrowEnd is the arrow with nothing behind it, which a done line still waiting
// for its result ends with.
const arrowEnd = " ->"

// readItem reads one line that opens with "- ", which is one entry of whichever
// list is open above it.
func (reading *reader) readItem(text string) error {
	switch reading.list {
	case labelDoneWhen:
		return reading.readDoneLine(text)
	case labelCorrections:
		return reading.readCorrection(text)
	case labelStopWhen:
		return reading.readPlainLine(text, &reading.record.Rules.StopWhen, "the stop list")
	case labelSituation:
		return reading.readPlainLine(text, &reading.record.Work.Situation, "the situation")
	case labelPlan:
		return reading.readPlanStep(text)
	case labelTasks:
		return reading.readJobTask(text)
	case labelResults, labelReports:
		return reading.readResultLine(text)
	case labelDecisions:
		return reading.readDecision(text)
	case labelFailures:
		return reading.readFailure(text)
	default:
		return reading.fail("this item has no list above it, so put a label such as %q before it", labelSituation)
	}
}

// readPlainLine reads one line of a list that is nothing but text.
func (reading *reader) readPlainLine(text string, into *[]string, what string) error {
	if text == "" {
		return reading.fail("this line of %s is empty, so write it or take it out", what)
	}
	*into = append(*into, unfoldText(text))
	return nil
}

// readCheckMark reads the box at the front of a line that can be finished.
func (reading *reader) readCheckMark(text string) (bool, string, error) {
	if rest, found := strings.CutPrefix(text, markDone+" "); found {
		return true, rest, nil
	}
	if rest, found := strings.CutPrefix(text, markWaiting+" "); found {
		return false, rest, nil
	}
	return false, "", reading.fail("this line opens with %q or %q, because it is something that can be finished", markWaiting, markDone)
}

// splitAtArrow takes the proof off the end of a line. What follows the last arrow
// is the proof only when it reads as one, so that an arrow inside the text itself
// stays where it is.
func (reading *reader) splitAtArrow(text string, isProof func(string) bool) (string, string) {
	if body, found := strings.CutSuffix(text, arrowEnd); found {
		return body, ""
	}
	at := strings.LastIndex(text, arrow)
	if at < 0 {
		return text, ""
	}
	candidate := text[at+len(arrow):]
	if !isProof(candidate) {
		return text, ""
	}
	return text[:at], candidate
}

// countsUpwards holds the rule that the labels of one list only ever count
// upwards, so that nothing a reader has already seen is ever renumbered.
func (reading *reader) countsUpwards(id string, number int, before string, beforeNumber int) error {
	if before == "" || number > beforeNumber {
		return nil
	}
	return reading.failRule(ErrIdentifiersOutOfOrder, "the label %q comes after %q, and the labels of a list only count upwards", id, before)
}

// readDoneLine reads one thing that must be true before the record can close.
func (reading *reader) readDoneLine(text string) error {
	done, rest, err := reading.readCheckMark(text)
	if err != nil {
		return err
	}
	body, proof := reading.splitAtArrow(rest, reading.provesADoneLine)
	if body == "" {
		return reading.fail("this done line has no text, so say in one line what must be true")
	}
	line := contract.DoneLine{Text: unfoldText(body), Done: done}
	if reply, inQuotes := unquote(proof); inQuotes {
		line.UserReply = reply
	} else {
		line.ResultID = unfoldText(proof)
	}
	if done && line.ResultID == "" && line.UserReply == "" {
		return reading.failRule(ErrDoneLineNeedsProof, "the done line %q is marked done and names nothing behind it", body)
	}
	reading.record.Goal.DoneWhen = append(reading.record.Goal.DoneWhen, line)
	return nil
}

// provesADoneLine says whether the text after an arrow on a done line is a proof:
// a result of this record, or the user's own words in quotes.
func (reading *reader) provesADoneLine(candidate string) bool {
	if words, inQuotes := unquote(candidate); inQuotes {
		return words != ""
	}
	return knownResultID(reading.record.Header.Kind, reading.record.Header.ID, candidate)
}

// readCorrection reads one thing the user said while the work was running, which
// is kept in the user's own words and never edited.
func (reading *reader) readCorrection(text string) error {
	id, words, split := strings.Cut(text, " ")
	corrections := reading.record.Rules.Corrections
	wanted := contract.CorrectionID(len(corrections) + 1)
	if !split || id != wanted {
		return reading.failRule(ErrIdentifiersOutOfOrder, "this correction is labelled %q and the next label is %q", id, wanted)
	}
	said, inQuotes := unquote(words)
	if !inQuotes || said == "" {
		return reading.fail("a correction is the user's own words in quotes, as in %q", `- C1 "no, lead with the date"`)
	}
	reading.record.Rules.Corrections = append(corrections, contract.Correction{ID: id, Text: said})
	return nil
}

// readPlanStep reads one numbered step of a task's plan.
func (reading *reader) readPlanStep(text string) error {
	done, rest, err := reading.readCheckMark(text)
	if err != nil {
		return err
	}
	body, proof := reading.splitAtArrow(rest, isResultID)
	numberText, what, split := strings.Cut(body, " ")
	number, isNumber := readCount(numberText)
	wanted := len(reading.record.Work.Plan) + 1
	if !split || !isNumber || number != wanted {
		return reading.fail("a plan step opens with its number and the next one is %d, as in %q", wanted, "- [ ] 3 post it")
	}
	if what == "" {
		return reading.fail("the plan step numbered %d has no text, so say what the step does", number)
	}
	if done && proof == "" {
		return reading.failRule(ErrPlanStepNeedsResult, "the plan step numbered %d is marked done and names no result", number)
	}
	reading.record.Work.Plan = append(reading.record.Work.Plan,
		contract.PlanStep{Number: number, Text: unfoldText(what), Done: done, ResultID: unfoldText(proof)})
	return nil
}

// readJobTask reads one task on a job's list, with the date it must wait for
// after the last comma of its text.
func (reading *reader) readJobTask(text string) error {
	done, rest, err := reading.readCheckMark(text)
	if err != nil {
		return err
	}
	body, proof := reading.splitAtArrow(rest, reading.isReportOfThisJob)
	id, what, split := strings.Cut(body, " ")
	number, isTaskID := contract.ParseTaskID(id)
	if !split || !isTaskID || what == "" {
		return reading.fail("a task on a job opens with its identifier and what it does, as in %q", "- [ ] t31 post for day three")
	}
	if err := reading.checkTaskOrder(id, number); err != nil {
		return err
	}
	if done && proof == "" {
		return reading.failRule(ErrJobTaskNeedsReport, "the task %s is marked done and names no report", id)
	}
	what, due := splitDueDate(what)
	reading.record.Work.Tasks = append(reading.record.Work.Tasks,
		contract.JobTask{TaskID: id, Text: unfoldText(what), Done: done, ReportID: unfoldText(proof), DueAt: unfoldText(due)})
	return nil
}

// checkTaskOrder holds the rule that a job lists its tasks in order, so that a
// later task can lean on the reports of the ones before it.
func (reading *reader) checkTaskOrder(id string, number int) error {
	tasks := reading.record.Work.Tasks
	if len(tasks) == 0 {
		return nil
	}
	before := tasks[len(tasks)-1].TaskID
	beforeNumber, _ := contract.ParseTaskID(before)
	if number <= beforeNumber {
		return reading.failRule(ErrTasksOutOfOrder, "the task %s comes after %s, and a job lists its tasks in order", id, before)
	}
	return nil
}

// isReportOfThisJob says whether the text after an arrow is a report this job
// wrote, such as "j4.2" inside job number four.
func (reading *reader) isReportOfThisJob(candidate string) bool {
	jobID, _, valid := contract.ParseReportID(candidate)
	return valid && jobID == reading.record.Header.ID
}

// isResultID says whether the text after an arrow is a task result, such as "r3".
func isResultID(candidate string) bool {
	_, valid := contract.ParseResultID(candidate)
	return valid
}

// splitDueDate takes the date a task must wait for off the end of its text, which
// is everything after the last comma.
func splitDueDate(text string) (string, string) {
	at := strings.LastIndex(text, ", ")
	if at < 0 || at+2 >= len(text) {
		return text, ""
	}
	return text[:at], text[at+2:]
}

// readResultLine reads the one line a result keeps in the record, whose full text
// is in the event log under the same label.
func (reading *reader) readResultLine(text string) error {
	pinned := false
	if rest, marked := strings.CutPrefix(text, markPinned+" "); marked {
		pinned, text = true, rest
	}
	id, summary, split := strings.Cut(text, " ")
	number, isLabel := ResultNumber(reading.record.Header, id)
	if !split || !isLabel {
		return reading.fail("a result is labelled %q and then one line saying what it was", nextResultID(reading.record.Header, 0))
	}
	if summary == "" {
		return reading.fail("the result %s has no summary, so say in one line what it was", id)
	}
	results := reading.record.Work.Results
	before, beforeNumber := "", 0
	if len(results) > 0 {
		before = results[len(results)-1].ID
		beforeNumber, _ = ResultNumber(reading.record.Header, before)
	}
	if err := reading.countsUpwards(id, number, before, beforeNumber); err != nil {
		return err
	}
	reading.record.Work.Results = append(results,
		contract.ResultLine{ID: id, Summary: unfoldText(summary), Pinned: pinned})
	return nil
}

// readDecision reads one choice the model made, which must carry its reason.
func (reading *reader) readDecision(text string) error {
	id, body, split := strings.Cut(text, " ")
	decisions := reading.record.Lessons.Decisions
	wanted := contract.DecisionID(len(decisions) + 1)
	if !split || id != wanted {
		return reading.failRule(ErrIdentifiersOutOfOrder, "this decision is labelled %q and the next label is %q", id, wanted)
	}
	what, reason, err := reading.splitAtJoin(body, reasonJoin, ErrDecisionNeedsReason, "a reason")
	if err != nil {
		return err
	}
	reading.record.Lessons.Decisions = append(decisions, contract.Decision{ID: id, Text: what, Reason: reason})
	return nil
}

// readFailure reads one thing that went wrong, which must carry its cause.
func (reading *reader) readFailure(text string) error {
	id, body, split := strings.Cut(text, " ")
	failures := reading.record.Lessons.Failures
	wanted := contract.FailureID(len(failures) + 1)
	if !split || id != wanted {
		return reading.failRule(ErrIdentifiersOutOfOrder, "this failure is labelled %q and the next label is %q", id, wanted)
	}
	what, cause, err := reading.splitAtJoin(body, causeJoin, ErrFailureNeedsCause, "a cause")
	if err != nil {
		return err
	}
	reading.record.Lessons.Failures = append(failures, contract.Failure{ID: id, Text: what, Cause: cause})
	return nil
}

// splitAtJoin cuts a lesson into what happened and why, which is the rule that
// makes the lessons worth keeping.
func (reading *reader) splitAtJoin(body string, join string, rule error, needs string) (string, string, error) {
	trimmed, found := strings.CutSuffix(body, fullStop)
	if !found {
		return "", "", reading.fail("this line ends with a full stop, and it reads %q", body)
	}
	at := strings.LastIndex(trimmed, join)
	if at < 0 {
		return "", "", reading.failRule(rule, "this line carries no %s, so write it as %q", needs, "what happened"+join+"why")
	}
	what, why := trimmed[:at], trimmed[at+len(join):]
	if what == "" || why == "" {
		return "", "", reading.failRule(rule, "this line carries no %s, so write it as %q", needs, "what happened"+join+"why")
	}
	return unfoldText(what), unfoldText(why), nil
}

// ResultNumber reads the number out of a result label, and says whether the label
// is one this kind of record writes: "r7" on a task, "j4.2" on job number four.
func ResultNumber(header contract.Header, id string) (int, bool) {
	if header.Kind == contract.RecordJob {
		jobID, number, valid := contract.ParseReportID(id)
		return number, valid && jobID == header.ID
	}
	return contract.ParseResultID(id)
}

// nextResultID is the label the next result of this record gets: "r7" on a task
// and "j4.2" on job number four.
func nextResultID(header contract.Header, sofar int) string {
	if header.Kind == contract.RecordJob {
		return contract.ReportID(header.ID, sofar+1)
	}
	return contract.ResultID(sofar + 1)
}

// readCount reads a whole number of at least one, written the way this package
// writes one: digits only, with no sign and no leading zero.
func readCount(text string) (int, bool) {
	if text == "" || text[0] == '0' {
		return 0, false
	}
	for at := range len(text) {
		if text[at] < '0' || text[at] > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return number, true
}
