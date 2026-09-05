package testkit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A review found four places the fake job store and the real one disagreed
// with no contract step between them. The tests here pin the fake to the rules
// the real store follows; CheckJob holds both stores to the same rules.

func TestTheFakeJobStoreRefusesToPutDownAFinishedTask(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, taskID := aJobWithOneTask(t, jobs, time.Time{})
	if _, due, err := jobs.NextTask(ctx, theEpoch()); err != nil || !due {
		t.Fatalf("the one task was not handed out (due %v, error %v)", due, err)
	}
	if _, err := jobs.FinishTask(ctx, jobID, taskID, "done", false); err != nil {
		t.Fatalf("finishing the task failed: %v", err)
	}

	err := jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: jobID, TaskID: taskID}, Run: "3"})
	if err == nil {
		t.Fatal("a job was put down on a task that had finished, and a finished task cannot be put down")
	}
	if !strings.Contains(err.Error(), taskID) {
		t.Errorf("the refusal reads %q and does not name the task %s", err, taskID)
	}
	if state := summaryOf(t, jobs, jobID).State; state != contract.JobDone {
		t.Errorf("after the refused put-down the job is %q, want %q, because a refused put-down changes nothing", state, contract.JobDone)
	}
}

func TestTheFakeJobStoreRefusesToPutDownAJobThatIsOff(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, taskID := aJobWithOneTask(t, jobs, time.Time{})
	if err := jobs.SwitchOff(ctx, jobID); err != nil {
		t.Fatalf("switching the job off failed: %v", err)
	}

	err := jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: jobID, TaskID: taskID}, Run: "3"})
	if err == nil {
		t.Fatal("a job that is off was put down, and a job that is off cannot be put down")
	}
	if !strings.Contains(err.Error(), jobID) {
		t.Errorf("the refusal reads %q and does not name the job %s", err, jobID)
	}
	if state := summaryOf(t, jobs, jobID).State; state != contract.JobOff {
		t.Errorf("after the refused put-down the job is %q, want %q", state, contract.JobOff)
	}
	if held, there, _ := jobs.PutDownTask(ctx); there {
		t.Errorf("the refused put-down left a mark behind: %+v", held)
	}
}

func TestTheFakeJobStoreReadsPastATaskWhoseDateHasNotCome(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, waiting := aJobWithOneTask(t, jobs, theEpoch().Add(24*time.Hour))
	behind, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task behind the one that waits"})
	if err != nil {
		t.Fatalf("adding the task behind failed: %v", err)
	}

	next, due, err := jobs.NextTask(ctx, theEpoch())
	if err != nil || !due || next.TaskID != behind {
		t.Fatalf("the next task is %+v (due %v, error %v), want %s, because task %s dated tomorrow does not hold up the task behind it", next, due, err, behind, waiting)
	}
	if again, due, _ := jobs.NextTask(ctx, theEpoch()); due {
		t.Errorf("a second ask handed out %+v, want nothing, because the task behind is running and the waiting task's date has not come", again)
	}
}

func TestRunNowPullsAScheduledJobsNextTickToNow(t *testing.T) {
	ctx := context.Background()
	clock := testkit.NewFakeClock(theEpoch())
	jobs := testkit.NewFakeJob(clock)
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every hour.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "post the hourly update",
	})
	if err != nil {
		t.Fatalf("creating the scheduled job failed: %v", err)
	}
	if before := summaryOf(t, jobs, jobID).NextRun; !before.Equal(theEpoch().Add(time.Hour)) {
		t.Fatalf("the first tick is at %v, want an hour on from the clock", before)
	}
	clock.Advance(10 * time.Minute)

	if err := jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("running the scheduled job now failed: %v", err)
	}
	if after := summaryOf(t, jobs, jobID).NextRun; !after.Equal(clock.Now()) {
		t.Errorf("after a run-now the next tick is at %v, want now, %v", after, clock.Now())
	}
	next, due, err := jobs.NextTask(ctx, clock.Now())
	if err != nil || !due || !next.Unattended || next.Text != "post the hourly update" {
		t.Errorf("asking for work now handed out %+v (due %v, error %v), want the template's task, unattended", next, due, err)
	}
}

func TestResumeSetsAPausedJobRunningAndTouchesNoDateAndNoTick(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every hour.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "post the hourly update",
	})
	if err != nil {
		t.Fatalf("creating the scheduled job failed: %v", err)
	}
	running, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task that was running"})
	if err != nil {
		t.Fatalf("adding the undated task failed: %v", err)
	}
	if _, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task that waits", DueAt: theEpoch().Add(24 * time.Hour)}); err != nil {
		t.Fatalf("adding the dated task failed: %v", err)
	}
	if next, due, err := jobs.NextTask(ctx, theEpoch()); err != nil || !due || next.TaskID != running {
		t.Fatalf("the undated task was not handed out first (%+v, due %v, error %v)", next, due, err)
	}
	before := summaryOf(t, jobs, jobID)
	dated := jobs.Tasks(jobID)[1].DueAt
	if err := jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("pausing the job failed: %v", err)
	}

	if err := jobs.Resume(ctx, jobID); err != nil {
		t.Fatalf("resuming the paused job failed: %v", err)
	}
	after := summaryOf(t, jobs, jobID)
	if after.State != contract.JobRunning {
		t.Errorf("after a resume the job is %q, want %q", after.State, contract.JobRunning)
	}
	if !after.NextRun.Equal(before.NextRun) {
		t.Errorf("a resume moved the next tick from %v to %v, and a resume touches no tick", before.NextRun, after.NextRun)
	}
	if kept := jobs.Tasks(jobID)[1].DueAt; kept != dated || kept == "" {
		t.Errorf("a resume changed the dated task's date from %q to %q, and a resume touches no date", dated, kept)
	}
	if next, due, err := jobs.NextTask(ctx, theEpoch()); err != nil || !due || next.TaskID != running {
		t.Errorf("after a resume the next task is %+v (due %v, error %v), want %s again, because a resume lets go of the running marks its tasks held", next, due, err, running)
	}
	if err := jobs.Resume(ctx, "99"); err == nil {
		t.Error("resuming a job that is not there was reported as a success, want an error naming it")
	}
}
