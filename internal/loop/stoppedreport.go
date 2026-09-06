package loop

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// howToCarryOn is the last line of a person's stopped report: the invitation
// to say how to carry on, or, when this task made a job that is running now,
// which job and which task of it runs next and how to stop that too. The live
// run found the invitation misleading there: the task's work had become the
// job's, the driver started the job's first task the moment the task ended,
// and the person's "continue" landed on that task. A job store that cannot
// be read costs the report that line and nothing more, because the stop
// itself must not wait on it.
func (running *run) howToCarryOn(ctx context.Context) string {
	theInvitation := "Tell me how to carry on and I will pick it up from here; type /clear first to set it aside."
	if len(running.jobsMade) == 0 || running.theLoop.options.Jobs == nil {
		return theInvitation
	}
	listed, err := running.theLoop.options.Jobs.List(ctx)
	if err != nil {
		return theInvitation
	}
	for at := len(running.jobsMade) - 1; at >= 0; at-- {
		for _, summary := range listed {
			if summary.ID == running.jobsMade[at] && summary.State == contract.JobRunning {
				return theJobRunsOnLine(summary)
			}
		}
	}
	return theInvitation
}

// theJobRunsOnLine says which job the stopped task made is running and which
// of its tasks comes next, and how to stop that too.
func theJobRunsOnLine(summary contract.JobSummary) string {
	which := "next"
	if summary.TasksDone == 0 {
		which = "first"
	}
	if summary.NextTaskID == "" {
		return fmt.Sprintf("Job %s, which this task made, is running; watch it on the side, and say stop to stop that too.", summary.ID)
	}
	return fmt.Sprintf("Job %s, which this task made, is running: its %s task, %s, starts as soon as this one ends. Watch it on the side, and say stop to stop that too.",
		summary.ID, which, summary.NextTaskID)
}
