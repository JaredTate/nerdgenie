package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theReplyLabelThisTestUses is the label a done line points at when the answer
// itself is its proof. The package's own constant is asserted in the bounds
// test, so the two cannot drift apart.
const theReplyLabelThisTestUses = "reply"

// TestADoneLineProvedByTheReplyItselfCloses is what the live serve found. The
// person asked for a three-word reply; the model wrote one done line, "reply to
// the user with exactly three words", and had nothing to point it at, because
// the answer is the proof and the answer had not been given yet. It named a
// result the record did not hold, was refused twice, and the task failed.
func TestADoneLineProvedByTheReplyItselfCloses(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is written yet. I will write down what done looks like.",
			taskCall("c1t", `{"why":"the user wants three words","doneWhen":[`+
				`{"text":"reply to the user with exactly three words","done":true,"resultId":"reply"}]}`)),
		answerStep("Coeus is running."),
	})

	outcome := built.ask(t, "reply with exactly three words")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if !sentSomethingLike(built.channel.Sent(), "Coeus is running.") {
		t.Errorf("the user was sent %v, want the three words they asked for", built.channel.Sent())
	}
	held := built.held(t, outcome.TaskID)
	line := held.Goal.DoneWhen[0]
	if line.ResultID == theReplyLabelThisTestUses {
		t.Errorf("the done line still points at %q, and the reply is written into the record as a result of its own",
			line.ResultID)
	}
	if !holdsAResultSaying(held, "the reply") {
		t.Errorf("the record's results are %v, and the reply the line was proved by is one of them", held.Work.Results)
	}
}

// TestAResultTheRecordNeverWroteIsRefusedWithTheLabelsItHas proves the other
// half: a done line pointing at a result that was never written is told which
// results there are, and that the answer itself has a label of its own.
func TestAResultTheRecordNeverWroteIsRefusedWithTheLabelsItHas(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["the notes are read"]}`)),
		answerStep("It is done. What I checked: the notes."),
		callStep("I will point the line at the result the record really holds.",
			taskCall("c2t", `{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}]}`)),
		answerStep("It is done. What I checked: the notes."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done once the line pointed at a result the record holds", outcome.Status)
	}
	said := requestsJoined(built.model.Requests())
	if !strings.Contains(said, "The results this task has written are r1") {
		t.Error("the model pointed a done line at a result that was never written and was never told which results there are")
	}
	if !strings.Contains(said, `names "reply" as its result`) {
		t.Error("the model was never told that a line proved by the answer itself points at the reply")
	}
}

// holdsAResultSaying says whether the record holds a result line with the words
// given in its summary.
func holdsAResultSaying(held contract.Record, words string) bool {
	for _, one := range held.Work.Results {
		if strings.Contains(one.Summary, words) {
			return true
		}
	}
	return false
}
