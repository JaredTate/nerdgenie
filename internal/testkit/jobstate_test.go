package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theEpoch is the moment every job test starts from.
func theEpoch() time.Time {
	return time.Unix(0, 0).UTC()
}

// aJobWithOneTask makes a job holding one task, due when the caller says.
func aJobWithOneTask(t *testing.T, jobs *testkit.FakeJob, dueAt time.Time) (string, string) {
	t.Helper()
	ctx := context.Background()
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do a long thing.", Why: "because the user asked"})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the one task", DueAt: dueAt})
	if err != nil {
		t.Fatalf("adding a task failed: %v", err)
	}
	return jobID, taskID
}

// summaryOf reads one job's summary out of the listing.
func summaryOf(t *testing.T, jobs *testkit.FakeJob, jobID string) contract.JobSummary {
	t.Helper()
	listed, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary
		}
	}
	t.Fatalf("the job %s is not in the listing", jobID)
	return contract.JobSummary{}
}

func TestRunNowClearsTheDateSoTheNextTaskCanStart(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, taskID := aJobWithOneTask(t, jobs, theEpoch().Add(24*time.Hour))

	if _, due, err := jobs.NextTask(ctx, theEpoch()); err != nil || due {
		t.Fatalf("a task dated tomorrow was handed out today (due %v, error %v)", due, err)
	}
	if err := jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("running the job now failed: %v", err)
	}

	next, due, err := jobs.NextTask(ctx, theEpoch())

	if err != nil {
		t.Fatalf("asking for the next task failed: %v", err)
	}
	if !due || next.TaskID != taskID {
		t.Errorf("after run now the next task is %+v (due %v), and run now means start it without waiting", next, due)
	}
}

func TestLastRunIsWhenATaskLastFinishedAndNothingElse(t *testing.T) {
	ctx := context.Background()
	clock := testkit.NewFakeClock(theEpoch())
	jobs := testkit.NewFakeJob(clock)
	jobID, taskID := aJobWithOneTask(t, jobs, time.Time{})

	if err := jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("pausing the job failed: %v", err)
	}
	if stamped := summaryOf(t, jobs, jobID).LastRun; !stamped.IsZero() {
		t.Errorf("pausing stamped LastRun as %s, and LastRun means when a task last finished", stamped)
	}
	if err := jobs.SwitchOff(ctx, jobID); err != nil {
		t.Fatalf("switching the job off failed: %v", err)
	}
	if stamped := summaryOf(t, jobs, jobID).LastRun; !stamped.IsZero() {
		t.Errorf("switching off stamped LastRun as %s, and no task has finished", stamped)
	}

	clock.Advance(time.Hour)
	if _, err := jobs.FinishTask(ctx, jobID, taskID, "the task finished", false); err != nil {
		t.Fatalf("finishing the task failed: %v", err)
	}

	if stamped := summaryOf(t, jobs, jobID).LastRun; !stamped.Equal(theEpoch().Add(time.Hour)) {
		t.Errorf("LastRun is %s, want the moment the task finished", stamped)
	}
}

func TestAJobThatIsNotRunningHandsOutNoTask(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, _ := aJobWithOneTask(t, jobs, time.Time{})

	for _, stop := range []struct {
		name string
		stop func(context.Context, string) error
	}{
		{"paused", jobs.Pause},
		{"switched off", jobs.SwitchOff},
	} {
		t.Run(stop.name, func(t *testing.T) {
			if err := stop.stop(ctx, jobID); err != nil {
				t.Fatalf("stopping the job failed: %v", err)
			}
			next, due, err := jobs.NextTask(ctx, theEpoch())
			if err != nil {
				t.Fatalf("asking for the next task failed: %v", err)
			}
			if due {
				t.Errorf("a job that is %s handed out %+v, and only a running job hands out work", stop.name, next)
			}
		})
	}
}

func TestAPlainJobPausesAtThreeFailuresInARow(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, taskID := aJobWithOneTask(t, jobs, time.Time{})

	for range 3 {
		if _, err := jobs.FinishTask(ctx, jobID, taskID, "it went wrong", true); err != nil {
			t.Fatalf("finishing the task as a failure failed: %v", err)
		}
	}

	if state := summaryOf(t, jobs, jobID).State; state != contract.JobPaused {
		t.Errorf("a job with three failures in a row is %q, want %q", state, contract.JobPaused)
	}
}

func TestAScheduledJobIsSwitchedOffAtTenFailuresRatherThanPausedAtThree(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every day.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "write and post today's message",
	})
	if err != nil {
		t.Fatalf("creating a scheduled job failed: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the one task"})
	if err != nil {
		t.Fatalf("adding a task failed: %v", err)
	}

	for at := range 10 {
		if _, err := jobs.FinishTask(ctx, jobID, taskID, "it went wrong", true); err != nil {
			t.Fatalf("finishing the task as a failure failed: %v", err)
		}
		state := summaryOf(t, jobs, jobID).State
		if at < 9 && state != contract.JobRunning {
			t.Fatalf("a scheduled job is %q after %d failures, and only ten in a row switch it off", state, at+1)
		}
	}

	if state := summaryOf(t, jobs, jobID).State; state != contract.JobOff {
		t.Errorf("a scheduled job with ten failures in a row is %q, want %q", state, contract.JobOff)
	}
}

func TestATickBehindAFutureTaskStillRunsWhenItIsDue(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theEpoch()))
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every hour.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "write and post this hour's message",
	})
	if err != nil {
		t.Fatalf("creating a scheduled job failed: %v", err)
	}
	// A task the user put on the list for next week sits ahead of every tick.
	if _, err := jobs.AddTask(ctx, contract.NewTask{
		JobID: jobID, Text: "the task for next week", DueAt: theEpoch().Add(7 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("adding the future task failed: %v", err)
	}

	next, due, err := jobs.NextTask(ctx, theEpoch().Add(90*time.Minute))

	if err != nil {
		t.Fatalf("asking for the next task failed: %v", err)
	}
	if !due {
		t.Fatal("the schedule ticked an hour ago and nothing was handed out, because the task sat behind one dated next week")
	}
	if next.Text != "write and post this hour's message" {
		t.Errorf("the task handed out is %q, want the one the tick made", next.Text)
	}
	if !next.Unattended {
		t.Error("a task a schedule made is attended, and nobody is there to answer a preview")
	}
}
