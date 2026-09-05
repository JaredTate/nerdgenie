// The tests for a person stopping a job's task. The person's own words: when
// they interrupt work, it must "stay on the job and task". A stop puts the
// job's task down rather than finishing it, and the word that carries on picks
// the same task up under the same job.
package loop_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// modelThatWaitsOnOneCall plays a script, except that one of its calls, counted
// from one, says nothing until it is cancelled: a model gone quiet at the
// moment the person presses Escape. The waiting call consumes no step of the
// script, so the script is what every other call answers with.
type modelThatWaitsOnOneCall struct {
	inner   *testkit.FakeModel
	waitOn  int
	guard   sync.Mutex
	calls   int
	calling chan struct{}
}

// aModelThatWaitsOn wraps the harness's scripted model so that one call waits.
func aModelThatWaitsOn(call int, inner *testkit.FakeModel) *modelThatWaitsOnOneCall {
	return &modelThatWaitsOnOneCall{inner: inner, waitOn: call, calling: make(chan struct{})}
}

// Name is the alias the record and the cost line print.
func (waiting *modelThatWaitsOnOneCall) Name() string { return waiting.inner.Name() }

// ContextLength is the window the scripted model reports.
func (waiting *modelThatWaitsOnOneCall) ContextLength() int { return waiting.inner.ContextLength() }

// Send answers from the script, except on the one call that waits to be
// cancelled and never answers.
func (waiting *modelThatWaitsOnOneCall) Send(ctx context.Context, request contract.Request, onDelta func(delta string)) (contract.Reply, error) {
	waiting.guard.Lock()
	waiting.calls++
	call := waiting.calls
	waiting.guard.Unlock()
	if call == waiting.waitOn {
		close(waiting.calling)
		<-ctx.Done()
		return contract.Reply{}, ctx.Err()
	}
	return waiting.inner.Send(ctx, request, onDelta)
}

// callsMade is how many times the model was called, waiting call included.
func (waiting *modelThatWaitsOnOneCall) callsMade() int {
	waiting.guard.Lock()
	defer waiting.guard.Unlock()
	return waiting.calls
}

// aJobWhoseTaskIsStoppedOn builds the harness with a job of two tasks and a
// loop over a model that waits on the call given, so that a test can stop the
// job's first task at exactly that call.
func aJobWhoseTaskIsStoppedOn(t *testing.T, call int, steps []testkit.Step) (*harness, *loop.Loop, *modelThatWaitsOnOneCall, string) {
	t.Helper()
	built := newHarness(t, steps, scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	waiting := aModelThatWaitsOn(call, built.model)
	made, err := loop.New(built.optionsOver(waiting))
	if err != nil {
		t.Fatalf("cannot build a loop over a model that waits: %v", err)
	}
	return built, made, waiting, jobID
}

// stopTheJobsTask runs the job's next task in the background, stops it the
// moment the waiting model call is in flight, which is what Escape does, and
// waits for the loop to come back.
func stopTheJobsTask(t *testing.T, made *loop.Loop, built *harness, waiting *modelThatWaitsOnOneCall) {
	t.Helper()
	ended := make(chan error, 1)
	go func() {
		ran, err := made.RunNextJobTask(context.Background(), built.channel)
		if err == nil && !ran {
			err = errors.New("the loop found nothing to run, and the job has two tasks waiting")
		}
		ended <- err
	}()
	<-waiting.calling
	made.Stop()
	select {
	case err := <-ended:
		if err != nil {
			t.Fatalf("the job's task did not stop cleanly: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the loop never came back after the stop")
	}
}

// theJobsFirstTask reads the job's first task back out of the fake store.
func theJobsFirstTask(t *testing.T, built *harness, jobID string) contract.JobTask {
	t.Helper()
	tasks := built.jobs.Tasks(jobID)
	if len(tasks) == 0 {
		t.Fatalf("the job %s has no tasks at all", jobID)
	}
	return tasks[0]
}

// theSummaryOf reads one job's line out of the fake store's listing.
func theSummaryOf(t *testing.T, built *harness, jobID string) contract.JobSummary {
	t.Helper()
	listed, err := built.jobs.List(t.Context())
	if err != nil {
		t.Fatalf("cannot list the jobs: %v", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary
		}
	}
	t.Fatalf("the job %s is not listed", jobID)
	return contract.JobSummary{}
}

// TestAPersonsStopPutsAJobsTaskDownRatherThanFailingIt proves what a stop does
// to a job's task: nothing is written into the job, the task is neither done
// nor failed, the job is paused on it rather than moving on or trying it
// again, and the person is told which job waits on which task.
func TestAPersonsStopPutsAJobsTaskDownRatherThanFailingIt(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
	})

	stopTheJobsTask(t, made, built, waiting)

	if first := theJobsFirstTask(t, built, jobID); first.Done || first.ReportID != "" {
		t.Errorf("the stopped task reads %+v, want it neither done nor reported", first)
	}
	summary := theSummaryOf(t, built, jobID)
	if summary.FailuresInARow != 0 {
		t.Errorf("the stop counted %d failures in a row, and a stop is not a failure", summary.FailuresInARow)
	}
	if summary.State != contract.JobPaused {
		t.Errorf("after the stop the job is %q, want it paused on the task until the person carries on", summary.State)
	}
	held, err := built.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 0 {
		t.Errorf("the job holds %d reports after the stop, want none: a stopped task has not reported", len(held.Work.Results))
	}
	if calls := waiting.callsMade(); calls != 2 {
		t.Errorf("the model was called %d times, want 2: the task's own call and the one the stop cancelled, and no run of the task again", calls)
	}
	sent := built.channel.Sent()
	if len(sent) != 1 {
		t.Fatalf("the person was sent %d messages, want the one stopped report: %v", len(sent), sent)
	}
	for _, words := range []string{"I stopped this task", "Job " + jobID, "t1", "Say continue to pick this task up"} {
		if !strings.Contains(sent[0], words) {
			t.Errorf("the stopped report is %q, want it to say %q", sent[0], words)
		}
	}
	if strings.Contains(sent[0], "tasks done") {
		t.Errorf("the stopped report is %q, and a task that was only stopped has no progress line", sent[0])
	}
	if strings.Contains(sent[0], "Tell me how to carry on") {
		t.Errorf("the stopped report is %q, and it gives two instructions where one is the truth: only the word continue picks a job's task up", sent[0])
	}
	if standing := built.held(t, "1").Header.Status; standing != contract.StatusStopped {
		t.Errorf("the task's record stands at %q, want %q", standing, contract.StatusStopped)
	}
	if lines := built.recordLines(); len(lines) != 2 || !strings.HasPrefix(lines[1], "job "+jobID+" task 1 stopped") {
		t.Errorf("the record lines are %v, want the task started and stopped once and nothing run afterwards", lines)
	}
}

// TestContinuePicksAPutDownJobTaskUpUnderItsJob proves the other half: the one
// word picks the same task up, under the same job and with the same number,
// and when it finishes its report goes into the job and the next task starts
// as usual.
func TestContinuePicksAPutDownJobTaskUpUnderItsJob(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	})
	stopTheJobsTask(t, made, built, waiting)

	outcome, err := made.Run(t.Context(), built.task("continue"))
	if err != nil {
		t.Fatalf("carrying the task on failed: %v", err)
	}

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("continue ended as %+v, want task 1, the one that was stopped, finished", outcome)
	}
	lines := built.recordLines()
	if !sentSomethingLike(lines, "job "+jobID+" task 1 started · continue") {
		t.Errorf("the record lines are %v, want task 1 started again as the job's task", lines)
	}
	if held := built.held(t, "1"); len(held.Work.Results) == 0 || held.Work.Results[0].Summary == "" {
		t.Errorf("the record picked up reads %+v, want the result it held before the stop still there", held.Work)
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done || task.ReportID == "" {
			t.Errorf("the task %s reads %+v, want it done and pointing at its report", task.TaskID, task)
		}
	}
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the picked-up task's report with the job's progress on it", sent)
	}
	if !sentSomethingLike(sent, "Job "+jobID+" is finished") {
		t.Errorf("the person was sent %v, want the next task run and the job closed after it", sent)
	}
}

// TestContinueOnAFreshLoopPicksUpTheTaskTheStoreSaysWasPutDown is the restart
// half of the put-down: the memory of which task was put down lives in the
// job store, not in the loop, so a loop built afresh over the same store,
// which is what a restart leaves, picks the same task up under the same job
// on the word that carries on.
func TestContinueOnAFreshLoopPicksUpTheTaskTheStoreSaysWasPutDown(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	})
	stopTheJobsTask(t, made, built, waiting)
	mark, there, err := built.jobs.PutDownTask(t.Context())
	if err != nil || !there || mark.Task.JobID != jobID || mark.Task.TaskID != "t1" || mark.Run != "1" || !mark.HasRecord || mark.Waiting {
		t.Fatalf("the store holds the put-down task as %+v (there %v, error %v), want task t1 of job %s, run 1, with a record, stopped rather than asking", mark, there, err, jobID)
	}
	fresh, err := loop.New(built.options())
	if err != nil {
		t.Fatalf("cannot build a fresh loop over the same store: %v", err)
	}

	outcome, err := fresh.Run(t.Context(), built.task("continue"))
	if err != nil {
		t.Fatalf("carrying the task on from a fresh loop failed: %v", err)
	}

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("continue on a fresh loop ended as %+v, want task 1, the one the store says was put down, finished", outcome)
	}
	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the picked-up task's report with the job's progress on it", built.channel.Sent())
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store still holds a put-down task after it was picked up")
	}
}

// TestAPutDownTaskWithNoRecordStartsAfreshUnderItsJob proves the case where the
// stop landed before the task's first tool call: there is no record to pick
// up, so the word starts the task again from the beginning, still as the
// job's task.
func TestAPutDownTaskWithNoRecordStartsAfreshUnderItsJob(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 1, []testkit.Step{
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	})
	stopTheJobsTask(t, made, built, waiting)

	if _, err := made.Run(t.Context(), built.task("Carry on.")); err != nil {
		t.Fatalf("carrying the task on failed: %v", err)
	}

	lines := built.recordLines()
	if !sentSomethingLike(lines, "job "+jobID+" task 2 started · post the anniversary tweet") {
		t.Errorf("the record lines are %v, want the task started afresh as the job's task with the task's own words", lines)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the task reads %+v, want it done after being started afresh", first)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone {
		t.Errorf("the job is %q, want it done after both tasks ran", summary.State)
	}
}

// TestAJobsTaskPickedUpByItsNumberIsPickedUpAsTheJobsTask proves that a job's
// task which asked a question and was answered by its number, the way the
// program answers any waiting task, still reports to its job.
func TestAJobsTaskPickedUpByItsNumberIsPickedUpAsTheJobsTask(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which account should I post from?"),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Ask about the account first."),
	}, scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the job's task: %v", err)
	}
	if first := theJobsFirstTask(t, built, jobID); first.Done {
		t.Fatalf("the task reads %+v before its question was answered, want it still open", first)
	}

	answer := built.task("the main one")
	answer.ResumeID = "1"
	outcome, err := built.loop.Run(t.Context(), answer)
	if err != nil {
		t.Fatalf("answering the task's question failed: %v", err)
	}

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("the answer ended as %+v, want task 1 finished", outcome)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the task reads %+v, want its report in the job once its question was answered", first)
	}
	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the answered task's report with the job's progress on it", built.channel.Sent())
	}
}

// TestContinueLeavesAPutDownTaskAloneWhenAPlainTaskStoppedAfterIt proves which
// task the word means when two stopped: the one that stopped last, which the
// program names by its number when it is the person's own.
func TestContinueLeavesAPutDownTaskAloneWhenAPlainTaskStoppedAfterIt(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 1, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("I stopped where I was and read the notes again. What is left: nothing."),
	})
	stopTheJobsTask(t, made, built, waiting)
	// The person's own task makes a record and is then stopped, so the program
	// remembers it as task 2 and names it on the next word.
	if err := made.Deliver(contract.Inbound{Text: "stop", Channel: "terminal"}); err != nil {
		t.Fatalf("cannot hand the loop the word stop: %v", err)
	}
	if outcome, err := made.Run(t.Context(), built.task("read the notes")); err != nil || outcome.Status != contract.StatusStopped {
		t.Fatalf("the person's own task ended as %+v (%v), want it stopped", outcome, err)
	}

	named := built.task("continue")
	named.ResumeID = "2"
	outcome, err := made.Run(t.Context(), named)
	if err != nil {
		t.Fatalf("carrying the person's own task on failed: %v", err)
	}

	if outcome.TaskID != "2" {
		t.Errorf("continue picked up task %q, want task 2, the person's own, which stopped last", outcome.TaskID)
	}
	if first := theJobsFirstTask(t, built, jobID); first.Done {
		t.Errorf("the job's task reads %+v, want it still put down", first)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobPaused {
		t.Errorf("the job is %q, want it still paused on its task", summary.State)
	}
}

// TestContinuePicksThePutDownTaskUpWhenItStoppedAfterThePlainTask is the
// other order: the person's own task stopped first and the job's task after
// it, so the word means the job's task whatever number the program names.
func TestContinuePicksThePutDownTaskUpWhenItStoppedAfterThePlainTask(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("I will read the notes for the post.", callFor("c2", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	}, scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	waiting := aModelThatWaitsOn(3, built.model)
	made, err := loop.New(built.optionsOver(waiting))
	if err != nil {
		t.Fatalf("cannot build a loop over a model that waits: %v", err)
	}
	// The person's own task, task 1, is stopped first, after its one tool
	// call; the job's task, task 2, then stops at its second call, so both
	// have a record and the job's is the one that stopped last.
	if err := made.Deliver(contract.Inbound{Text: "stop", Channel: "terminal"}); err != nil {
		t.Fatalf("cannot hand the loop the word stop: %v", err)
	}
	if outcome, err := made.Run(t.Context(), built.task("read the notes")); err != nil || outcome.Status != contract.StatusStopped {
		t.Fatalf("the person's own task ended as %+v (%v), want it stopped", outcome, err)
	}
	stopTheJobsTask(t, made, built, waiting)

	named := built.task("keep going")
	named.ResumeID = "1"
	outcome, err := made.Run(t.Context(), named)
	if err != nil {
		t.Fatalf("carrying on failed: %v", err)
	}

	if outcome.TaskID != "2" || outcome.Status != contract.StatusDone {
		t.Errorf("continue ended as %+v, want the job's task 2, which stopped last, picked up and finished", outcome)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the job's task reads %+v, want it done", first)
	}
}
