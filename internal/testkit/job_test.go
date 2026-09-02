package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
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

func TestTheFakeJobStoreKeepsTheJobContract(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err := testkit.CheckJob(context.Background(), jobs); err != nil {
		t.Fatalf("the fake job store does not keep the job contract: %v", err)
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
