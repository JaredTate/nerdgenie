// The tests for a reply that ends in a question on a task whose done list is
// proven. A small model ends a finished piece of work with a chatty question,
// "Anything else?", and the loop read that one character before it read the
// done list: a task with every done line proven, the file written and the
// test printing PASS, was put into waiting instead of closing, and a job's
// task so ended put the job down at "1 of 3". The done-check is what says a
// task is done; a trailing question is only chatter once it passes.
package loop_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theChattyClose is a finished reply that ends the way a small model ends one.
const theChattyClose = "It is done. What changed: nothing. What I checked: the notes. What is left: nothing. Anything else?"

// scriptThatProvesItsDoneLineAndThenAsks is a task that writes one done line,
// proves it with its one result, and closes with a question on the end.
func scriptThatProvesItsDoneLineAndThenAsks() []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["the notes are read"]}`)),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}]}`)),
		answerStep(theChattyClose),
	}
}

// TestAProvenDoneListClosesTheTaskWhateverQuestionEndsTheReply is the fix
// itself: every done line proven, so the task is done, and the question on the
// end changes nothing.
func TestAProvenDoneListClosesTheTaskWhateverQuestionEndsTheReply(t *testing.T) {
	built := newHarness(t, scriptThatProvesItsDoneLineAndThenAsks(), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want %q: every done line is proven, and a question on the end of a finished reply is chatter", outcome.Status, contract.StatusDone)
	}
	if standing := built.held(t, "1").Header.Status; standing != contract.StatusDone {
		t.Errorf("the record stands at %q, want %q", standing, contract.StatusDone)
	}
	if !sentSomethingLike(built.channel.Sent(), theChattyClose) {
		t.Errorf("the user was sent %v, want the finished reply as the report", built.channel.Sent())
	}
}

// TestAJobsTaskWithAProvenDoneListClosesAndReportsWhateverQuestionEndsTheReply
// is the same on a job's task, which is where it was caught: the task reports
// to the job and the job is not put down.
func TestAJobsTaskWithAProvenDoneListClosesAndReportsWhateverQuestionEndsTheReply(t *testing.T) {
	steps := append(scriptThatProvesItsDoneLineAndThenAsks(),
		answerStep("The summary is written."),
		aReviewReply("Close on the done list, not on the last character."))
	built := newHarness(t, steps, scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	runTheJobToTheEnd(t, built.loop, built.channel)

	if first := theJobsFirstTask(t, built, jobID); !first.Done || first.ReportID == "" {
		t.Errorf("the job's task reads %+v, want it done with its report in the job", first)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone {
		t.Errorf("the job is %q, want it done after both tasks ran, rather than put down on a finished one", summary.State)
	}
	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the finished task's report with the job's progress on it", built.channel.Sent())
	}
}

// TestAQuestionOnATaskWithWorkLeftStillWaits pins the other side: a done line
// with nothing behind it and a real question on the end is a task waiting on
// the person, as it always was.
func TestAQuestionOnATaskWithWorkLeftStillWaits(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the tweet posted","doneWhen":["the tweet is posted"]}`)),
		answerStep("The notes name two accounts. Which handle should I post from?"),
	}, scriptedTool("read", "the notes name two accounts"))

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, want waiting: its done line is not proven and the model asked the person something", outcome.Status)
	}
	if calls := len(built.model.Requests()); calls != 2 {
		t.Errorf("the model was called %d times, and a question costs the round it was asked in and nothing more", calls)
	}
}

// TestAQuestionOnATaskWithNoDoneListStillWaits pins the reading for a record
// with nothing to close on: a question is a question.
func TestAQuestionOnATaskWithNoDoneListStillWaits(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The notes name two accounts. Which handle should I post from?"),
	}, scriptedTool("read", "the notes name two accounts"))

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, want waiting, because there is no done list to close on and the model asked", outcome.Status)
	}
}
