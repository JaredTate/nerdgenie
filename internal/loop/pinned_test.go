package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theBrandRule is the piece of evidence the model must not lose, written so
// that it can be found in a prompt and nowhere else by accident.
const theBrandRule = "the brand rule: never write the word cheap about the product"

// TestAResultTheModelPinsStaysInFrontOfIt is the gate review's eighth finding.
// The instruction text tells the model on every call that the harness gives it
// any evidence that has been pinned, and nothing anywhere ever pinned any: the
// working context's pinned layer had no producer at all.
func TestAResultTheModelPinsStaysInFrontOfIt(t *testing.T) {
	built := newHarness(t, scriptThatPins(`{"operation":"pin_evidence","result":"r1"}`),
		scriptedTool("read", theBrandRule, "the notes are long and say nothing about the brand"))

	built.ask(t, "write the post")

	last := built.model.Requests()[len(built.model.Requests())-1]
	pinned := pinnedEvidenceIn(last)
	if pinned == "" {
		t.Fatalf("no call carried any pinned evidence, and the model was told on every one of them that it would")
	}
	if !strings.Contains(pinned, theBrandRule) {
		t.Errorf("the pinned evidence reads %q, want the whole text of the result the model pinned", pinned)
	}
	if !strings.Contains(pinned, "pinned "+contract.ResultID(1)) {
		t.Errorf("the pinned evidence reads %q, and a pin names the result it came from", pinned)
	}
}

// TestAResultTheModelUnpinsLeavesTheWindow proves the other half: what is
// pinned until it is unpinned is unpinned when the model says so.
func TestAResultTheModelUnpinsLeavesTheWindow(t *testing.T) {
	built := newHarness(t, scriptThatPins(
		`{"operation":"pin_evidence","result":"r1"}`,
		`{"operation":"unpin_evidence","result":"r1"}`),
		scriptedTool("read", theBrandRule, "the notes are long and say nothing about the brand"))

	built.ask(t, "write the post")

	last := built.model.Requests()[len(built.model.Requests())-1]
	if pinned := pinnedEvidenceIn(last); pinned != "" {
		t.Errorf("the last call still carried the pinned evidence %q after the model unpinned it", pinned)
	}
}

// TestAPinOfAResultThatIsNotThereIsRefused proves the harness answers a pin it
// cannot honour with what to write instead, rather than pinning nothing
// silently.
func TestAPinOfAResultThatIsNotThereIsRefused(t *testing.T) {
	built := newHarness(t, scriptThatPins(`{"operation":"pin_evidence","result":"r99"}`),
		scriptedTool("read", theBrandRule, "the notes"))

	built.ask(t, "write the post")

	if !strings.Contains(requestsJoined(built.model.Requests()), "there is no result r99") {
		t.Error("the model pinned a result this task never wrote and was never told so")
	}
}

// TestOnlySoManyResultsMayBePinnedAtOnce pins the bound: the pinned layer never
// leaves the window, so a task that pins everything would leave no room for the
// work.
func TestOnlySoManyResultsMayBePinnedAtOnce(t *testing.T) {
	pins := []string{}
	for at := 1; at <= theNumberOfPinsThisTestExpects+1; at++ {
		pins = append(pins, `{"operation":"pin_evidence","result":"`+contract.ResultID(at)+`"}`)
	}
	built := newHarness(t, scriptThatPins(pins...), scriptedTool("read", theBrandRule, "the notes"))

	built.ask(t, "write the post")

	last := built.model.Requests()[len(built.model.Requests())-1]
	if pinned := strings.Count(pinnedEvidenceIn(last), "\npinned "); pinned > theNumberOfPinsThisTestExpects {
		t.Errorf("%d results were pinned at once, and the cap is %d so that the work still fits",
			pinned, theNumberOfPinsThisTestExpects)
	}
}

// theNumberOfPinsThisTestExpects is the cap this test was written against. The
// bounds test asserts that the package's own constant still reads the same.
const theNumberOfPinsThisTestExpects = 4

// scriptThatPins is one task that reads a file and then makes the record writes
// the test gave it, one to a round, before ending the turn with a question.
func scriptThatPins(writes ...string) []testkit.Step {
	steps := []testkit.Step{
		callStep("Nothing is read yet. I will read the brand file.",
			callFor("c1", "read", `{"path":"brand.md"}`),
			taskCall("c1t", `{"why":"the user wants a post","doneWhen":["the post is written"]}`)),
	}
	for at, written := range writes {
		steps = append(steps, callStep("I will write that down.", taskCall("w"+contract.ResultID(at+1), written)))
	}
	return append(steps, answerStep("Which account should I post from?"))
}

// TestAPinMadeBeforeAStopIsStillThereAfterTheResume is the finding this file
// was reopened for: the pinned evidence lived in the run and nowhere else, so a
// task put down with the brand rule in front of the model was picked up with it
// gone, and the model wrote the very word the rule forbids.
func TestAPinMadeBeforeAStopIsStillThereAfterTheResume(t *testing.T) {
	built := newHarness(t, append(scriptThatPins(`{"operation":"pin_evidence","result":"r1"}`),
		answerStep("The post is written. What is left: nothing.")),
		scriptedTool("read", theBrandRule, "the notes are long and say nothing about the brand"))

	waiting := built.ask(t, "write the post")
	if waiting.Status != contract.StatusWaiting {
		t.Fatalf("the first turn ended %q, want waiting, so that there is a task to pick up", waiting.Status)
	}
	callsBefore := len(built.model.Requests())

	answered := built.task("the DigiByte account")
	answered.ResumeID = waiting.TaskID
	if _, err := built.loop.Run(t.Context(), answered); err != nil {
		t.Fatalf("the loop could not pick the task up again: %v", err)
	}

	first := built.model.Requests()[callsBefore]
	pinned := pinnedEvidenceIn(first)
	if pinned == "" {
		t.Fatal("the first call after the resume carried no pinned evidence, and the pin was made before the task was put down")
	}
	if !strings.Contains(pinned, theBrandRule) {
		t.Errorf("the pinned evidence after the resume reads %q, want the whole text of the result that was pinned", pinned)
	}
}

// TestAPinIsWrittenIntoTheRecordItself proves where the pin now lives: in the
// record, which is the only thing that survives the wait, rather than in the
// run, which does not.
func TestAPinIsWrittenIntoTheRecordItself(t *testing.T) {
	built := newHarness(t, scriptThatPins(`{"operation":"pin_evidence","result":"r1"}`),
		scriptedTool("read", theBrandRule, "the notes"))

	outcome := built.ask(t, "write the post")

	results := built.held(t, outcome.TaskID).Work.Results
	if len(results) == 0 {
		t.Fatal("the record holds no results at all, so there was nothing to pin")
	}
	if !results[0].Pinned {
		t.Errorf("the record's line for %s is not marked pinned, so nothing would bring the pin back after a resume", results[0].ID)
	}
}

// TestAPinTheModelLetsGoOfIsNotBroughtBack proves the unpin is written down too,
// so that a resume does not put back evidence the model has finished with.
func TestAPinTheModelLetsGoOfIsNotBroughtBack(t *testing.T) {
	built := newHarness(t, scriptThatPins(
		`{"operation":"pin_evidence","result":"r1"}`,
		`{"operation":"unpin_evidence","result":"r1"}`),
		scriptedTool("read", theBrandRule, "the notes"))

	outcome := built.ask(t, "write the post")

	if built.held(t, outcome.TaskID).Work.Results[0].Pinned {
		t.Error("the record still marks the result pinned after the model unpinned it")
	}
}

// pinnedEvidenceIn is the pinned block of one request, and is empty when the
// request carried none.
func pinnedEvidenceIn(request contract.Request) string {
	for _, message := range request.Messages {
		if strings.Contains(message.Text, "Pinned evidence") {
			return message.Text
		}
	}
	return ""
}
