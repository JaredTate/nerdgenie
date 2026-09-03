package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theQuestionWithNoQuestionMark is the reviewer's own probe: a model that asks
// for what it needs in a sentence ending with a full stop.
const theQuestionWithNoQuestionMark = "I need to know which account to post from. Please tell me the handle."

// TestAQuestionWithNoQuestionMarkStillWaitsForTheUser is the gate review's
// ninth finding. A question was told from an answer by one character, so a
// model that asked politely was sent to the done-check, nudged three times, and
// the task failed with the user never seeing the question.
func TestAQuestionWithNoQuestionMarkStillWaitsForTheUser(t *testing.T) {
	built := newHarness(t, scriptThatAsks(theQuestionWithNoQuestionMark), scriptedTool("read", "the notes name two accounts"))

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, want waiting, because the model asked the user for something", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "Please tell me the handle") {
		t.Errorf("the user was sent %v, and they never saw the question the task is waiting on", built.channel.Sent())
	}
	if calls := len(built.model.Requests()); calls != 2 {
		t.Errorf("the model was called %d times, and a question costs the round it was asked in and nothing more", calls)
	}
}

// TestTheSameWordsWithAQuestionMarkWaitTheSameWay is the reviewer's second
// probe, which passed before this change and must go on passing.
func TestTheSameWordsWithAQuestionMarkWaitTheSameWay(t *testing.T) {
	built := newHarness(t, scriptThatAsks("I need to know which account to post from. Which handle should I use?"),
		scriptedTool("read", "the notes name two accounts"))

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, want waiting", outcome.Status)
	}
}

// TestAnAnswerWithADoneListBehindItStillGoesToTheDoneCheck proves the other
// side of the reading: a reply with a done list behind it is the model saying
// the work is finished, whatever its last character is, and the done-check still
// has the last word on that.
func TestAnAnswerWithADoneListBehindItStillGoesToTheDoneCheck(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["the notes are read"]}`)),
		answerStep("It is done. What changed: nothing. What I checked: the notes."),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}]}`)),
		answerStep("It is done. What changed: nothing. What I checked: the notes."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done, because the reply had a done list behind it", outcome.Status)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "cannot close yet") {
		t.Error("the done-check never ran on a reply that ended with a full stop and had a done list behind it")
	}
}

// scriptThatAsks is one task that reads a file and then says the words the test
// gave it, three times over, so that a reading which sends the model back to
// work has somewhere to send it.
func scriptThatAsks(said string) []testkit.Step {
	steps := []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
	}
	for range 4 {
		steps = append(steps, answerStep(said))
	}
	return steps
}
