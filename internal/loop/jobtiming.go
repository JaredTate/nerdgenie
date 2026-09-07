package loop

import (
	"context"
	"time"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A person watching a long job wants to know how long its tasks take and how
// long the whole job has run. The job store keeps the moments on the
// harness's clock, and the two lines the person reads at a task's end and at
// the job's end carry them in words, so that the log and the terminal say
// what each took without anybody counting.

// theTookLine is what a task's progress line ends with when the store has
// both the task's moments, such as " Task t1 took 3m 12s.", and nothing when
// it has not.
func theTookLine(timing contract.JobTiming, taskID string) string {
	held, there := timing.Tasks[taskID]
	if !there || held.Started.IsZero() || held.Finished.IsZero() {
		return ""
	}
	return " Task " + taskID + " took " + clock.Words(held.Finished.Sub(held.Started)) + "."
}

// theJobsSpan is what the job's closing line ends with: ", in 42m 3s" from
// the job's start to its finish, or to now when the store has not closed it,
// and nothing when the start is unknown.
func theJobsSpan(timing contract.JobTiming, now time.Time) string {
	if timing.Started.IsZero() {
		return ""
	}
	end := timing.Finished
	if end.IsZero() {
		end = now
	}
	return ", in " + clock.Words(end.Sub(timing.Started))
}

// theTimingOf reads the job's timing, or an empty timing when the store
// cannot say, because a report is never lost over a span of time.
func (theLoop *Loop) theTimingOf(ctx context.Context, jobID string) contract.JobTiming {
	if theLoop.options.Jobs == nil {
		return contract.JobTiming{}
	}
	timing, err := theLoop.options.Jobs.Timing(ctx, jobID)
	if err != nil {
		return contract.JobTiming{}
	}
	return timing
}
