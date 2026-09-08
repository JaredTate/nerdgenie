package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheFakeJobStoreCreatesAJobAndCountsItsTasks(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))

	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask: "Run the DigiByte anniversary campaign this month.",
		Why: "keep the anniversary in front of people all month",
	})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}

	for _, text := range []string{"post the anniversary tweet", "draft the blog piece"} {
		if _, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatalf("adding the task %q failed: %v", text, err)
		}
	}

	listed, err := jobs.List(ctx)
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("there are %d jobs, want 1", len(listed))
	}
	if listed[0].TasksTotal != 2 || listed[0].TasksDone != 0 {
		t.Errorf("the progress line says %d of %d tasks done, want 0 of 2", listed[0].TasksDone, listed[0].TasksTotal)
	}
	if listed[0].State != contract.JobRunning {
		t.Errorf("a new job is %q, want %q", listed[0].State, contract.JobRunning)
	}
}

func TestTheFakeJobStoreGivesTasksTheIdentifiersTheDesignShows(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do a long thing."})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}

	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the first task"})
	if err != nil {
		t.Fatalf("adding a task failed: %v", err)
	}

	if _, valid := contract.ParseTaskID(taskID); !valid {
		t.Errorf("the task identifier is %q, want the shape the design shows, such as t31", taskID)
	}
}

func TestTheFakeJobStoreCarriesASchedule(t *testing.T) {
	ctx := context.Background()
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	jobs := testkit.NewFakeJob(clock)

	_, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every weekday at seven in the morning.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 7 * * 1-5", Timezone: "America/New_York"},
		TaskTemplate: "write and post today's message",
	})
	if err != nil {
		t.Fatalf("creating a scheduled job failed: %v", err)
	}

	listed, err := jobs.List(ctx)
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	if listed[0].NextRun.IsZero() {
		t.Error("a scheduled job has no next run, and the cron listing needs one")
	}
}

func TestTheFakeJobStorePausesRunsAndSwitchesOff(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do a long thing."})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}
	if _, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the first task"}); err != nil {
		t.Fatalf("adding a task failed: %v", err)
	}

	if err := jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("pausing failed: %v", err)
	}
	if state := stateOf(t, jobs, jobID); state != contract.JobPaused {
		t.Errorf("after pausing, the job is %q, want %q", state, contract.JobPaused)
	}

	if err := jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("running now failed: %v", err)
	}
	if state := stateOf(t, jobs, jobID); state != contract.JobRunning {
		t.Errorf("after running now, the job is %q, want %q", state, contract.JobRunning)
	}

	if err := jobs.SwitchOff(ctx, jobID); err != nil {
		t.Fatalf("switching off failed: %v", err)
	}
	if state := stateOf(t, jobs, jobID); state != contract.JobOff {
		t.Errorf("after switching off, the job is %q, want %q", state, contract.JobOff)
	}
}

func TestTheFakeJobStoreRefusesAJobItDoesNotHold(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))

	if err := jobs.Pause(ctx, "99"); err == nil {
		t.Error("pausing a job that is not there was reported as a success, want an error naming it")
	}
	if _, err := jobs.AddTask(ctx, contract.NewTask{JobID: "99", Text: "nothing"}); err == nil {
		t.Error("adding a task to a job that is not there was reported as a success, want an error naming it")
	}
}

func TestTheFakeJobStoreHandsBackTheNewestPutDownTaskAndRefusesWhatItCannotMark(t *testing.T) {
	ctx := context.Background()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	older, err := jobs.Create(ctx, contract.NewJob{Ask: "The job put down first."})
	if err != nil {
		t.Fatalf("creating the older job failed: %v", err)
	}
	olderTask, err := jobs.AddTask(ctx, contract.NewTask{JobID: older, Text: "its task"})
	if err != nil {
		t.Fatalf("adding the older job's task failed: %v", err)
	}
	newer, err := jobs.Create(ctx, contract.NewJob{Ask: "The job put down second."})
	if err != nil {
		t.Fatalf("creating the newer job failed: %v", err)
	}
	newerTask, err := jobs.AddTask(ctx, contract.NewTask{JobID: newer, Text: "its task"})
	if err != nil {
		t.Fatalf("adding the newer job's task failed: %v", err)
	}
	if listed := jobs.Tasks(newer); len(listed) != 1 || listed[0].TaskID != newerTask {
		t.Fatalf("the newer job's tasks are %+v, want the one that was added", listed)
	}

	if err := jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: "99", TaskID: "t1"}}); err == nil {
		t.Error("a job that is not there was put down without an error naming it")
	}
	if err := jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: older, TaskID: "t99"}}); err == nil {
		t.Error("a job was put down on a task that is not on its list without an error naming it")
	}
	for _, mark := range []contract.PutDownMark{
		{Task: contract.TaskToRun{JobID: newer, TaskID: newerTask}, Run: "12"},
		{Task: contract.TaskToRun{JobID: older, TaskID: olderTask}, Run: "9"},
	} {
		if err := jobs.PutDown(ctx, mark); err != nil {
			t.Fatalf("putting job %s down failed: %v", mark.Task.JobID, err)
		}
	}

	held, there, err := jobs.PutDownTask(ctx)
	if err != nil || !there || held.Task.JobID != newer {
		t.Errorf("the put-down task handed back is %+v (there %v, error %v), want the newer job's, whose run is newest", held, there, err)
	}
	if err := jobs.SwitchOff(ctx, newer); err != nil {
		t.Fatalf("switching the newer job off failed: %v", err)
	}
	if held, there, _ := jobs.PutDownTask(ctx); !there || held.Task.JobID != older {
		t.Errorf("with the newer job switched off the put-down task is %+v (there %v), want the older job's", held, there)
	}
	if err := jobs.RunNow(ctx, older); err != nil {
		t.Fatalf("running the older job now failed: %v", err)
	}
	if held, there, _ := jobs.PutDownTask(ctx); there {
		t.Errorf("with both jobs picked up or switched off the put-down task is %+v, want none", held)
	}
}

func TestTheFakeJobStoreKeepsTheJobContract(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err := testkit.CheckJob(context.Background(), jobs); err != nil {
		t.Fatalf("the fake job store does not keep the job contract: %v", err)
	}
}

func TestTheFakeJobStoreKeepsTheJobContractForAJobWithoutAName(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err := testkit.CheckJobWithoutAName(context.Background(), jobs); err != nil {
		t.Fatalf("the fake job store does not keep the job contract for a job without a name: %v", err)
	}
}

// stateOf reads one job's state out of the listing.
func stateOf(t *testing.T, jobs contract.Job, jobID string) contract.JobState {
	t.Helper()
	listed, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	for _, job := range listed {
		if job.ID == jobID {
			return job.State
		}
	}
	t.Fatalf("the job %s is not in the listing", jobID)
	return ""
}

func TestTheFakeJobStoreHandsOutTheNextDueTaskAndTakesItsReport(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	clock := testkit.NewFakeClock(start)
	jobs := testkit.NewFakeJob(clock)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Run the campaign this month.", Why: "keep it in front of people"})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}
	firstID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "post the anniversary tweet"})
	if err != nil {
		t.Fatalf("adding the first task failed: %v", err)
	}
	secondID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "post for day two", DueAt: start.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("adding the second task failed: %v", err)
	}

	next, due, err := jobs.NextTask(ctx, start)
	if err != nil || !due {
		t.Fatalf("the first task should be due at once, got due=%v err=%v", due, err)
	}
	if next.JobID != jobID || next.TaskID != firstID || next.Unattended {
		t.Errorf("the next task is %+v, want the first task of job %s, attended", next, jobID)
	}

	reportID, err := jobs.FinishTask(ctx, jobID, firstID, "posted, 236 characters, link saved", false)
	if err != nil {
		t.Fatalf("finishing the first task failed: %v", err)
	}
	if reportID != contract.ReportID(jobID, 1) {
		t.Errorf("the report id is %q, want %q", reportID, contract.ReportID(jobID, 1))
	}

	if _, due, err := jobs.NextTask(ctx, start); err != nil || due {
		t.Errorf("the second task is due tomorrow, but NextTask said due=%v err=%v now", due, err)
	}
	next, due, err = jobs.NextTask(ctx, start.Add(25*time.Hour))
	if err != nil || !due || next.TaskID != secondID {
		t.Errorf("after a day the second task should be due, got %+v due=%v err=%v", next, due, err)
	}

	record, err := jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("loading the job record failed: %v", err)
	}
	if record.Header.Kind != contract.RecordJob || record.Header.TasksDone != 1 || record.Header.TasksTotal != 2 {
		t.Errorf("the job header is %+v, want a job with 1 of 2 tasks done", record.Header)
	}
	if record.Goal.Ask != "Run the campaign this month." {
		t.Errorf("the job record's ask is %q, want the user's words unchanged", record.Goal.Ask)
	}
	if len(record.Work.Tasks) != 2 || !record.Work.Tasks[0].Done || record.Work.Tasks[0].ReportID != reportID {
		t.Errorf("the task list is %+v, want the first task done with report %s", record.Work.Tasks, reportID)
	}
	if len(record.Work.Results) != 1 || record.Work.Results[0].ID != reportID {
		t.Errorf("the reports are %+v, want one line with id %s", record.Work.Results, reportID)
	}
}

func TestTheFakeJobStorePausesAfterThreeFailuresAndSwitchesAScheduledJobOffAfterTen(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(start))

	plainID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do a long thing."})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}
	for round := 0; round < 3; round++ {
		taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: plainID, Text: "a task that fails"})
		if err != nil {
			t.Fatalf("adding a task failed: %v", err)
		}
		if _, err := jobs.FinishTask(ctx, plainID, taskID, "it failed", true); err != nil {
			t.Fatalf("finishing a failed task failed: %v", err)
		}
	}
	if state := stateOf(t, jobs, plainID); state != contract.JobPaused {
		t.Errorf("after three failures in a row the job is %q, want %q", state, contract.JobPaused)
	}

	scheduledID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every day.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "post today's message",
	})
	if err != nil {
		t.Fatalf("creating a scheduled job failed: %v", err)
	}
	for round := 0; round < 10; round++ {
		taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: scheduledID, Text: "a task that fails"})
		if err != nil {
			t.Fatalf("adding a task failed: %v", err)
		}
		if _, err := jobs.FinishTask(ctx, scheduledID, taskID, "it failed", true); err != nil {
			t.Fatalf("finishing a failed task failed: %v", err)
		}
	}
	if state := stateOf(t, jobs, scheduledID); state != contract.JobOff {
		t.Errorf("after ten failures in a row the scheduled job is %q, want %q", state, contract.JobOff)
	}
}

func TestTheFakeJobStoreMakesOneUnattendedTaskPerTickOfASchedule(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(start))
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post every hour.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "post this hour's message",
	})
	if err != nil {
		t.Fatalf("creating a scheduled job failed: %v", err)
	}

	if _, due, err := jobs.NextTask(ctx, start); err != nil || due {
		t.Fatalf("nothing is due before the first tick, got due=%v err=%v", due, err)
	}
	next, due, err := jobs.NextTask(ctx, start.Add(time.Hour))
	if err != nil || !due {
		t.Fatalf("the first tick should make a task, got due=%v err=%v", due, err)
	}
	if next.JobID != jobID || next.Text != "post this hour's message" || !next.Unattended {
		t.Errorf("the tick's task is %+v, want the template text, unattended", next)
	}
	if _, due, err := jobs.NextTask(ctx, start.Add(time.Hour)); err != nil || due {
		t.Errorf("the same tick handed out a second task, got due=%v err=%v", due, err)
	}
	if _, err := jobs.FinishTask(ctx, jobID, next.TaskID, "posted", false); err != nil {
		t.Fatalf("finishing the tick's task failed: %v", err)
	}
	if _, due, err := jobs.NextTask(ctx, start.Add(2*time.Hour)); err != nil || !due {
		t.Errorf("the second tick should make a task, got due=%v err=%v", due, err)
	}
	if _, err := jobs.Load(ctx, "99"); err == nil {
		t.Error("loading a job that is not there returned no error, want one naming it")
	}
}

func TestTheFakeJobStoreKeepsTheTimingContract(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(1700000000, 0).UTC())
	if err := testkit.CheckJobTiming(context.Background(), testkit.NewFakeJob(clock), clock); err != nil {
		t.Fatalf("the fake job store does not keep the timing contract: %v", err)
	}
}
