package loop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The pace of a task against its job. On the night of 7 September 2026 the
// flight simulator's sky task ran 443 rounds and ten hours where its
// neighbours took 84 to 173 rounds, almost all of it on one done line, and
// nothing in the harness weighed one task against the job; the work order's
// own visual QA and final regression tasks, which exist to catch what earlier
// tasks left, never got the chance. A job's task is now measured once a
// round against the median running time of the job's finished tasks.

const (
	// CostLineAtTimesTheMedian is the multiple of the job's median at which
	// the model reads its cost line, once: at twice the median a task has
	// spent what two ordinary tasks of the job would, which is when a task
	// that is behind should be told so while it can still close on what it
	// holds.
	CostLineAtTimesTheMedian = 2
	// CloseAtTimesTheMedian is the multiple at which the harness ends the
	// task as failed: three ordinary tasks' time is room for a hard task and
	// not for a stuck one, and the job's later tasks read the report of what
	// was proved and what stood in the way.
	CloseAtTimesTheMedian = 3
	// TasksThatMakeAMedian is how many finished tasks the job needs before
	// there is a median to pace against: one task's time is a guess, and the
	// job's second task is measured against nothing.
	TasksThatMakeAMedian = 2
	// LeastMedianToPaceBy is the shortest median a task is paced against: a
	// job whose tasks finish inside five minutes is a job of small tasks,
	// and a small task's time says nothing about how long a big one should
	// take, so on a job whose first two tasks answered in a minute each the
	// third would otherwise be closed at three minutes.
	LeastMedianToPaceBy = 5 * time.Minute
)

// paceAgainstTheJob measures the task once a round against the median running
// time of its job's finished tasks, and says whether it ended the task. The
// task's running time is measured from the start of this run of it. At twice
// the median the model reads the cost line, once; at three times the harness
// ends the task as failed with what was proved and the text of every open
// line on the report, so the job takes it as a failed task and goes on. The
// done check is not bypassed: a task that has not proved a line does not close
// as done. A plain task outside a job, and a job's task while fewer than two
// of the job's tasks have finished, are never paced.
func (running *run) paceAgainstTheJob(ctx context.Context) (Outcome, bool, error) {
	median, there := running.theJobsMedian(ctx)
	if !there {
		return Outcome{}, false, nil
	}
	ran := running.theLoop.options.Clock.Now().Sub(running.startedAt)
	if ran >= CloseAtTimesTheMedian*median {
		outcome, err := running.failHere(ctx, errors.New(thePacedOutReport(running.theDoneLines())))
		return outcome, true, err
	}
	if ran >= CostLineAtTimesTheMedian*median && !running.costLineSaid {
		running.costLineSaid = true
		running.remember(contract.Message{Role: contract.RoleUser, Text: theCostLine(ran, median, running.theDoneLines())})
	}
	return Outcome{}, false, nil
}

// theJobsMedian is the median running time of the job's finished tasks, each
// task's finish less its start on the store's clock over the tasks that have
// both, and false for a task outside a job, a job with fewer than
// TasksThatMakeAMedian finished, or a median under LeastMedianToPaceBy. An
// even count takes the middle two's mean.
func (running *run) theJobsMedian(ctx context.Context) (time.Duration, bool) {
	if running.task.FromJob == nil || running.theLoop.options.Jobs == nil {
		return 0, false
	}
	took := []time.Duration{}
	for _, task := range running.theLoop.theTimingOf(ctx, running.task.FromJob.JobID).Tasks {
		if task.Started.IsZero() || task.Finished.IsZero() {
			continue
		}
		took = append(took, task.Finished.Sub(task.Started))
	}
	if len(took) < TasksThatMakeAMedian {
		return 0, false
	}
	slices.Sort(took)
	middle := len(took) / 2
	median := took[middle]
	if len(took)%2 == 0 {
		median = (took[middle-1] + took[middle]) / 2
	}
	return median, median >= LeastMedianToPaceBy
}

// theDoneLines is the record's done list, and nothing for a task that has
// made no record yet.
func (running *run) theDoneLines() []contract.DoneLine {
	if running.keeper == nil {
		return nil
	}
	return running.keeper.Record().Goal.DoneWhen
}

// provedCount is how many of the done lines are marked proved.
func provedCount(lines []contract.DoneLine) int {
	proved := 0
	for _, line := range lines {
		if line.Done {
			proved++
		}
	}
	return proved
}

// theCostLine is what the model reads once at twice the median: how long the
// task has run against the median, in minutes, how many done lines are
// proved, and what to do about the open ones.
func theCostLine(ran time.Duration, median time.Duration, lines []contract.DoneLine) string {
	return fmt.Sprintf("this task has run %d minutes against a median of %d for this job's finished tasks; "+
		"%d of %d done lines are proved; prove the open lines with what you hold, or write what stands in their way as a failure and close the task",
		int(ran.Minutes()), int(median.Minutes()), provedCount(lines), len(lines))
}

// thePacedOutReport is the report of a task the harness ended at three times
// the median: how many done lines were proved, and the text of every open
// line, so the job's later tasks know what was left.
func thePacedOutReport(lines []contract.DoneLine) string {
	report := fmt.Sprintf("paced out at three times the job's median: %d of %d done lines proved", provedCount(lines), len(lines))
	open := []string{}
	for _, line := range lines {
		if !line.Done {
			open = append(open, line.Text)
		}
	}
	if len(open) == 0 {
		return report
	}
	return report + "; not proved: " + strings.Join(open, "; ")
}
