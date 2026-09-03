package loop_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestAWaitingTaskIsPickedUpAgainFromItsCheckpoint proves the promise that a
// task can be put down and picked up later: the question ends the turn, and the
// user's next message carries on from the last checkpoint with the record
// intact.
func TestAWaitingTaskIsPickedUpAgainFromItsCheckpoint(t *testing.T) {
	ask := "post the anniversary tweet"
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":["the post is up"]}`)),
		answerStep("There are two accounts. Which one should I post from?"),
		callStep("The user has answered, so I will point the line at the result.",
			taskCall("c2t", `{"doneWhen":[{"text":"the post is up","done":true,"resultId":"r1"}]}`)),
		answerStep("Posted. What changed: one post. What I checked: the account. What is left: nothing."),
	}, scriptedTool("read", "the notes name two accounts"))

	waiting := built.ask(t, ask)
	if waiting.Status != contract.StatusWaiting {
		t.Fatalf("the first turn ended %q, want waiting", waiting.Status)
	}

	answered := built.task("the DigiByte account")
	answered.ResumeID = waiting.TaskID
	outcome, err := built.loop.Run(t.Context(), answered)
	if err != nil {
		t.Fatalf("the loop could not pick the task up again: %v", err)
	}

	if outcome.TaskID != waiting.TaskID {
		t.Errorf("the task was picked up as %q, want the same task %q", outcome.TaskID, waiting.TaskID)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	held := built.held(t, outcome.TaskID)
	if held.Goal.Ask != ask {
		t.Errorf("the ask now reads %q, and the user wrote %q", held.Goal.Ask, ask)
	}
	if len(held.Work.Results) != 3 {
		t.Errorf("the record holds %d results, want the read and the two record writes, all still there after the wait",
			len(held.Work.Results))
	}
}

// TestThePickedUpTaskKeepsTheBudgetItHadLeft proves the budget is the record's
// and not the turn's, so a task cannot buy itself more rounds by waiting.
func TestThePickedUpTaskKeepsTheBudgetItHadLeft(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which account should I post from?"),
		answerStep("Nothing is left to do. Shall I stop?"),
	}, scriptedTool("read", "the notes"))

	// The task runs under a budget of its own, because the caps set none and
	// a task with no budget has nothing a wait could buy.
	first := built.task("post the anniversary tweet")
	first.Budget = loop.Budget{Rounds: 10, Time: time.Hour}
	waiting, err := built.loop.Run(t.Context(), first)
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}
	before := built.held(t, waiting.TaskID).Header.RoundsLeft

	answered := built.task("the DigiByte account")
	answered.ResumeID = waiting.TaskID
	if _, err := built.loop.Run(t.Context(), answered); err != nil {
		t.Fatalf("the loop could not pick the task up again: %v", err)
	}

	after := built.held(t, waiting.TaskID).Header.RoundsLeft
	if after >= before {
		t.Errorf("the task had %d rounds left before the wait and %d after, and a wait buys no rounds", before, after)
	}
}

// TestPickingUpATaskThatIsNotThereSaysSo proves the loop answers plainly when
// the number names nothing.
func TestPickingUpATaskThatIsNotThereSaysSo(t *testing.T) {
	built := newHarness(t, nil)

	task := built.task("carry on")
	task.ResumeID = "404"
	if _, err := built.loop.Run(t.Context(), task); err == nil {
		t.Error("the loop picked up a task the log does not hold")
	} else if !strings.Contains(err.Error(), "404") {
		t.Errorf("the refusal reads %q, and it should name the task that is not there", err)
	}
}

// TestATaskNeedsAChannelToAnswerOn proves the loop refuses work it could not
// report on.
func TestATaskNeedsAChannelToAnswerOn(t *testing.T) {
	built := newHarness(t, nil)

	if _, err := built.loop.Run(t.Context(), loop.Task{Message: contract.Inbound{Text: "hello"}}); err == nil {
		t.Error("the loop took a task with nowhere to answer")
	}
}
