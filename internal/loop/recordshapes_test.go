package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestADoneLineNamesItsResultEitherWay is the gate review's twelfth finding. The
// loop's own reader took the fixture's name for a done line's result, resultId,
// and not the task tool's, result, so a model that wrote the shipping tool's
// shape had its proof read as nothing and was sent back until the task was given
// up on.
func TestADoneLineNamesItsResultEitherWay(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["the notes are read"]}`)),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"done_when":[{"text":"the notes are read","done":true,"result":"r1"}]}`)),
		answerStep("It is done. What I checked: the notes."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if line := built.held(t, outcome.TaskID).Goal.DoneWhen[0]; line.ResultID != contract.ResultID(1) {
		t.Errorf("the done line reads %+v, and it named r1 as the result that proves it", line)
	}
}

// TestThePinResultOperationProvesADoneLine proves the loop's own reader
// understands the shipping tool's other way of writing the same thing: the
// operation that points one done line at the result that proves it.
func TestThePinResultOperationProvesADoneLine(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"operation":"done_when","done_when":["the notes are read"]}`)),
		callStep("I will pin the result to the line.",
			taskCall("c2t", `{"operation":"pin_result","line":1,"result":"r1"}`)),
		answerStep("It is done. What I checked: the notes."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	line := built.held(t, outcome.TaskID).Goal.DoneWhen[0]
	if !line.Done || line.ResultID != contract.ResultID(1) {
		t.Errorf("the done line reads %+v, want it ticked and pointing at r1", line)
	}
}

// TestADecisionWrittenFlatIsRead proves the last of the shipping tool's shapes:
// a decision and a failure written as an operation with the text beside it,
// rather than as an object of their own.
func TestADecisionWrittenFlatIsRead(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"operation":"decision","text":"post in the morning","reason":"the notes say mornings do best"}`),
			taskCall("c2t", `{"operation":"failure","text":"the first draft was too long","cause":"the notes were pasted whole"}`)),
		answerStep("Which account should I post from?"),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "post the tweet")

	held := built.held(t, outcome.TaskID)
	if len(held.Lessons.Decisions) != 1 || held.Lessons.Decisions[0].Reason == "" {
		t.Errorf("the record holds the decisions %v, want the one the model wrote with its reason", held.Lessons.Decisions)
	}
	if len(held.Lessons.Failures) != 1 || held.Lessons.Failures[0].Cause == "" {
		t.Errorf("the record holds the failures %v, want the one the model wrote with its cause", held.Lessons.Failures)
	}
}

// TestAPinResultThatNamesNoSuchLineIsRefused proves the refusal says what to
// write instead, which is what every refusal here does.
func TestAPinResultThatNamesNoSuchLineIsRefused(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"done_when":["the notes are read"]}`)),
		callStep("I will pin the result to a line that is not there.",
			taskCall("c2t", `{"operation":"pin_result","line":4,"result":"r1"}`)),
		answerStep("Which line should I pin it to?"),
	}, scriptedTool("read", "the notes"))

	built.ask(t, "read the notes")

	if !strings.Contains(requestsJoined(built.model.Requests()), "done line numbered 4") {
		t.Error("the model pinned a result to a line that is not there and was never told which lines there are")
	}
}
