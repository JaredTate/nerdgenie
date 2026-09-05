// The tests for what opening the store does with the claims it finds. A claim
// is a row saying some process is running a task; a process that has died
// leaves its rows behind, and until now nothing released them, so a task cut
// off by a restart sat claimed for the hour of its budget before it could run
// again. These tests run in the package, because taking a claim under another
// process's name is not something the store lets anyone outside it do.
package job

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aProcessNumberNoProcessCanHave is above the highest process number Linux
// hands out, which is four million and change, so no process is ever running
// under it and a claim taken in its name is a dead process's.
const aProcessNumberNoProcessCanHave = "process 1073741824"

// theMomentTheseTestsStart is where their clocks start.
var theMomentTheseTestsStart = time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)

// openInThePackage opens an event log and a job store on the home given, on a
// fake clock, and closes both when the test ends.
func openInThePackage(t *testing.T, home contract.Home) *Jobs {
	t.Helper()
	ctx := t.Context()
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log: %v", err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })
	jobs, err := Open(ctx, home, eventLog, testkit.NewFakeClock(theMomentTheseTestsStart))
	if err != nil {
		t.Fatalf("cannot open the job store: %v", err)
	}
	t.Cleanup(func() { _ = jobs.Close() })
	return jobs
}

// aJobWithOneTaskClaimedBy makes a job with one task and takes the claim on
// that task under the owner given, the way the process before this one would
// have, and returns the store, the job, and the task.
func aJobWithOneTaskClaimedBy(t *testing.T, home contract.Home, owner string) (*Jobs, string, string) {
	t.Helper()
	ctx := t.Context()
	jobs := openInThePackage(t, home)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do a long thing.", Why: "because the user asked"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task the old process was running"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	jobs.owner = owner
	won, err := jobs.claim(ctx, jobID, taskID, theMomentTheseTestsStart)
	if err != nil || !won {
		t.Fatalf("cannot take the claim on the task under %q (won %v, error %v)", owner, won, err)
	}
	return jobs, jobID, taskID
}

// TestOpeningTheStoreReleasesTheClaimsOfAProcessThatIsGone is the acid test's
// fifth complaint: a restart in the middle of a job's task left that task
// claimed by the dead process for the hour of its budget. Opening the store
// releases every claim whose process is no longer running, so the task is
// runnable at once.
func TestOpeningTheStoreReleasesTheClaimsOfAProcessThatIsGone(t *testing.T) {
	home := testkit.NewTempHome(t)
	_, jobID, taskID := aJobWithOneTaskClaimedBy(t, home, aProcessNumberNoProcessCanHave)

	reopened := openInThePackage(t, home)

	next, due, err := reopened.NextTask(context.Background(), theMomentTheseTestsStart.Add(time.Second))
	if err != nil {
		t.Fatalf("cannot ask the reopened store for the next task: %v", err)
	}
	if !due || next.JobID != jobID || next.TaskID != taskID {
		t.Errorf("a second after the restart the next task is %+v (due %v), want task %s of job %s, which the dead process's claim must not hold for an hour", next, due, taskID, jobID)
	}
	if failures := summaryInThePackage(t, reopened, jobID).FailuresInARow; failures != 0 {
		t.Errorf("releasing a dead process's claim counted %d failures, want none: the process died, the task did not fail", failures)
	}
}

// TestOpeningTheStoreKeepsTheClaimsOfAProcessStillRunning is the other side:
// a second store opened beside a process that is alive and holding a task, as
// the integration tests do, must not take that task away from it.
func TestOpeningTheStoreKeepsTheClaimsOfAProcessStillRunning(t *testing.T) {
	home := testkit.NewTempHome(t)
	aJobWithOneTaskClaimedBy(t, home, thisProcess())

	reopened := openInThePackage(t, home)

	next, due, err := reopened.NextTask(context.Background(), theMomentTheseTestsStart.Add(time.Second))
	if err != nil {
		t.Fatalf("cannot ask the reopened store for the next task: %v", err)
	}
	if due {
		t.Errorf("the reopened store handed out %+v, and that task is held by a process that is still running", next)
	}
}

// TestOpeningTheStoreLeavesAPutDownJobPausedWhenItReleasesItsClaim is the
// rule from the person's stop: a put-down task's claim is kept while its job
// is paused, on purpose, so that nothing of the job runs. Releasing it on open
// is right, because the old process is dead, but the job must stay paused on
// that task rather than start it, and once the person carries on the task is
// handed out at once rather than an hour later.
func TestOpeningTheStoreLeavesAPutDownJobPausedWhenItReleasesItsClaim(t *testing.T) {
	home := testkit.NewTempHome(t)
	ctx := context.Background()
	jobs, jobID, taskID := aJobWithOneTaskClaimedBy(t, home, aProcessNumberNoProcessCanHave)
	if err := jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job on its put-down task: %v", err)
	}

	reopened := openInThePackage(t, home)

	if state := summaryInThePackage(t, reopened, jobID).State; state != contract.JobPaused {
		t.Fatalf("after the restart the put-down job is %q, want it still %q on its task", state, contract.JobPaused)
	}
	if next, due, err := reopened.NextTask(ctx, theMomentTheseTestsStart.Add(time.Second)); err != nil || due {
		t.Errorf("the reopened store handed out %+v (due %v, error %v), and nothing of a paused job runs on its own", next, due, err)
	}
	if err := reopened.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot set the job running again: %v", err)
	}
	next, due, err := reopened.NextTask(ctx, theMomentTheseTestsStart.Add(2*time.Second))
	if err != nil {
		t.Fatalf("cannot ask for the next task once the job runs again: %v", err)
	}
	if !due || next.TaskID != taskID {
		t.Errorf("once the job runs again the next task is %+v (due %v), want %s at once, not after the hour a dead claim used to take", next, due, taskID)
	}
}

// TestAClaimUnderANameNoProcessAnswersToIsReleasedOnOpen pins what happens to
// an owner that is not a process number at all: nothing can be asked whether
// it lives, so it is treated as gone.
func TestAClaimUnderANameNoProcessAnswersToIsReleasedOnOpen(t *testing.T) {
	home := testkit.NewTempHome(t)
	_, _, taskID := aJobWithOneTaskClaimedBy(t, home, "a name that is not a process")

	reopened := openInThePackage(t, home)

	next, due, err := reopened.NextTask(context.Background(), theMomentTheseTestsStart.Add(time.Second))
	if err != nil {
		t.Fatalf("cannot ask the reopened store for the next task: %v", err)
	}
	if !due || next.TaskID != taskID {
		t.Errorf("the next task is %+v (due %v), want %s, because a claim under a name that is no process is nobody's", next, due, taskID)
	}
}

// summaryInThePackage reads one job's summary out of the listing.
func summaryInThePackage(t *testing.T, jobs *Jobs, jobID string) contract.JobSummary {
	t.Helper()
	listed, err := jobs.List(context.Background())
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
