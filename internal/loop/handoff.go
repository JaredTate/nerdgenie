package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// handTheWorkToTheJob ends the task that made a job with a first task. Every
// game build so far made the job and then kept working inside the task that
// made it: the job's tasks could not start until that task ended, so the side
// panel showed nought of ten done for an hour while the work went on unmarked,
// and the task's record and not the job's held all of it. A job with a first
// task carries the work from the moment it is made, so the task that made it
// ends here, done, with one done line naming the job and proved by the job
// tool's own result, and the driver takes the job's first task the moment the
// loop is free. A job made with a schedule and no first task carries none of
// the task's work, and the task goes on.
func (running *run) handTheWorkToTheJob(ctx context.Context, results []contract.ToolResult) (Outcome, error) {
	jobID := running.jobToHandTo
	running.jobToHandTo = ""
	if running.keeper != nil {
		line := contract.DoneLine{Text: "the ask became job " + jobID + ", whose tasks carry the work from here", ResultID: labelOfTheJobResult(results)}
		if err := running.keeper.Apply(ctx, record.Update{DoneWhen: []contract.DoneLine{line}}); err != nil {
			return Outcome{}, fmt.Errorf("cannot write the done line that hands task %s over to job %s: %w", running.keeper.ID(), jobID, err)
		}
	}
	return running.finish(ctx, running.theHandOverReport(ctx, jobID))
}

// labelOfTheJobResult is the label of the result the job tool answered with,
// which is the proof the hand-over's done line points at, and empty when the
// results hold none.
func labelOfTheJobResult(results []contract.ToolResult) string {
	for _, result := range results {
		if strings.HasPrefix(result.Text, "created job ") {
			return result.Label
		}
	}
	return ""
}

// theHandOverReport is the one message the person reads when their task
// becomes a job: which job, how many tasks, that the first starts now, and
// where to watch it. The job store fills the numbers in when it can be read,
// and the report says less rather than waiting on it when it cannot.
func (running *run) theHandOverReport(ctx context.Context, jobID string) string {
	report := fmt.Sprintf("I made job %s, and its first task starts now. Watch it on the side panel, and say stop to stop it.", jobID)
	if running.theLoop.options.Jobs == nil {
		return report
	}
	listed, err := running.theLoop.options.Jobs.List(ctx)
	if err != nil {
		return report
	}
	for _, summary := range listed {
		if summary.ID == jobID && summary.NextTaskID != "" {
			return fmt.Sprintf("I made job %s with %d tasks, and its first task, %s, starts now. Watch it on the side panel, and say stop to stop it.",
				jobID, summary.TasksTotal, summary.NextTaskID)
		}
	}
	return report
}
