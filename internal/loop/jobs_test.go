package loop_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aJobOfTwoTasks puts a job with two tasks into the fake job store.
func aJobOfTwoTasks(t *testing.T, built *harness) string {
	t.Helper()
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{
		Ask: "run the anniversary campaign", Why: "keep the anniversary in front of people",
	})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	for _, text := range []string{"post the anniversary tweet", "write the summary"} {
		if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatalf("cannot add a task to job %s: %v", jobID, err)
		}
	}
	return jobID
}

// MaxTasksARunToTheEndRuns is how many of a job's tasks runTheJobToTheEnd
// will run before it gives up, so that a job whose task is handed out again
// and again cannot hang a test.
const MaxTasksARunToTheEndRuns = 10

// runTheJobToTheEnd asks the loop for the job's due task again and again,
// the way the driver in cmd/nerdgenie does, until nothing is due, and says how
// many tasks ran.
func runTheJobToTheEnd(t *testing.T, made *loop.Loop, where contract.Channel) int {
	t.Helper()
	ran := 0
	for range MaxTasksARunToTheEndRuns {
		more, err := made.RunNextJobTask(t.Context(), where)
		if err != nil {
			t.Fatalf("the loop could not run the job's task: %v", err)
		}
		if !more {
			return ran
		}
		ran++
	}
	t.Fatalf("the job still had a task due after %d ran", MaxTasksARunToTheEndRuns)
	return ran
}

// twoTasksAndTheirReview is the script for a job of two tasks, each of which
// writes its done list, proves it, and reports, and the review of the job.
func twoTasksAndTheirReview() []testkit.Step {
	steps := []testkit.Step{}
	for range 2 {
		steps = append(steps, closingScript("the post is up")...)
	}
	return append(steps, aReviewReply("Keep posting at the same hour every day."))
}

// TestATaskInAJobReportsToTheJobAndStartsTheNext proves the job half of the
// loop: a report goes into the job, the user is told with the job's progress on
// it, and the next task starts on the driver's next ask.
func TestATaskInAJobReportsToTheJobAndStartsTheNext(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 2 {
		t.Fatalf("the loop ran %d tasks, and the job has two", ran)
	}
	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the user was sent %v, want the first report with the job's progress on it", built.channel.Sent())
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done || task.ReportID == "" {
			t.Errorf("the task %s reads %+v, want it done and pointing at its report", task.TaskID, task)
		}
	}
}

// TestTheLastTaskOfAJobClosesItWithItsOwnDoneCheck proves the end of a job:
// when its last task finishes, the store writes one done line per task
// pointing at that task's report, the job's done list is checked the way a
// task's is and passes, the job closes, its review runs, and the user gets the
// final report. Before this the test asserted the opposite, that the job had
// run every task and stayed open on a done list nothing could write.
func TestTheLastTaskOfAJobClosesItWithItsOwnDoneCheck(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	runTheJobToTheEnd(t, built.loop, built.channel)

	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+" is finished: every one of its 2 tasks is done") {
		t.Errorf("the user was sent %v, want the final report of a job whose done list its tasks prove", built.channel.Sent())
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone {
		t.Errorf("the job is %q after its last task, want %q", summary.State, contract.JobDone)
	}
	held, err := built.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Goal.DoneWhen) != 2 {
		t.Fatalf("the job's done list is %+v, want one line per task", held.Goal.DoneWhen)
	}
	for at, line := range held.Goal.DoneWhen {
		if !line.Done || line.ResultID != held.Work.Tasks[at].ReportID {
			t.Errorf("done line %d is %+v, want it proved by the report of task %s", at+1, line, held.Work.Tasks[at].TaskID)
		}
	}
	if len(factsIn(t, built)) != 1 {
		t.Error("the job was not reviewed when its last task finished")
	}
}

// TestAJobWhoseDoneListIsProvenSaysSo proves the other side of the job's
// done-check, on a job whose done list points at its reports.
func TestAJobWhoseDoneListIsProvenSaysSo(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	options := built.options()
	options.Jobs = &jobWithAProvenDoneList{Job: built.jobs}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the job with a proven done list: %v", err)
	}

	runTheJobToTheEnd(t, made, built.channel)

	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+" is finished") {
		t.Errorf("the user was sent %v, want the final report of a job whose done list is proven", built.channel.Sent())
	}
}

// jobWithAProvenDoneList is the fake job store with one thing added: a done list
// whose one line points at the report that proves it, which the fake itself has
// no way to write.
type jobWithAProvenDoneList struct {
	contract.Job
}

// Load returns the job's record with a done list behind it.
func (jobs *jobWithAProvenDoneList) Load(ctx context.Context, jobID string) (contract.Record, error) {
	held, err := jobs.Job.Load(ctx, jobID)
	if err != nil {
		return contract.Record{}, err
	}
	if len(held.Work.Results) == 0 {
		return held, nil
	}
	held.Goal.DoneWhen = []contract.DoneLine{{
		Text: "every task of the campaign has reported", Done: true, ResultID: held.Work.Results[0].ID,
	}}
	return held, nil
}

// TestAJobTaskCarriesTheJobsSummaryAboveTheRecord proves a task of a job can
// lean on the reports of the tasks before it.
func TestAJobTaskCarriesTheJobsSummaryAboveTheRecord(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	aJobOfTwoTasks(t, built)

	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the job's tasks: %v", err)
	}

	if !strings.Contains(requestsJoined(built.model.Requests()), "run the anniversary campaign") {
		t.Error("the job's summary never reached the model, and a task of a job is told what job it is in")
	}
}

// TestNothingRunsWhenNoJobTaskIsDue proves the loop says so rather than
// inventing work.
func TestNothingRunsWhenNoJobTaskIsDue(t *testing.T) {
	built := newHarness(t, nil)

	ran, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil {
		t.Fatalf("asking for the next due task failed: %v", err)
	}
	if ran {
		t.Error("the loop said it ran a task, and there are no jobs at all")
	}
}
