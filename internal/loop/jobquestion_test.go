// The tests for a job's task that asks the person a question. Small models ask
// constantly, and a question used to be a dead end: the task ended waiting,
// nothing put it down, its claim ran out an hour later and counted as a
// failure. A waiting job task is now put down the way a stopped one is: the
// job holds on it, and the person's answer picks that same task up under the
// same job.
package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theQuestionTheTaskAsks is what the job's first task asks the person.
const theQuestionTheTaskAsks = "Which folder should the summary go in?"

// theAnswerThePersonGives is what the person types back.
const theAnswerThePersonGives = "the docs folder"

// aJobWhoseFirstTaskAsks builds the harness with a job of two tasks over the
// script given, which begins with the first task asking a question.
func aJobWhoseFirstTaskAsks(t *testing.T, steps []testkit.Step) (*harness, string) {
	t.Helper()
	built := newHarness(t, steps, scriptedTool("read", "the notes", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	ran, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil {
		t.Fatalf("the loop could not run the job's task: %v", err)
	}
	if !ran {
		t.Fatal("the loop found nothing to run, and the job has two tasks waiting")
	}
	return built, jobID
}

// theJobHoldsOnItsFirstTask asserts the state a question leaves the job in:
// the task neither done nor failed, no report written, the job paused on it,
// and the person told the question with the job and the task named under it.
func theJobHoldsOnItsFirstTask(t *testing.T, built *harness, jobID string) {
	t.Helper()
	if first := theJobsFirstTask(t, built, jobID); first.Done || first.ReportID != "" {
		t.Errorf("the task that asked reads %+v, want it neither done nor reported", first)
	}
	summary := theSummaryOf(t, built, jobID)
	if summary.State != contract.JobPaused || summary.FailuresInARow != 0 {
		t.Errorf("after the question the job reads %+v, want it paused on the task with no failure counted", summary)
	}
	sent := built.channel.Sent()
	if len(sent) != 1 {
		t.Fatalf("the person was sent %d messages, want the one question with the job named under it: %v", len(sent), sent)
	}
	for _, words := range []string{theQuestionTheTaskAsks, "Job " + jobID, "t1", "answer"} {
		if !strings.Contains(sent[0], words) {
			t.Errorf("the person was sent %q, want it to say %q", sent[0], words)
		}
	}
}

// TestAJobsTaskThatAsksAQuestionHoldsTheJobOnItAndTheAnswerPicksItUp is the
// whole rule: the question puts the job's task down, and the person's next
// message, with no task named, picks that task up under the job, so its
// report lands in the job and the next task starts.
func TestAJobsTaskThatAsksAQuestionHoldsTheJobOnItAndTheAnswerPicksItUp(t *testing.T) {
	built, jobID := aJobWhoseFirstTaskAsks(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep(theQuestionTheTaskAsks),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Ask about the folder first."),
	})
	theJobHoldsOnItsFirstTask(t, built, jobID)
	if standing := built.held(t, "1").Header.Status; standing != contract.StatusWaiting {
		t.Errorf("the task's record stands at %q, want %q", standing, contract.StatusWaiting)
	}

	outcome, err := built.loop.Run(t.Context(), built.task(theAnswerThePersonGives))
	if err != nil {
		t.Fatalf("answering the question failed: %v", err)
	}

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("the answer ended as %+v, want task 1, the one that asked, picked up and finished", outcome)
	}
	if lines := built.recordLines(); !sentSomethingLike(lines, "job "+jobID+" task 1 started · "+theAnswerThePersonGives) {
		t.Errorf("the record lines are %v, want task 1 started again as the job's task on the answer", lines)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), theAnswerThePersonGives) {
		t.Error("the answer never reached the model")
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done || task.ReportID == "" {
			t.Errorf("the task %s reads %+v, want it done and pointing at its report", task.TaskID, task)
		}
	}
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the answered task's report with the job's progress on it", sent)
	}
	if !sentSomethingLike(sent, "Job "+jobID+" is finished") {
		t.Errorf("the person was sent %v, want the next task run and the job closed after it", sent)
	}
}

// TestAQuestionAskedBeforeAnyToolCallStartsTheJobsTaskAfreshWithTheAnswer
// covers the question a small model asks first of all, before any tool call
// has made a record. There is nothing to pick up, so the answer starts the
// task afresh under the job, with the task's own words as the ask and the
// answer put in front of the model right after them.
func TestAQuestionAskedBeforeAnyToolCallStartsTheJobsTaskAfreshWithTheAnswer(t *testing.T) {
	built, jobID := aJobWhoseFirstTaskAsks(t, []testkit.Step{
		answerStep(theQuestionTheTaskAsks),
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Ask about the folder first."),
	})
	theJobHoldsOnItsFirstTask(t, built, jobID)

	outcome, err := built.loop.Run(t.Context(), built.task(theAnswerThePersonGives))
	if err != nil {
		t.Fatalf("answering the question failed: %v", err)
	}

	if outcome.Status != contract.StatusDone {
		t.Errorf("the answer ended as %+v, want the task started afresh and finished", outcome)
	}
	if lines := built.recordLines(); !sentSomethingLike(lines, "job "+jobID+" task 2 started · post the anniversary tweet") {
		t.Errorf("the record lines are %v, want the task started afresh as the job's task with the task's own words", lines)
	}
	asked := requestsJoined(built.model.Requests())
	for _, words := range []string{"post the anniversary tweet", theAnswerThePersonGives} {
		if !strings.Contains(asked, words) {
			t.Errorf("the model was never told %q, and a task started afresh on an answer is told both what to do and what the person said", words)
		}
	}
	if held := built.held(t, "2"); held.Goal.Ask != "post the anniversary tweet" {
		t.Errorf("the fresh task's ask is %q, want the task's own words and not the answer", held.Goal.Ask)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the task reads %+v, want it done after being started afresh on the answer", first)
	}
}

// TestAnAnswerNamedForAnOlderTaskOfThePersonsOwnAnswersTheJobsNewerQuestion
// pins which question an answer answers when two are open: the one asked
// last. The person's own task asks first and the program remembers it, so it
// names that task on the person's next message; the job's task asks after it,
// and that is the question the person is looking at, so the answer goes to
// the job's task and the person's own is left waiting for the message after.
func TestAnAnswerNamedForAnOlderTaskOfThePersonsOwnAnswersTheJobsNewerQuestion(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes for the handle.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which handle should I post from?"),
		callStep("I will read the notes.", callFor("c2", "read", `{"path":"notes.md"}`)),
		answerStep(theQuestionTheTaskAsks),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Ask about the folder first."),
	}, scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	if outcome := built.ask(t, "post from my handle"); outcome.TaskID != "1" || outcome.Status != contract.StatusWaiting {
		t.Fatalf("the person's own task ended as %+v, want task 1 waiting on its question", outcome)
	}
	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the job's task: %v", err)
	}

	named := built.task(theAnswerThePersonGives)
	named.ResumeID = "1"
	outcome, err := built.loop.Run(t.Context(), named)
	if err != nil {
		t.Fatalf("answering failed: %v", err)
	}

	if outcome.TaskID != "2" || outcome.Status != contract.StatusDone {
		t.Errorf("the answer ended as %+v, want task 2, the job's, which asked last, picked up and finished", outcome)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the job's task reads %+v, want it done on the answer", first)
	}
	if standing := built.held(t, "1").Header.Status; standing != contract.StatusWaiting {
		t.Errorf("the person's own task stands at %q, want it still waiting for the message after", standing)
	}
}

// TestAQuestionNobodyCanAnswerFinishesAScheduledTaskAsFailed pins the other
// side of the rule, the same as for a stop: a schedule's task that asks has
// nobody to answer it, so it is finished as the failure it is, at once rather
// than an hour later when its claim runs out, and the next tick brings its
// own task.
func TestAQuestionNobodyCanAnswerFinishesAScheduledTaskAsFailed(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep(theQuestionTheTaskAsks),
	}, scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	unattended := &jobThatHandsOutOneUnattendedTask{Job: built.jobs}
	options := built.options()
	options.Jobs = unattended
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the unattended job: %v", err)
	}

	if _, err := made.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the schedule's task: %v", err)
	}

	if len(unattended.finished) != 1 || !unattended.finished[0] {
		t.Errorf("the job store was told %v, want the one task that asked finished as failed", unattended.finished)
	}
	if summary := theSummaryOf(t, built, jobID); summary.FailuresInARow != 1 || summary.State != contract.JobRunning {
		t.Errorf("the job reads %+v, want one failure counted and the job still running for its next tick", summary)
	}
	if !sentSomethingLike(built.channel.Sent(), "0 of 2 tasks done") {
		t.Errorf("the person was sent %v, want the failure report with the job's progress on it", built.channel.Sent())
	}
}
