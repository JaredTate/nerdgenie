package loop_test

import (
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestABudgetThatRanOutCostsOneReportAndOneCall is the gate review's fourteenth
// finding. The ending asked the model for a report with the tools off, then sent
// the harness's own stopped report as well, and then asked the four review
// questions on top: two messages to the user and two model calls, on a task that
// had just been told its budget was spent.
func TestABudgetThatRanOutCostsOneReportAndOneCall(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read one.", callFor("c1", "read", `{"path":"one.md"}`)),
		callStep("One is read. I will read two.", callFor("c2", "read", `{"path":"two.md"}`)),
		answerStep("I read two files. What is left: the third one."),
	}, scriptedTool("read", "the first file", "the second file"))

	task := built.task("read the three files")
	task.Budget = loop.Budget{Rounds: 2}
	outcome, err := built.loop.Run(t.Context(), task)
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}

	if sent := built.channel.Sent(); len(sent) != 1 {
		t.Errorf("the user was sent %d messages about one ending: %v", len(sent), sent)
	}
	if calls := len(built.model.Requests()); calls != 3 {
		t.Errorf("the model was called %d times for a two-round budget, want three: the two rounds and the one report",
			calls)
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because its budget ran out", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "What is left: the third one") {
		t.Errorf("the user was sent %v, want the model's own report of what is left", built.channel.Sent())
	}
	if !sentSomethingLike(built.channel.Sent(), "budget of 2 rounds is used up") {
		t.Errorf("the user was sent %v, and the one report says why the task ended", built.channel.Sent())
	}
}
