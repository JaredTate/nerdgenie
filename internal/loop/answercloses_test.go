package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theSmallTaskAnswer is the report a model gives on a small ask: what it did,
// what it checked, and that nothing is left. It asks the user nothing.
const theSmallTaskAnswer = "I wrote hello to greeting.txt and read it back. It says hello. Nothing is left."

// TestATaskThatWroteNoDoneListClosesOnItsAnswer is what the live functional
// suite found on all three real models. Asked to write hello to a file and read
// it back, no model writes a done list: it does the work, answers, and never
// calls the task tool at all. The harness read that answer as a question,
// because a record with an empty done list had nothing behind it that could
// close a task, so the task ended waiting on a user who had been told nothing
// and the done-check never ran. An answer that asks nothing closes the task, and
// the answer itself is what proves it.
func TestATaskThatWroteNoDoneListClosesOnItsAnswer(t *testing.T) {
	built := newHarness(t, aSmallTaskThatWritesNoRecord(theSmallTaskAnswer),
		scriptedTool("write", "wrote greeting.txt, 5 characters"),
		scriptedTool("read", "hello"))

	outcome := built.ask(t, "write hello to greeting.txt and read it back")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: the model answered and asked nothing", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "read it back") {
		t.Errorf("the user was sent %v, and they never saw the answer", built.channel.Sent())
	}
	checkTheAnswerIsTheProof(t, built, outcome.TaskID)
}

// checkTheAnswerIsTheProof reads the closed record back and proves the done
// list the harness wrote is one line, ticked, and pointing at a result that
// holds the answer word for word.
func checkTheAnswerIsTheProof(t *testing.T, built *harness, taskID string) {
	t.Helper()
	keeper, err := record.Load(t.Context(), built.store, contract.RecordTask, taskID)
	if err != nil {
		t.Fatalf("cannot load the record of task %s: %v", taskID, err)
	}
	held := keeper.Record()
	if len(held.Goal.DoneWhen) != 1 {
		t.Fatalf("the closed record has %d done lines, and the harness writes one when the model wrote none",
			len(held.Goal.DoneWhen))
	}
	line := held.Goal.DoneWhen[0]
	if !line.Done || line.ResultID == "" {
		t.Fatalf("the done line %+v is not ticked or points at nothing, and the done-check let the task close", line)
	}
	text, err := keeper.Read(t.Context(), line.ResultID)
	if err != nil {
		t.Fatalf("cannot read the result %s the done line points at: %v", line.ResultID, err)
	}
	if text != theSmallTaskAnswer {
		t.Errorf("the result behind the done line reads %q, and the answer was %q", text, theSmallTaskAnswer)
	}
}

// TestATaskThatWroteNoDoneListAndAsksSomethingStillWaits is the other side of
// the rule, and the gate review's ninth finding kept: a reply that asks the user
// for something waits for them, whether or not it ends in a question mark, and
// however little the record has behind it.
func TestATaskThatWroteNoDoneListAndAsksSomethingStillWaits(t *testing.T) {
	for _, asking := range []string{
		"I wrote greeting.txt. Tell me what the file should say next.",
		"I wrote greeting.txt. What should it say next?",
		"I wrote greeting.txt. Let me know whether to read it back.",
	} {
		built := newHarness(t, aSmallTaskThatWritesNoRecord(asking),
			scriptedTool("write", "wrote greeting.txt, 5 characters"),
			scriptedTool("read", "hello"))

		outcome := built.ask(t, "write hello to greeting.txt and read it back")

		if outcome.Status != contract.StatusWaiting {
			t.Errorf("the reply %q ended the task %q, want waiting: it asks the user for something", asking, outcome.Status)
		}
	}
}

// TestAnAnswerSayingTellMeStillClosesWhenTheRecordHasADoneList pins the fence
// round the list of plain ways of asking: those words are read only on a reply
// the record has nothing to close on. The harness's own stopped report ends
// "Tell me how to carry on and I will pick it up from here", and a replay hands
// that report straight back to the loop as a model reply, so a list read on
// every reply would leave every such replay waiting.
func TestAnAnswerSayingTellMeStillClosesWhenTheRecordHasADoneList(t *testing.T) {
	said := "I read the notes. Tell me how to carry on and I will pick it up from here."
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read",`+
				`"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}]}`)),
		answerStep(said),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done: the record said what done looked like and the done-check passed",
			outcome.Status)
	}
}

// aSmallTaskThatWritesNoRecord is the small task the live suite ran: two tool
// calls, no call to the task tool at any point, and then the words the test
// gave it, four times over so that a reading which sends the model back to work
// has somewhere to send it.
func aSmallTaskThatWritesNoRecord(said string) []testkit.Step {
	steps := []testkit.Step{
		callStep("Nothing is written yet. I will write the file.",
			callFor("c1", "write", `{"path":"greeting.txt","text":"hello"}`)),
		callStep("The file is written. I will read it back.",
			callFor("c2", "read", `{"path":"greeting.txt"}`)),
	}
	for range 4 {
		steps = append(steps, answerStep(said))
	}
	return steps
}

// TestTheFortyStepFixtureStillClosesOnItsOwnDoneList proves the new rule leaves
// a task that did write a done list alone: the fixture's own two lines are what
// close it, and the harness adds nothing of its own.
func TestTheFortyStepFixtureStillClosesOnItsOwnDoneList(t *testing.T) {
	played := playTheWholeFixture(t)

	held := played.built.held(t, played.finished.TaskID)
	if len(held.Goal.DoneWhen) != len(played.fixture.DoneWhen) {
		t.Fatalf("the fixture's record closed with %d done lines and the fixture wrote %d",
			len(held.Goal.DoneWhen), len(played.fixture.DoneWhen))
	}
	for at, line := range held.Goal.DoneWhen {
		if line.Text != played.fixture.DoneWhen[at].Text {
			t.Errorf("done line %d reads %q and the fixture wrote %q", at+1, line.Text, played.fixture.DoneWhen[at].Text)
		}
	}
	if strings.Contains(string(record.Print(held)), "-> "+contract.ResultID(44)) {
		t.Error("the harness wrote a done line of its own into a record that already had one")
	}
}
