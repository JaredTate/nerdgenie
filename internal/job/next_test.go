package job_test

import (
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/job"
)

func TestTheReportLandsInTheJobAndTheNextTaskIsHandedOut(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Run the campaign this month.")
	first := holding.aTask(t, jobID, "post the anniversary tweet", time.Time{})
	second := holding.aTask(t, jobID, "draft the blog piece", time.Time{})

	next, due := holding.nextTask(t, theEpoch())
	if !due || next.JobID != jobID || next.TaskID != first || next.Unattended {
		t.Fatalf("the first task handed out is %+v (due %v), want task %s of job %s, attended", next, due, first, jobID)
	}
	next, due = holding.nextTask(t, theEpoch())
	if !due || next.TaskID != second {
		t.Fatalf("the second call handed out %+v (due %v), and a task somebody is already running is nobody else's to start", next, due)
	}
	if _, due := holding.nextTask(t, theEpoch()); due {
		t.Error("a third task was handed out, and the job holds only two")
	}

	reportID := holding.finish(t, jobID, first, "posted, 236 characters, link saved", false)
	if reportID != contract.ReportID(jobID, 1) {
		t.Errorf("the report identifier is %q, want %q", reportID, contract.ReportID(jobID, 1))
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if !held.Work.Tasks[0].Done || held.Work.Tasks[0].ReportID != reportID {
		t.Errorf("the finished task is %+v, want it done with report %s behind it", held.Work.Tasks[0], reportID)
	}
	if _, err := holding.jobs.FinishTask(t.Context(), jobID, first, "posted again", false); err == nil {
		t.Error("a finished task took a second report, and its report is written once")
	}
	if _, err := holding.jobs.FinishTask(t.Context(), jobID, "t99", "nothing", false); err == nil {
		t.Error("a task that is not on the list took a report")
	}
}

func TestATaskWithADateWaitsForIt(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Post on the anniversary itself.")
	taskID := holding.aTask(t, jobID, "post for day three", theEpoch().Add(24*time.Hour))

	if _, due := holding.nextTask(t, theEpoch()); due {
		t.Fatal("a task dated tomorrow was handed out today")
	}
	next, due := holding.nextTask(t, theEpoch().Add(25*time.Hour))
	if !due || next.TaskID != taskID {
		t.Errorf("a day later the task is %+v (due %v), want task %s", next, due, taskID)
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if held.Work.Tasks[0].DueAt == "" {
		t.Errorf("the task's date is not written into the record: %+v", held.Work.Tasks[0])
	}
}

func TestATaskDatedNextWeekDoesNotHoldUpTheOneBehindIt(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	holding.aTask(t, jobID, "the task for next week", theEpoch().Add(7*24*time.Hour))
	soon := holding.aTask(t, jobID, "the task that can start at once", time.Time{})

	next, due := holding.nextTask(t, theEpoch())

	if !due || next.TaskID != soon {
		t.Errorf("the task handed out is %+v (due %v), want %s, because a task dated next week waits on its own", next, due, soon)
	}
}

func TestOnlyARunningJobHandsOutWork(t *testing.T) {
	ctx := t.Context()
	for _, stopping := range []struct {
		name  string
		stop  func(*opened, string) error
		state contract.JobState
	}{
		{"paused", func(holding *opened, jobID string) error { return holding.jobs.Pause(ctx, jobID) }, contract.JobPaused},
		{"switched off", func(holding *opened, jobID string) error { return holding.jobs.SwitchOff(ctx, jobID) }, contract.JobOff},
	} {
		t.Run(stopping.name, func(t *testing.T) {
			holding := newJobs(t)
			jobID := holding.aJob(t, "Do a long thing.")
			holding.aTask(t, jobID, "the one task", time.Time{})

			if err := stopping.stop(holding, jobID); err != nil {
				t.Fatalf("cannot stop the job: %v", err)
			}

			if summary := holding.summaryOf(t, jobID); summary.State != stopping.state {
				t.Errorf("the job is %q, want %q", summary.State, stopping.state)
			}
			if next, due := holding.nextTask(t, theEpoch()); due {
				t.Errorf("a job that is %s handed out %+v", stopping.name, next)
			}
		})
	}
}

func TestRunNowTakesTheDateOffTheNextTask(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the one task", theEpoch().Add(24*time.Hour))
	if err := holding.jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job: %v", err)
	}

	if err := holding.jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot run the job now: %v", err)
	}

	next, due := holding.nextTask(t, theEpoch())
	if !due || next.TaskID != taskID {
		t.Errorf("after run now the next task is %+v (due %v), and run now means start it without waiting", next, due)
	}
	if err := holding.jobs.RunNow(ctx, "99"); err == nil {
		t.Error("a job that is not there was run now without an error naming it")
	}
}

func TestATaskPastItsBudgetIsGivenUpAndCountedAsAFailure(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task that never comes back", time.Time{})
	if _, due := holding.nextTask(t, theEpoch()); !due {
		t.Fatal("the task was not handed out to begin with")
	}

	next, due := holding.nextTask(t, theEpoch().Add(job.TaskBudget+time.Minute))

	if !due || next.TaskID != taskID {
		t.Errorf("after the budget ran out the task is %+v (due %v), want %s offered again", next, due, taskID)
	}
	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 1 {
		t.Errorf("giving up a task past its budget counted %d failures, want 1", failures)
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 1 {
		t.Errorf("the job holds %d reports, want the one saying the task was given up", len(held.Work.Results))
	}
}

func TestWaitSleepsUntilThereCouldBeWorkAndNeverLongerThanTheClamp(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	holding.aTask(t, jobID, "the task for next week", theEpoch().Add(7*24*time.Hour))

	waiting := make(chan error, 1)
	go func() { waiting <- holding.jobs.Wait(t.Context()) }()
	waitForSleeper(t, holding)
	holding.clock.Advance(job.TimerClamp)

	if err := <-waiting; err != nil {
		t.Fatalf("waiting for work failed: %v", err)
	}

	holding.aTask(t, jobID, "the task for half a minute from now", theEpoch().Add(90*time.Second))
	go func() { waiting <- holding.jobs.Wait(t.Context()) }()
	waitForSleeper(t, holding)
	holding.clock.Advance(30 * time.Second)
	if err := <-waiting; err != nil {
		t.Fatalf("waiting for the nearer task failed: %v", err)
	}
}

// waitForSleeper waits until the store is really asleep, so that the clock is
// never moved before there is anybody to wake.
func waitForSleeper(t *testing.T, holding *opened) {
	t.Helper()
	for range 1000 {
		if holding.clock.Sleepers() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the job store never went to sleep waiting for work")
}
