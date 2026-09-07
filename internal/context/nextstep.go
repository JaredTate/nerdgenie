package context

import (
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// NextStepLine is the last thing the model reads: the step it is on, the
// newest result, the step after, and how many done lines are open, in one
// line the harness writes from the record. The model answers the last thing
// it reads, which the plan regression of 6 September 2026 proved: with a
// folder listing after the ask, no task but the first wrote a plan. A task
// with no record yet has no line.
func NextStepLine(held contract.Record) string {
	if held.Header.Kind == "" {
		return ""
	}
	parts := []string{}
	open, next := openSteps(held.Work.Plan)
	openLines := fmt.Sprintf("Open done lines: %d.", openDoneLines(held.Goal.DoneWhen))
	if open != nil {
		parts = append(parts, fmt.Sprintf("Step %d of %d: %s.", open.Number, len(held.Work.Plan), strings.TrimSuffix(open.Text, ".")))
	} else {
		parts = append(parts, openLines)
	}
	if last := len(held.Work.Results) - 1; last >= 0 {
		parts = append(parts, fmt.Sprintf("Last: %s %s.", held.Work.Results[last].ID, strings.TrimSuffix(held.Work.Results[last].Summary, ".")))
	}
	if next != nil {
		parts = append(parts, fmt.Sprintf("Next: step %d, %s.", next.Number, strings.TrimSuffix(next.Text, ".")))
	}
	if open != nil {
		parts = append(parts, openLines)
	}
	return strings.Join(parts, " ")
}

// openSteps is the first step not yet done and the one after it, either of
// which may be missing.
func openSteps(plan []contract.PlanStep) (*contract.PlanStep, *contract.PlanStep) {
	for at := range plan {
		if plan[at].Done {
			continue
		}
		open := &plan[at]
		if at+1 < len(plan) {
			return open, &plan[at+1]
		}
		return open, nil
	}
	return nil, nil
}

// openDoneLines counts the done lines not yet marked.
func openDoneLines(lines []contract.DoneLine) int {
	open := 0
	for _, line := range lines {
		if !line.Done {
			open++
		}
	}
	return open
}
