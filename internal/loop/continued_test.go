package loop_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theOneRoundBudget is the budget the first half of these tests runs out of, so
// that a task really does end stopped with nothing left rather than being made
// to look as though it had.
var theOneRoundBudget = loop.Budget{Rounds: 1}

// TestAStoppedTaskThePersonContinuesGetsAFreshBudget is what the resume work
// found. A task whose budget ran out ends stopped with no rounds left, and the
// report tells the person to say how to carry on. When they did, the task was
// picked up at exactly the budget it had stopped on — nothing — so it spent its
// two spare rounds writing the same ending again and told them the budget was
// used up. Asking a task to carry on is asking for more, so a stopped task that
// is picked up starts on a fresh budget, with one line in its record saying so.
func TestAStoppedTaskThePersonContinuesGetsAFreshBudget(t *testing.T) {
	built := newHarness(t, aTaskThatRunsOutAndThenCarriesOn(), scriptedTool("read", "the notes", "the brand file"))

	stopped := runOutOfBudget(t, built)
	carryOn := built.task("carry on")
	carryOn.ResumeID = stopped.TaskID
	// The carry-on runs under the same skill as the first sitting, so it has a
	// budget of its own; the caps themselves set none.
	carryOn.Budget = loop.Budget{Rounds: 5, Time: 10 * time.Minute}
	carriedOn, err := built.loop.Run(t.Context(), carryOn)
	if err != nil {
		t.Fatalf("the loop could not pick task %s up again: %v", stopped.TaskID, err)
	}

	if carriedOn.Status != contract.StatusDone {
		t.Fatalf("the continued task ended %q, want done: it was given a fresh budget to finish on. %s",
			carriedOn.Status, carriedOn.Report)
	}
	held := built.held(t, stopped.TaskID)
	if held.Header.RoundsLeft < 1 {
		t.Errorf("the continued task closed with %d rounds left, and a fresh budget is more than it stopped on",
			held.Header.RoundsLeft)
	}
	if held.Header.MinutesLeft < 1 {
		t.Errorf("the continued task closed with %d minutes left, and a fresh budget is more than it stopped on",
			held.Header.MinutesLeft)
	}
	situation := strings.Join(held.Work.Situation, "\n")
	if !strings.Contains(situation, "carry on") {
		t.Errorf("the situation reads %q, and one line of it says the person asked this task to carry on", situation)
	}
}

// runOutOfBudget plays the first half: one round of work on a budget of one
// round, and then the ending the harness writes when the budget is spent.
func runOutOfBudget(t *testing.T, built *harness) loop.Outcome {
	t.Helper()
	first := built.task("read the notes and the brand file")
	first.Budget = theOneRoundBudget
	outcome, err := built.loop.Run(t.Context(), first)
	if err != nil {
		t.Fatalf("the loop could not run the task that runs out of budget: %v", err)
	}
	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped, because its budget ran out", outcome.Status)
	}
	if left := built.held(t, outcome.TaskID).Header.RoundsLeft; left != 0 {
		t.Fatalf("the stopped task has %d rounds left, and it stopped because it had none", left)
	}
	return outcome
}

// continueTheTask is the person saying to carry on, which is a message with the
// stopped task's number on it.
func continueTheTask(t *testing.T, built *harness, taskID string) loop.Outcome {
	t.Helper()
	carryOn := built.task("carry on")
	carryOn.ResumeID = taskID
	outcome, err := built.loop.Run(t.Context(), carryOn)
	if err != nil {
		t.Fatalf("the loop could not pick task %s up again: %v", taskID, err)
	}
	return outcome
}

// aTaskThatRunsOutAndThenCarriesOn is four replies: one round of work, the
// report the spent budget buys, and then the round of work and the answer the
// fresh budget pays for.
func aTaskThatRunsOutAndThenCarriesOn() []testkit.Step {
	return []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("I read the notes. The brand file is still to read."),
		callStep("The notes are read. I will read the brand file.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("I read the notes and the brand file. Nothing is left."),
	}
}

// TestAWaitingTaskThePersonAnswersKeepsTheBudgetItHad is the other side of the
// rule, and the one a wait must not buy its way round: a task waiting on a
// question of its own is picked up on the budget it had left, because it was the
// model that stopped the work and not the budget.
func TestAWaitingTaskThePersonAnswersKeepsTheBudgetItHad(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which account should I post from?"),
		answerStep("I posted from the DigiByte account. Nothing is left."),
	}, scriptedTool("read", "the notes"))

	first := built.task("post the anniversary tweet")
	first.Budget = loop.Budget{Rounds: 10, Time: time.Hour}
	waiting, err := built.loop.Run(t.Context(), first)
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}
	if waiting.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting, because the model asked the user something", waiting.Status)
	}
	before := built.held(t, waiting.TaskID).Header.RoundsLeft

	answered := built.task("the DigiByte account")
	answered.ResumeID = waiting.TaskID
	if _, err := built.loop.Run(t.Context(), answered); err != nil {
		t.Fatalf("the loop could not pick the waiting task up again: %v", err)
	}

	held := built.held(t, waiting.TaskID)
	if held.Header.RoundsLeft >= before {
		t.Errorf("the task had %d rounds left before the wait and %d after, and answering a question buys no rounds",
			before, held.Header.RoundsLeft)
	}
	if strings.Contains(strings.Join(held.Work.Situation, "\n"), "carry on") {
		t.Error("the situation of a waiting task says it was asked to carry on, and it was answered rather than continued")
	}
}

// TestAPickedUpTaskStartsWithItsLastRoundsInFrontOfIt pins what a pick-up
// already does, found while chasing the re-read burst of the fifth game
// build's pick-ups: the working context's window is rebuilt from the log, so
// the last rounds' results are in front of the model in full from the first
// request of the new sitting. The burst was the model choosing to read older
// results back, not a window that started empty.
func TestAPickedUpTaskStartsWithItsLastRoundsInFrontOfIt(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read both files.",
			callFor("c1", contract.ToolRead, `{"path":"notes.md"}`),
			callFor("c2", contract.ToolRead, `{"path":"brand.md"}`)),
		answerStep("The budget is spent. What changed: nothing. What I checked: the files. What is left: the post."),
		answerStep("Both files are in front of me. What changed: nothing. What I checked: both. What is left: nothing."),
	}, scriptedTool(contract.ToolRead,
		"the notes say the launch is in March. "+strings.Repeat("More about the launch. ", 8)+"The venue is Berlin.",
		"the brand file says the colour is blue. "+strings.Repeat("More about the brand. ", 8)+"The typeface is Inter."))

	stopped := runOutOfBudget(t, built)
	continueTheTask(t, built, stopped.TaskID)

	// The record's result lines carry only the first words of each result;
	// the venue and the typeface are past them, so they can only come from
	// the replayed round.
	requests := built.model.Requests()
	shown := wholeRequestText(requests[len(requests)-2])
	for _, words := range []string{"The venue is Berlin.", "The typeface is Inter."} {
		if !strings.Contains(shown, words) {
			t.Errorf("the pick-up's first request does not carry %q from the last round of the sitting before, and the model would read it again:\n%s", words, shown)
		}
	}
	if strings.Count(shown, "The venue is Berlin.") != 1 {
		t.Errorf("the replayed result is in the request %d times, want once", strings.Count(shown, "The venue is Berlin."))
	}
}
