package loop

import (
	"fmt"
	"strconv"
	"strings"
)

// A plan with steps still open says the work is not over. The tenth nightly
// run's frontend task had its done list refused, its plan of five steps two
// marked done, and its reply "Let me write it carefully" closed the task as
// done, the answer taken as the whole done list. A record with no done list
// but a plan still open is sent back to the plan instead.

// thePlanStepsStillOpen names the plan steps not marked done and says what to
// do, or is empty when the plan is done or there is none.
func (running *run) thePlanStepsStillOpen() string {
	var open []int
	for _, step := range running.keeper.Record().Work.Plan {
		if !step.Done {
			open = append(open, step.Number)
		}
	}
	if len(open) == 0 {
		return ""
	}
	return fmt.Sprintf("The plan's %s not marked done, so the work is not over. Carry on with step %d, "+
		"and mark each step done on your first line when a result proves it, or write a failure with its cause.",
		stepsNamed(open), open[0])
}

// stepsNamed writes "step 3 is", "steps 1 and 2 are", or "steps 1, 2 and 3 are".
func stepsNamed(numbers []int) string {
	if len(numbers) == 1 {
		return "step " + strconv.Itoa(numbers[0]) + " is"
	}
	names := make([]string, len(numbers))
	for at, number := range numbers {
		names[at] = strconv.Itoa(number)
	}
	return "steps " + strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " are"
}
