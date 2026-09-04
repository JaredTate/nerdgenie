package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/job"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theEpoch is the moment every job test starts from, so that a due date in a
// test reads as an obvious number of hours from the start.
func theEpoch() time.Time {
	return time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
}

// opened is one job store on a real event log in a temporary home, with the
// pieces a test needs to close it and open it again.
type opened struct {
	jobs     *job.Jobs
	eventLog *log.Log
	clock    *testkit.FakeClock
	home     contract.Home
}

// newJobs opens an event log and a job store on a temporary home, and closes
// both when the test ends.
func newJobs(t *testing.T) *opened {
	t.Helper()
	home := testkit.NewTempHome(t)
	return openJobsIn(t, home, testkit.NewFakeClock(theEpoch()))
}

// openJobsIn opens an event log and a job store on a home that already exists,
// which is what a restart test uses the second time round.
func openJobsIn(t *testing.T, home contract.Home, clock *testkit.FakeClock) *opened {
	t.Helper()
	ctx := t.Context()
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log in the temporary home: %v", err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })

	store, err := job.Open(ctx, home, eventLog, clock)
	if err != nil {
		t.Fatalf("cannot open the job store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &opened{jobs: store, eventLog: eventLog, clock: clock, home: home}
}

// restart closes the job store and the log under it and opens both again on the
// same home, which is how a test proves that a job is rebuilt from the log.
func (holding *opened) restart(t *testing.T) *opened {
	t.Helper()
	if err := holding.jobs.Close(); err != nil {
		t.Fatalf("cannot close the job store: %v", err)
	}
	if err := holding.eventLog.Close(); err != nil {
		t.Fatalf("cannot close the event log: %v", err)
	}
	return openJobsIn(t, holding.home, holding.clock)
}

// aJob creates a plain job with the ask given and fails the test when it cannot.
func (holding *opened) aJob(t *testing.T, ask string) string {
	t.Helper()
	jobID, err := holding.jobs.Create(context.Background(), contract.NewJob{Ask: ask, Why: "because the user asked"})
	if err != nil {
		t.Fatalf("cannot create the job %q: %v", ask, err)
	}
	return jobID
}

// aTask adds one task to a job and fails the test when it cannot.
func (holding *opened) aTask(t *testing.T, jobID string, text string, dueAt time.Time) string {
	t.Helper()
	taskID, err := holding.jobs.AddTask(context.Background(), contract.NewTask{JobID: jobID, Text: text, DueAt: dueAt})
	if err != nil {
		t.Fatalf("cannot add the task %q: %v", text, err)
	}
	return taskID
}

// summaryOf reads one job's summary out of the listing.
func (holding *opened) summaryOf(t *testing.T, jobID string) contract.JobSummary {
	t.Helper()
	listed, err := holding.jobs.List(context.Background())
	if err != nil {
		t.Fatalf("cannot list the jobs: %v", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary
		}
	}
	t.Fatalf("the job %s is not in the listing", jobID)
	return contract.JobSummary{}
}

// finish finishes one task of a job and returns the report's identifier.
func (holding *opened) finish(t *testing.T, jobID string, taskID string, report string, failed bool) string {
	t.Helper()
	reportID, err := holding.jobs.FinishTask(context.Background(), jobID, taskID, report, failed)
	if err != nil {
		t.Fatalf("cannot finish the task %s of job %s: %v", taskID, jobID, err)
	}
	return reportID
}

// nextTask asks for the next task that may start and fails the test on an error.
func (holding *opened) nextTask(t *testing.T, now time.Time) (contract.TaskToRun, bool) {
	t.Helper()
	next, due, err := holding.jobs.NextTask(context.Background(), now)
	if err != nil {
		t.Fatalf("cannot ask for the next task: %v", err)
	}
	return next, due
}
