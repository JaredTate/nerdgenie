package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/job"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
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

// TestAJobWhoseLastTaskFinishesClosesOnADoneLinePerTask is the acid test's
// third complaint: no job could ever finish, because closing one ran the
// done-check on a done list nothing lets the model write. When the last task
// finishes and the job's own done list is empty, the harness writes one done
// line per task, each pointing at that task's report, the way it writes the
// one done line of a task whose answer is its own proof; the done-check then
// passes and the job closes. A done list the model did write is left alone.
func TestAJobWhoseLastTaskFinishesClosesOnADoneLinePerTask(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Run the two-part campaign.")
	first := holding.aTask(t, jobID, "post the tweet", time.Time{})
	second := holding.aTask(t, jobID, "write the summary for the user", time.Time{})

	firstReport := holding.finish(t, jobID, first, "the tweet is posted", false)
	if state := holding.summaryOf(t, jobID).State; state != contract.JobRunning {
		t.Fatalf("the job is %q with a task still to run, want %q", state, contract.JobRunning)
	}
	secondReport := holding.finish(t, jobID, second, "the summary is written", false)

	if state := holding.summaryOf(t, jobID).State; state != contract.JobDone {
		t.Errorf("the job is %q after its last task finished, want %q: a job with no done list of its own closes on one line per task", state, contract.JobDone)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if held.Header.Status != contract.StatusDone {
		t.Errorf("the closed job's record reads %q, want %q", held.Header.Status, contract.StatusDone)
	}
	want := []contract.DoneLine{
		{Text: "post the tweet", Done: true, ResultID: firstReport},
		{Text: "write the summary for the user", Done: true, ResultID: secondReport},
	}
	if len(held.Goal.DoneWhen) != len(want) {
		t.Fatalf("the job's done list is %+v, want one line per task: %+v", held.Goal.DoneWhen, want)
	}
	for at, line := range held.Goal.DoneWhen {
		if line != want[at] {
			t.Errorf("done line %d is %+v, want %+v, the task's own words pointing at its report", at+1, line, want[at])
		}
	}
}

// TestAPutDownMarkSurvivesARestartAndIsForgottenWhenTheJobRunsAgain is the
// acid test's sixth complaint: the memory of which task a job was put down on
// lived in the loop's process, so after a restart "continue" found nothing.
// The mark lives in the store now, beside the job's state, and a restart
// reads it back; a job set running again forgets it, because the task is
// picked up and put down no longer.
func TestAPutDownMarkSurvivesARestartAndIsForgottenWhenTheJobRunsAgain(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task the person stopped", time.Time{})
	mark := contract.PutDownMark{
		Task: contract.TaskToRun{JobID: jobID, TaskID: taskID, Text: "the task the person stopped"},
		Run:  "4", HasRecord: true, Waiting: true,
	}
	if err := holding.jobs.PutDown(ctx, mark); err != nil {
		t.Fatalf("cannot put the job down on its task: %v", err)
	}
	if state := holding.summaryOf(t, jobID).State; state != contract.JobPaused {
		t.Fatalf("a job put down on its task is %q, want %q", state, contract.JobPaused)
	}

	after := holding.restart(t)

	held, there, err := after.jobs.PutDownTask(ctx)
	if err != nil {
		t.Fatalf("cannot ask the restarted store for the put-down task: %v", err)
	}
	if !there || held != mark {
		t.Errorf("after the restart the put-down task is %+v (there %v), want the mark written before it: %+v", held, there, mark)
	}
	if record, err := after.jobs.Load(ctx, jobID); err != nil || record.Header.Status != contract.StatusWaiting {
		t.Errorf("the put-down job's record stands at %q (error %v), want %q, which is what /jobs shows for a job holding on a task", record.Header.Status, err, contract.StatusWaiting)
	}
	if _, due := after.nextTask(t, theEpoch()); due {
		t.Error("the restarted store handed the put-down task out on its own, and nothing of a put-down job runs until the person picks it up")
	}
	if err := after.jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot set the job running again: %v", err)
	}
	if held, there, err := after.jobs.PutDownTask(ctx); err != nil || there {
		t.Errorf("after the job was set running again the put-down task is %+v (there %v, error %v), want none", held, there, err)
	}
	if next, due := after.nextTask(t, theEpoch()); !due || next.TaskID != taskID {
		t.Errorf("once the job runs again the next task is %+v (due %v), want %s", next, due, taskID)
	}
}

// TestTheNewestPutDownTaskIsTheOneHandedBack pins which mark comes back when
// two jobs are put down: the one whose run is newest, because that is the
// task the person was just looking at.
func TestTheNewestPutDownTaskIsTheOneHandedBack(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	older := holding.aJob(t, "The job put down first.")
	olderTask := holding.aTask(t, older, "its task", time.Time{})
	newer := holding.aJob(t, "The job put down second.")
	newerTask := holding.aTask(t, newer, "its task", time.Time{})
	for _, put := range []contract.PutDownMark{
		{Task: contract.TaskToRun{JobID: newer, TaskID: newerTask}, Run: "12"},
		{Task: contract.TaskToRun{JobID: older, TaskID: olderTask}, Run: "9"},
	} {
		if err := holding.jobs.PutDown(ctx, put); err != nil {
			t.Fatalf("cannot put job %s down: %v", put.Task.JobID, err)
		}
	}

	held, there, err := holding.jobs.PutDownTask(ctx)

	if err != nil || !there || held.Task.JobID != newer {
		t.Errorf("the put-down task handed back is %+v (there %v, error %v), want the one of job %s, whose run is newest", held, there, err, newer)
	}
	if err := holding.jobs.SwitchOff(ctx, newer); err != nil {
		t.Fatalf("cannot switch the newer job off: %v", err)
	}
	if held, there, _ := holding.jobs.PutDownTask(ctx); !there || held.Task.JobID != older {
		t.Errorf("with the newer job switched off the put-down task is %+v (there %v), want the older job's, because a job switched off is not picked up", held, there)
	}
}

// TestPutDownRefusesWhatItCannotMark pins the two refusals: a job that is not
// there, and a task that is not on the job's list.
func TestPutDownRefusesWhatItCannotMark(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	holding.aTask(t, jobID, "the one task", time.Time{})

	if err := holding.jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: "99", TaskID: "t1"}}); err == nil {
		t.Error("a job that is not there was put down without an error naming it")
	}
	if err := holding.jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: jobID, TaskID: "t99"}}); err == nil {
		t.Error("a job was put down on a task that is not on its list without an error naming it")
	}
	if _, there, err := holding.jobs.PutDownTask(ctx); err != nil || there {
		t.Errorf("a refused put-down left a mark behind (there %v, error %v)", there, err)
	}
}

func TestTheJobStoreKeepsTheJobContractForAJobWithoutAName(t *testing.T) {
	holding := newJobs(t)
	if err := testkit.CheckJobWithoutAName(t.Context(), holding.jobs); err != nil {
		t.Fatalf("the job store does not keep the job contract for a job without a name: %v", err)
	}
}

func TestTheJobStoreKeepsTheTimingContract(t *testing.T) {
	holding := newJobs(t)
	if err := testkit.CheckJobTiming(t.Context(), holding.jobs, holding.clock); err != nil {
		t.Fatalf("the job store does not keep the timing contract: %v", err)
	}
}
