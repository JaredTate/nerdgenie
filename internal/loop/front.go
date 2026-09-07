package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
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
// order, its NERDGENIE.md, when the folder has one. A rule the job summary
// already prints is left out of the standing order, so that no rule is in
// front of the model twice.
func (running *run) readWhatRidesInFront(ctx context.Context) error {
	shown, err := running.readJobSummary(ctx)
	if err != nil {
		return err
	}
	if running.keeper != nil {
		shown = append(shown, running.keeper.Record().Rules.Corrections...)
	}
	running.standingOrder = withoutTheRulesAlreadyShown(readTheStandingOrder(running.folder()), shown)
	return nil
}

// readJobSummary prints the job this task belongs to, which rides above the
// task record so that a later task can lean on the reports of the earlier
// ones, and returns the job's rules, which the summary prints.
func (running *run) readJobSummary(ctx context.Context) ([]contract.Correction, error) {
	if running.task.FromJob == nil || running.theLoop.options.Jobs == nil {
		return nil, nil
	}
	held, err := running.theLoop.options.Jobs.Load(ctx, running.task.FromJob.JobID)
	if err != nil {
		return nil, fmt.Errorf("cannot read job %s to put its summary above the task: %w", running.task.FromJob.JobID, err)
	}
	running.jobSummary = running.theJobSummaryOf(held)
	running.projectFolder = projectFolderIn(held)
	return held.Rules.Corrections, nil
}

// withoutTheRulesAlreadyShown drops from the standing order every list line
// whose text is one of the rules the record already prints above it, word
// for word after the list mark and the spaces around it.
func withoutTheRulesAlreadyShown(order string, shown []contract.Correction) string {
	if order == "" || len(shown) == 0 {
		return order
	}
	held := make(map[string]bool, len(shown))
	for _, rule := range shown {
		held[strings.TrimSpace(rule.Text)] = true
	}
	lines := strings.Split(order, "\n")
	kept := lines[:0]
	for _, line := range lines {
		text, isAnItem := strings.CutPrefix(strings.TrimSpace(line), "- ")
		if isAnItem && held[strings.TrimSpace(text)] {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
