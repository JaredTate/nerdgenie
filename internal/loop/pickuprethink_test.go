// The tests of a pick-up starting with a rethink. The sky task of the
// flight-simulator work order was picked up three times after a guard stop,
// and each time it opened on the record, read the failure list that had
// stopped it, and went straight back to it.
package loop_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/orientation"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aTaskThatWritesAFailureAndRunsOut is one round of work that writes a
// failure into the record, and then the report the spent budget buys, so
// that the task ends stopped with a failure on its record.
func aTaskThatWritesAFailureAndRunsOut() []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes and write what went wrong.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("f1", `{"operation":"failure","text":"the notes never name the room","cause":"the room is written in the brand file"}`)),
		answerStep("The budget is spent. What changed: nothing. What I checked: the notes. What is left: the room."),
	}
}

// TestAPickedUpTaskWithAFailureOpensWithARethink: the sky task was picked up
// three times, and each time it opened on the record and went straight back
// to the failure list that had stopped it. A task picked up after a stop
// whose record holds a failure now opens with a rethink, before its first
// working call: the record and the newest result in front of the model, the
// five questions, the answer into the record, and the fresh window on the
// answer, with the person's word last.
func TestAPickedUpTaskWithAFailureOpensWithARethink(t *testing.T) {
	steps := append(aTaskThatWritesAFailureAndRunsOut(),
		aRethinkAnswer("the notes were read once and never named the room", "the notes do not hold the room",
			"the room is on the printed invitation, or nobody wrote it down", "read brand.md"),
		callStep("I will read the brand file, as the rethink says.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("The room is in the brand file. What changed: nothing. What I checked: both files. What is left: nothing."))
	built := newHarness(t, steps, scriptedTool("read", "the notes say the meeting is at noon", "the brand file names the room"))
	stopped := runOutOfBudget(t, built)
	before := len(built.model.Requests())

	outcome := continueTheTask(t, built, stopped.TaskID)

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	requests := built.model.Requests()
	if len(requests) < before+2 {
		t.Fatalf("the pick-up made %d model calls, want at least two: the rethink and the working call after it", len(requests)-before)
	}
	asked := requests[before]
	if !asked.ToolsOff || asked.Messages[len(asked.Messages)-1].Text != loop.TheRethinkQuestion {
		t.Fatalf("the pick-up's first call is not the rethink with the tools off:\n%s", wholeRequestText(asked))
	}
	background := asked.Messages[0].Text
	for _, words := range []string{"F1", "the notes never name the room", "the notes say the meeting is at noon"} {
		if !strings.Contains(background, words) {
			t.Errorf("the rethink's background does not carry %q:\n%s", words, background)
		}
	}
	window := requests[before+1]
	opening, rethought, there := theMessagesAround(window, loop.TheRethinkLine)
	if !there || !strings.Contains(rethought.Text, "Next: read brand.md") || !strings.Contains(rethought.Text, loop.TheDoTheNextLine) {
		t.Fatalf("the working call after the rethink does not open on the answer with the instruction to do its Next line:\n%s", wholeRequestText(window))
	}
	if !strings.HasPrefix(opening.Text, orientation.TheHeading) || !strings.Contains(opening.Text, "the notes say the meeting is at noon") {
		t.Errorf("the fresh window does not open with the orientation and the newest results in full:\n%s", opening.Text)
	}
	// The working context puts the record's tail after the loop's own
	// messages, so the ask is the message right after the rethink's.
	at := slices.IndexFunc(window.Messages, func(message contract.Message) bool { return message.Text == rethought.Text })
	if at < 0 || at+1 >= len(window.Messages) || window.Messages[at+1].Text != "carry on" {
		t.Errorf("the person's word does not ride right after the rethink; the window reads:\n%s", wholeRequestText(window))
	}
	held := built.held(t, outcome.TaskID)
	if !aFailureBeginning(held, "stalled: the notes were read once") {
		t.Errorf("the record's failures read %+v, want the rethink's Showed line written as a failure", held.Lessons.Failures)
	}
	if _, written := aDecisionBeginning(held, "Rethink: next read brand.md"); !written {
		t.Errorf("the record's decisions read %+v, want the Next line as a decision", held.Lessons.Decisions)
	}
}

// TestAPickedUpTaskWithNoFailureOpensAsBefore: a picked-up task with no
// failure on its record has nothing to rethink, and opens on the orientation
// and the person's word as it did.
func TestAPickedUpTaskWithNoFailureOpensAsBefore(t *testing.T) {
	built := newHarness(t, aTaskThatRunsOutAndThenCarriesOn(), scriptedTool("read", "the notes", "the brand file"))
	stopped := runOutOfBudget(t, built)
	before := len(built.model.Requests())

	outcome := continueTheTask(t, built, stopped.TaskID)

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if rethinks := theRethinkRequests(built); len(rethinks) != 0 {
		t.Errorf("the model was asked the rethink's question %d times, and the record holds no failure", len(rethinks))
	}
	first := built.model.Requests()[before]
	if first.ToolsOff || !strings.Contains(wholeRequestText(first), orientation.TheHeading) {
		t.Errorf("the pick-up's first call does not open on the orientation with the tools on:\n%s", wholeRequestText(first))
	}
	if _, count := requestsCarrying(built, loop.TheRethinkLine); count != 0 {
		t.Errorf("the rethink line rode on %d calls, and there was no rethink", count)
	}
}

// TestATaskAnsweredOnItsQuestionOpensWithoutARethink: a task waiting on its
// own question is not a task picked up after a stop. The person's answer is
// what it needs, so it opens as before, failure on the record or not.
func TestATaskAnsweredOnItsQuestionOpensWithoutARethink(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes and write what went wrong.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("f1", `{"operation":"failure","text":"the notes never name the room","cause":"the room is written in the brand file"}`)),
		answerStep("Which room did you mean?"),
		answerStep("The blue room it is. What changed: nothing. What I checked: the notes. What is left: nothing."),
	}, scriptedTool("read", "the notes say the meeting is at noon"))
	waiting := built.ask(t, "read the notes and find the room")
	if waiting.Status != contract.StatusWaiting {
		t.Fatalf("the first sitting ended %q, want waiting on the question", waiting.Status)
	}

	answered := built.task("the blue room")
	answered.ResumeID = waiting.TaskID
	outcome, err := built.loop.Run(t.Context(), answered)
	if err != nil {
		t.Fatalf("the loop could not pick the task up again: %v", err)
	}

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the answered task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if rethinks := theRethinkRequests(built); len(rethinks) != 0 {
		t.Errorf("the model was asked the rethink's question %d times on an answer to its own question", len(rethinks))
	}
}
