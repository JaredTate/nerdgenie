package record

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The headings of the four parts. They are always printed, always in this order,
// because everything above the cache line must not move from one turn to the
// next.
const (
	headingGoal    = "## Goal"
	headingRules   = "## Rules"
	headingWork    = "## Work"
	headingLessons = "## Lessons"
)

// The label that opens each list inside a part. A label is printed only when the
// list under it has something in it.
const (
	labelAsk         = "Ask: "
	labelWhy         = "Why: "
	labelDoneWhen    = "Done when:"
	labelCorrections = "Corrections:"
	labelStopWhen    = "Stop and tell the user if:"
	labelSituation   = "Situation:"
	labelPlan        = "Plan:"
	labelTasks       = "Tasks:"
	labelResults     = "Results (read any of them in full with `read r7`):"
	labelReports     = "Reports (read any of them in full with `read j4.2`):"
	labelDecisions   = "Decisions:"
	labelFailures    = "Failures:"
)

// The small pieces of punctuation the format is built from.
const (
	headerGap   = "   "
	itemMark    = "- "
	markDone    = "[x]"
	markWaiting = "[ ]"
	arrow       = " -> "
	reasonJoin  = ". Reason: "
	causeJoin   = ". Cause: "
	fullStop    = "."
)

// Print writes a record in the one text form the design defines, which is the
// only form anything in Coeus ever writes. Parse reads it back.
func Print(record contract.Record) []byte {
	lines := printHeader(record.Header)
	for _, part := range [][]string{
		printGoal(record.Goal),
		printRules(record.Rules),
		printWork(record.Work, record.Header),
		printLessons(record.Lessons),
	} {
		lines = append(lines, "")
		lines = append(lines, part...)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// printHeader writes the first line of a record, and, on a task, the cost line
// under it. A task carries its budget and what the last turn cost; a job carries
// how many of its tasks are done and which one is due next.
func printHeader(header contract.Header) []string {
	parts := []string{"# " + string(header.Kind) + " " + foldText(header.ID), string(header.Status)}
	if header.Origin != "" {
		parts = append(parts, "from "+foldText(header.Origin))
	}
	if header.Kind == contract.RecordJob {
		parts = append(parts, fmt.Sprintf("%d of %d tasks done", header.TasksDone, header.TasksTotal))
		if header.NextDue != "" {
			parts = append(parts, "next: "+foldText(header.NextDue))
		}
		return []string{strings.Join(parts, headerGap)}
	}
	parts = append(parts, fmt.Sprintf("budget left: %d rounds, %d minutes", header.RoundsLeft, header.MinutesLeft))
	return []string{strings.Join(parts, headerGap), printCostLine(header.Cost)}
}

// printCostLine writes what the last turn cost, in thousands of tokens to one
// decimal place, which is the only precision the record keeps.
func printCostLine(cost contract.CostLine) string {
	return fmt.Sprintf("this turn: %s tokens in, %s of them cached, %s out",
		thousands(cost.InputTokens), thousands(cost.CachedInputTokens), thousands(cost.OutputTokens))
}

// thousands writes a token count as "6.1k". A count below zero is written as
// nothing at all, because a negative number of tokens is not a thing.
func thousands(tokens int) string {
	if tokens < 0 {
		tokens = 0
	}
	return fmt.Sprintf("%d.%dk", tokens/1000, (tokens%1000)/100)
}

// printGoal writes what the user asked for and what done looks like.
func printGoal(goal contract.Goal) []string {
	lines := []string{headingGoal}
	if goal.Ask != "" {
		lines = append(lines, labelAsk+quoted(goal.Ask))
	}
	if goal.Why != "" {
		lines = append(lines, labelWhy+foldText(goal.Why))
	}
	if len(goal.DoneWhen) == 0 {
		return lines
	}
	lines = append(lines, labelDoneWhen)
	arrows := arrowsAreNeeded(goal.DoneWhen)
	for _, line := range goal.DoneWhen {
		lines = append(lines, printDoneLine(line, arrows))
	}
	return lines
}

// printDoneLine writes one thing that must be true before the record can close.
// The arrow is drawn on every line once any line has a proof behind it, so that
// the lines still waiting for one are plain to see.
func printDoneLine(line contract.DoneLine, arrows bool) string {
	text := itemMark + checkMark(line.Done) + " " + foldText(line.Text)
	switch proof := proofOf(line); {
	case proof != "":
		return text + arrow + proof
	case arrows:
		return text + strings.TrimRight(arrow, " ")
	default:
		return text
	}
}

// proofOf returns what a done line points at: the result that proves it, or the
// user's own words in quotes when the proof is something the user said.
func proofOf(line contract.DoneLine) string {
	if line.ResultID != "" {
		return foldText(line.ResultID)
	}
	if line.UserReply != "" {
		return quoted(line.UserReply)
	}
	return ""
}

// arrowsAreNeeded says whether the done list is drawn with an arrow on every
// line. It is, once any line has something behind it, so that the lines still
// waiting are plain to see. It is also drawn when a line's own words end in an
// arrow, because a bare arrow at the end of a line is the mark of a line waiting
// for its proof, and without one of its own such a line would lose its last two
// characters the next time the record was read.
func arrowsAreNeeded(lines []contract.DoneLine) bool {
	for _, line := range lines {
		if proofOf(line) != "" || strings.HasSuffix(line.Text, arrowEnd) {
			return true
		}
	}
	return false
}

// checkMark writes the box at the front of a line that can be finished.
func checkMark(done bool) string {
	if done {
		return markDone
	}
	return markWaiting
}

// printRules writes what the user has corrected and what must stop the work.
func printRules(rules contract.Rules) []string {
	lines := []string{headingRules}
	if len(rules.Corrections) > 0 {
		lines = append(lines, labelCorrections)
		for _, correction := range rules.Corrections {
			lines = append(lines, itemMark+foldText(correction.ID)+" "+quoted(correction.Text))
		}
	}
	if len(rules.StopWhen) > 0 {
		lines = append(lines, labelStopWhen)
		for _, stop := range rules.StopWhen {
			lines = append(lines, itemMark+foldText(stop))
		}
	}
	return lines
}

// printWork writes where things stand: the facts the harness checked, the plan
// or the task list, and one line for every result.
func printWork(work contract.Work, header contract.Header) []string {
	lines := []string{headingWork}
	if len(work.Situation) > 0 {
		lines = append(lines, labelSituation)
		for _, fact := range work.Situation {
			lines = append(lines, itemMark+foldText(fact))
		}
	}
	lines = append(lines, printPlanOrTasks(work, header.Kind)...)
	if len(work.Results) == 0 {
		return lines
	}
	lines = append(lines, resultsLabel(header.Kind))
	for _, result := range work.Results {
		lines = append(lines, itemMark+foldText(result.ID)+" "+foldText(result.Summary))
	}
	return lines
}

// printPlanOrTasks writes a task's plan or a job's task list, whichever this
// record has.
func printPlanOrTasks(work contract.Work, kind contract.RecordKind) []string {
	lines := []string{}
	if kind == contract.RecordJob {
		if len(work.Tasks) == 0 {
			return lines
		}
		lines = append(lines, labelTasks)
		for _, task := range work.Tasks {
			lines = append(lines, printJobTask(task))
		}
		return lines
	}
	if len(work.Plan) == 0 {
		return lines
	}
	lines = append(lines, labelPlan)
	for _, step := range work.Plan {
		lines = append(lines, printPlanStep(step))
	}
	return lines
}

// printPlanStep writes one numbered step of a task's plan.
func printPlanStep(step contract.PlanStep) string {
	line := itemMark + checkMark(step.Done) + " " + strconv.Itoa(step.Number) + " " + foldText(step.Text)
	if step.ResultID != "" {
		line += arrow + foldText(step.ResultID)
	}
	return line
}

// printJobTask writes one task on a job's list, with the date it must wait for
// after the last comma of the line.
func printJobTask(task contract.JobTask) string {
	line := itemMark + checkMark(task.Done) + " " + foldText(task.TaskID) + " " + foldText(task.Text)
	if task.DueAt != "" {
		line += ", " + foldText(task.DueAt)
	}
	if task.ReportID != "" {
		line += arrow + foldText(task.ReportID)
	}
	return line
}

// resultsLabel is the line that opens the result list, which tells the model how
// to read any of them back in full.
func resultsLabel(kind contract.RecordKind) string {
	if kind == contract.RecordJob {
		return labelReports
	}
	return labelResults
}

// printLessons writes what has been decided and what has gone wrong, each with
// the reason or the cause it must carry.
func printLessons(lessons contract.Lessons) []string {
	lines := []string{headingLessons}
	if len(lessons.Decisions) > 0 {
		lines = append(lines, labelDecisions)
		for _, decision := range lessons.Decisions {
			lines = append(lines, itemMark+foldText(decision.ID)+" "+foldText(decision.Text)+reasonJoin+foldText(decision.Reason)+fullStop)
		}
	}
	if len(lessons.Failures) > 0 {
		lines = append(lines, labelFailures)
		for _, failure := range lessons.Failures {
			lines = append(lines, itemMark+foldText(failure.ID)+" "+foldText(failure.Text)+causeJoin+foldText(failure.Cause)+fullStop)
		}
	}
	return lines
}
