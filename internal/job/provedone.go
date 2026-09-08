package job

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// A job made from a work order carries the person's done lines, and the
// harness proves them one at a time: a bracketed check it ran, or, at the
// close, the task reports. The marks go through the record's own rules, so a
// line can only point at a report the job holds, and the job closes the way
// it always has, when every task is done and the done check passes.

// doneLinesOf turns the person's lines into unmarked done lines.
func doneLinesOf(lines []string) []contract.DoneLine {
	made := make([]contract.DoneLine, 0, len(lines))
	for _, line := range lines {
		made = append(made, contract.DoneLine{Text: line})
	}
	return made
}

// ProveDoneLine marks one line of the job's done list, counting from one, done
// and pointing at the report that proves it, or unmarks it when the result is
// empty, and closes the job when every line is proved and every task is done.
func (jobs *Jobs) ProveDoneLine(ctx context.Context, jobID string, number int, resultID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	lines := append([]contract.DoneLine(nil), held.keeper.Record().Goal.DoneWhen...)
	if number < 1 || number > len(lines) {
		return fmt.Errorf("the job %s has no done line %d, because its done list holds %d lines", jobID, number, len(lines))
	}
	lines[number-1].Done = resultID != ""
	lines[number-1].ResultID = resultID
	if err := held.keeper.Apply(ctx, record.Update{DoneWhen: lines}); err != nil {
		return fmt.Errorf("cannot mark done line %d of job %s: %w", number, jobID, err)
	}
	return jobs.closeIfEveryTaskIsDone(ctx, jobID, held, jobs.clock.Now())
}
