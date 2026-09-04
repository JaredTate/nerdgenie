package job_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/job"
)

func TestThreeFailuresInARowPauseAPlainJobAndSayWhy(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task that keeps failing", time.Time{})

	for round := 1; round <= job.FailuresThatPause; round++ {
		holding.finish(t, jobID, taskID, "the site answered 500", true)
		state := holding.summaryOf(t, jobID).State
		if round < job.FailuresThatPause && state != contract.JobRunning {
			t.Fatalf("the job is %q after %d failures, and only %d in a row pause it", state, round, job.FailuresThatPause)
		}
	}

	if state := holding.summaryOf(t, jobID).State; state != contract.JobPaused {
		t.Errorf("after three failures in a row the job is %q, want %q", state, contract.JobPaused)
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Lessons.Failures) != 1 || !strings.Contains(held.Lessons.Failures[0].Text, "paused") {
		t.Errorf("the job does not say why it was paused: %+v", held.Lessons.Failures)
	}
	if held.Header.Status != contract.StatusWaiting {
		t.Errorf("the paused job's record reads %q, want %q", held.Header.Status, contract.StatusWaiting)
	}
	if !held.Work.Tasks[0].Done {
		if summary := holding.summaryOf(t, jobID); summary.TasksDone != 0 {
			t.Errorf("a task that only failed counts as %d done", summary.TasksDone)
		}
	}
}

func TestTenFailuresInARowSwitchAScheduledJobOffAndSayWhy(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	taskID := holding.aTask(t, jobID, "the task that keeps failing", time.Time{})

	for round := 1; round <= job.FailuresThatSwitchOff; round++ {
		holding.finish(t, jobID, taskID, "the site answered 500", true)
		state := holding.summaryOf(t, jobID).State
		if round < job.FailuresThatSwitchOff && state != contract.JobRunning {
			t.Fatalf("a scheduled job is %q after %d failures, and only %d in a row switch it off", state, round, job.FailuresThatSwitchOff)
		}
	}

	if state := holding.summaryOf(t, jobID).State; state != contract.JobOff {
		t.Errorf("after ten failures in a row the scheduled job is %q, want %q", state, contract.JobOff)
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Lessons.Failures) != 1 || !strings.Contains(held.Lessons.Failures[0].Text, "switched off") {
		t.Errorf("the job does not say why it was switched off: %+v", held.Lessons.Failures)
	}
	if held.Header.Status != contract.StatusStopped {
		t.Errorf("the switched-off job's record reads %q, want %q", held.Header.Status, contract.StatusStopped)
	}
}

// TestAJobToldToKeepRunningIsNeverStoppedForFailing is the nightly self-check's
// night after night. Its task fails whenever it finds a broken skill, which is
// the check working rather than the check breaking, so ten such nights must not
// switch the check off and leave the user hearing nothing.
func TestAJobToldToKeepRunningIsNeverStoppedForFailing(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aScheduledJob(t, "Check yourself every night: ask the memory twenty questions and run every skill's dry run.",
		contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 4 * * *"})
	taskID := holding.aTask(t, jobID, "Run the nightly self-check.", time.Time{})
	if err := holding.jobs.KeepRunningWhenItsTasksFail(ctx, jobID); err != nil {
		t.Fatalf("cannot tell job %s to keep running: %v", jobID, err)
	}

	for night := 1; night <= 12; night++ {
		holding.finish(t, jobID, taskID, "nightly self-check: 0 passed, 1 failed; these failed: the skill post-the-update", true)
		if state := holding.summaryOf(t, jobID).State; state != contract.JobRunning {
			t.Fatalf("after %d nights of correctly reporting a broken skill the job is %q, and a check that switches itself off goes quiet exactly when it is working",
				night, state)
		}
	}
	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 12 {
		t.Errorf("twelve failed nights were counted as %d, and a job that keeps running still counts what went wrong", failures)
	}
}

// TestAJobThatStopsForFailingTellsTheUser holds brief 4.4 to its word: three
// failures pause the job with a message and ten switch it off with a message. A
// failure written into the job's own record is invisible until somebody thinks
// to type /jobs, which is exactly what nobody does when a job has quietly
// stopped.
func TestAJobThatStopsForFailingTellsTheUser(t *testing.T) {
	holding := newJobs(t)
	said := []string{}
	holding.jobs.TellTheUser(func(_ context.Context, text string) error {
		said = append(said, text)
		return nil
	})
	plain := holding.aJob(t, "Do a long thing.")
	plainTask := holding.aTask(t, plain, "the task that keeps failing", time.Time{})
	scheduled := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	scheduledTask := holding.aTask(t, scheduled, "the tick that keeps failing", time.Time{})

	for range job.FailuresThatPause {
		holding.finish(t, plain, plainTask, "the site answered 500", true)
	}
	for range job.FailuresThatSwitchOff {
		holding.finish(t, scheduled, scheduledTask, "the site answered 500", true)
	}

	if len(said) != 2 {
		t.Fatalf("the user was sent %d messages, want the one for the paused job and the one for the job switched off: %q", len(said), said)
	}
	for _, want := range []string{"job " + plain, "paused", "the site answered 500", "/jobs " + plain} {
		if !strings.Contains(said[0], want) {
			t.Errorf("the message about the paused job is %q, and it does not say %q", said[0], want)
		}
	}
	for _, want := range []string{"job " + scheduled, "switched off", "/cron run " + scheduled} {
		if !strings.Contains(said[1], want) {
			t.Errorf("the message about the job switched off is %q, and it does not say %q", said[1], want)
		}
	}
}

func TestResumeStartsAPausedJobAgainAndForgetsTheFailuresBehindIt(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task that keeps failing", time.Time{})
	for range job.FailuresThatPause {
		holding.finish(t, jobID, taskID, "the site answered 500", true)
	}

	if err := holding.jobs.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot resume the job: %v", err)
	}

	summary := holding.summaryOf(t, jobID)
	if summary.State != contract.JobRunning {
		t.Errorf("the resumed job is %q, want %q", summary.State, contract.JobRunning)
	}
	if summary.FailuresInARow != 0 {
		t.Errorf("the resumed job still counts %d failures in a row, and the next one should start the count afresh", summary.FailuresInARow)
	}
	if _, due := holding.nextTask(t, theEpoch()); !due {
		t.Error("the resumed job handed out no work")
	}
	if err := holding.jobs.Resume(ctx, "99"); err == nil {
		t.Error("a job that is not there was resumed without an error naming it")
	}
}

func TestAFailedTickIsOverAndTheNextOneRuns(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	first, due := holding.nextTask(t, theEpoch().Add(time.Hour))
	if !due || !first.Unattended {
		t.Fatalf("the first tick made %+v (due %v), want an unattended task", first, due)
	}

	holding.finish(t, jobID, first.TaskID, "the site was down", true)

	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if !held.Work.Tasks[0].Done {
		t.Error("a tick that failed is still waiting to be tried again, and the next tick makes its own task")
	}
	if !strings.HasPrefix(held.Work.Results[0].Summary, "failed:") {
		t.Errorf("the report of a failed tick reads %q, and it must say that it failed", held.Work.Results[0].Summary)
	}
	second, due := holding.nextTask(t, theEpoch().Add(4*time.Hour))
	if !due || second.TaskID == first.TaskID {
		t.Errorf("the next tick made %+v (due %v), want a task of its own", second, due)
	}
}

func TestAJobClosesWhenItsLastTaskIsDoneAndItsDoneListIsProved(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Write the anniversary blog piece.")
	taskID := holding.aTask(t, jobID, "draft the blog piece", time.Time{})
	if err := holding.jobs.Update(ctx, jobID, aDoneList("the blog piece is published", "")); err != nil {
		t.Fatalf("cannot write the job's done list: %v", err)
	}

	reportID := holding.finish(t, jobID, taskID, "the blog piece went up", false)

	if state := holding.summaryOf(t, jobID).State; state != contract.JobRunning {
		t.Errorf("the job closed with a done line that nothing proves, and it is %q", state)
	}
	if err := holding.jobs.Update(ctx, jobID, aDoneList("the blog piece is published", reportID)); err != nil {
		t.Fatalf("cannot prove the job's done line: %v", err)
	}
	second := holding.aTask(t, jobID, "tell the user", time.Time{})
	holding.finish(t, jobID, second, "the user has the summary", false)

	if state := holding.summaryOf(t, jobID).State; state != contract.JobDone {
		t.Errorf("a job whose last task is done and whose done list is proved is %q, want %q", state, contract.JobDone)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if held.Header.Status != contract.StatusDone {
		t.Errorf("the closed job's record reads %q, want %q", held.Header.Status, contract.StatusDone)
	}
}
