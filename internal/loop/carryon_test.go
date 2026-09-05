package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theOfferToCarryOn is how a small model ends a task it has not proved: not a
// question about the work, but a request for permission to go on with it.
const theOfferToCarryOn = "The notes are read. Next up is the summary. Want me to keep rolling?"

// aTaskThatOffersToCarryOnBeforeItsProof reads the notes, writes a done line
// with nothing behind it, offers to carry on, and only when sent back points the
// line at the result and closes.
func aTaskThatOffersToCarryOnBeforeItsProof() []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"operation":"done_when","done_when":[{"text":"the notes are read"}]}`)),
		answerStep(theOfferToCarryOn),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"operation":"done_when","done_when":[{"text":"the notes are read","done":true,"result":"r1"}]}`)),
		answerStep("Done. The notes are read, proven by r1."),
	}
}

// TestAJobsTaskThatOffersToCarryOnIsSentBackToWorkRatherThanPutDown is the
// live job that stopped at "1 of 11 tasks done". Its second task ended its
// report with "Want me to keep rolling?" over a done list with nothing marked,
// the harness read the trailing question mark as a question for the person,
// put the whole job down on it, and eleven tasks of autonomous work waited for
// someone to type "continue". Inside a job the person has already said to go
// on, so an offer to carry on is not a question: an unproven done list sends
// the model back to work, the way a plain answer would.
func TestAJobsTaskThatOffersToCarryOnIsSentBackToWorkRatherThanPutDown(t *testing.T) {
	built := newHarness(t, aTaskThatOffersToCarryOnBeforeItsProof(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	ran, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil || !ran {
		t.Fatalf("the loop could not run the job's task: ran %v, %v", ran, err)
	}

	requests := built.model.Requests()
	if len(requests) != 4 {
		t.Fatalf("the model was called %d times, want 4: the offer to carry on sends it back for the proof rather than ending the task", len(requests))
	}
	if told := wholeRequestText(requests[2]); !strings.Contains(told, "nothing behind") {
		t.Errorf("after the offer the model was told:\n%s\nwant the done-check's own line naming the unproven done line", told)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the job's first task reads %+v, want it done once the proof was pointed at", first)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State == contract.JobPaused {
		t.Errorf("the job reads %+v, and an offer to carry on must not put it down", summary)
	}
}

// TestAPlainTaskThatOffersToCarryOnStillWaitsForThePerson holds the other
// side: a person's own task is theirs to steer, so the same offer on a task
// typed at the terminal waits for the answer, as it always did.
func TestAPlainTaskThatOffersToCarryOnStillWaitsForThePerson(t *testing.T) {
	built := newHarness(t, aTaskThatOffersToCarryOnBeforeItsProof(), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, want it waiting on the person who can say whether to carry on", outcome.Status)
	}
	if calls := len(built.model.Requests()); calls != 2 {
		t.Errorf("the model was called %d times, want 2: the offer ends the turn on a person's task", calls)
	}
}
