package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// StandingOrderFile is the name of the file a project keeps its rules in: what
// it is, how to run it, how to test it, what holds on every task. It is the
// name the other agents read too, so a project that has one for them has one
// for Nerd Genie.
const StandingOrderFile = "NERDGENIE.md"

// MaxStandingOrderBytes is the most of the file that is read. A standing order
// is shown to sixty lines, so a file past this size is one nobody meant the
// model to read whole.
const MaxStandingOrderBytes = 64 << 10

// readTheStandingOrder reads the work folder's NERDGENIE.md once, at the task's
// start, so that it rides in front of the model on every call of the task. A
// folder with none, or one that cannot be read, gives nothing, because the
// rules of a project are a help and never a reason to fail a task.
func readTheStandingOrder(folder string) string {
	if folder == "" {
		return ""
	}
	held, err := os.ReadFile(filepath.Join(folder, StandingOrderFile))
	if err != nil {
		return ""
	}
	if len(held) > MaxStandingOrderBytes {
		held = held[:MaxStandingOrderBytes]
	}
	return string(held)
}

// readWhatRidesInFront gathers the two things that sit under the tools and
// above the record on every call of the task and hold still through it: the
// summary of the job the task belongs to, and the work folder's standing
// order, its NERDGENIE.md, when the folder has one.
func (running *run) readWhatRidesInFront(ctx context.Context) error {
	if err := running.readJobSummary(ctx); err != nil {
		return err
	}
	running.standingOrder = readTheStandingOrder(running.folder())
	return nil
}

// readJobSummary prints the job this task belongs to, which rides above the
// task record so that a later task can lean on the reports of the earlier ones.
func (running *run) readJobSummary(ctx context.Context) error {
	if running.task.FromJob == nil || running.theLoop.options.Jobs == nil {
		return nil
	}
	held, err := running.theLoop.options.Jobs.Load(ctx, running.task.FromJob.JobID)
	if err != nil {
		return fmt.Errorf("cannot read job %s to put its summary above the task: %w", running.task.FromJob.JobID, err)
	}
	running.jobSummary = running.theJobSummaryOf(held)
	running.projectFolder = projectFolderIn(held)
	return nil
}
