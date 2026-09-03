package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/job"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAJobRecordRoundTripsAndSurvivesARestart(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Run the DigiByte anniversary campaign this month.")
	first := holding.aTask(t, jobID, "post the anniversary tweet", time.Time{})
	holding.aTask(t, jobID, "post for day three", theEpoch().Add(24*time.Hour))
	reportID := holding.finish(t, jobID, first, "posted, 236 characters, link saved", false)

	after := holding.restart(t)

	held, err := after.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job after the restart: %v", err)
	}
	if held.Header.Kind != contract.RecordJob || held.Header.ID != jobID {
		t.Fatalf("the header after the restart is %+v, want job %s", held.Header, jobID)
	}
	if held.Goal.Ask != "Run the DigiByte anniversary campaign this month." {
		t.Errorf("the ask after the restart is %q, want the user's words unchanged", held.Goal.Ask)
	}
	if held.Header.TasksDone != 1 || held.Header.TasksTotal != 2 {
		t.Errorf("the progress after the restart is %d of %d, want 1 of 2", held.Header.TasksDone, held.Header.TasksTotal)
	}
	if len(held.Work.Tasks) != 2 || !held.Work.Tasks[0].Done || held.Work.Tasks[0].ReportID != reportID {
		t.Errorf("the task list after the restart is %+v, want the first task done with report %s", held.Work.Tasks, reportID)
	}
	if held.Work.Tasks[1].DueAt == "" {
		t.Errorf("the task with a date lost it across the restart: %+v", held.Work.Tasks[1])
	}
	if len(held.Work.Results) != 1 || held.Work.Results[0].ID != reportID {
		t.Errorf("the reports after the restart are %+v, want one line with id %s", held.Work.Results, reportID)
	}
	if summary := after.summaryOf(t, jobID); summary.State != contract.JobRunning || summary.TasksDone != 1 {
		t.Errorf("the summary after the restart is %+v, want a running job with one task done", summary)
	}
}

func TestTheWholeTextOfAReportStaysInTheLog(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Write the anniversary blog piece.")
	taskID := holding.aTask(t, jobID, "draft the blog piece", time.Time{})
	whole := strings.Repeat("the whole report, nine hundred words of it. ", 40)

	reportID := holding.finish(t, jobID, taskID, whole, false)

	after := holding.restart(t)
	keeper, err := record.Load(ctx, after.eventLog, contract.RecordJob, jobID)
	if err != nil {
		t.Fatalf("cannot load the job record straight from the log: %v", err)
	}
	brought, err := keeper.Read(ctx, reportID)
	if err != nil {
		t.Fatalf("cannot read the report %s back out of the log: %v", reportID, err)
	}
	if brought != whole {
		t.Errorf("the report read back is %d bytes, want the whole %d that were written", len(brought), len(whole))
	}
	if summary := keeper.Record().Work.Results[0].Summary; len(summary) >= len(whole) {
		t.Errorf("the record keeps the whole report rather than one line for it: %q", summary)
	}
}

func TestCreateRefusesAJobWithNothingToGoOn(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()

	if _, err := holding.jobs.Create(ctx, contract.NewJob{}); err == nil {
		t.Error("a job with no ask was created, and the ask is the user's own words")
	}
	if _, err := holding.jobs.Create(ctx, contract.NewJob{Ask: "Post every hour.", Schedule: &contract.Schedule{Kind: "sometimes"}}); err == nil {
		t.Error("a job was created with a schedule of a kind that does not exist")
	}
	if _, err := holding.jobs.Create(ctx, contract.NewJob{
		Ask:      "Post every hour.",
		Schedule: &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
	}); err == nil {
		t.Error("a scheduled job was created with no template, and a tick has nothing to make a task from")
	}
	if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: "99", Text: "nothing"}); err == nil {
		t.Error("a task was added to a job that is not there")
	}
	jobID := holding.aJob(t, "Do a long thing.")
	if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID}); err == nil {
		t.Error("a task with no text was added, and a task says what it does")
	}
	if _, err := holding.jobs.Load(ctx, "99"); err == nil {
		t.Error("a job that is not there was loaded without an error naming it")
	}
}

func TestOpenNeedsItsPieces(t *testing.T) {
	ctx := t.Context()
	home := testkit.NewTempHome(t)

	if _, err := job.Open(ctx, home, nil, testkit.NewFakeClock(theEpoch())); err == nil {
		t.Error("the job store opened with no event log to write to")
	}
	eventLog := testkit.NewFakeStore()
	if _, err := job.Open(ctx, home, eventLog, nil); err == nil {
		t.Error("the job store opened with no clock to read the time from")
	}
	if _, err := job.Open(ctx, home, eventLog, testkit.NewFakeClock(theEpoch())); err == nil {
		t.Error("the job store opened on a database file that no event log had made its tables in")
	}
}

func TestTheJobStoreKeepsTheJobContract(t *testing.T) {
	holding := newJobs(t)
	if err := testkit.CheckJob(t.Context(), holding.jobs); err != nil {
		t.Fatalf("the job store does not keep the job contract: %v", err)
	}
}
