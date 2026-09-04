package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// oneShellCall is the script every permission test uses: one command, then a
// question that ends the turn.
func oneShellCall() []testkit.Step {
	return []testkit.Step{
		callStep("I will delete the folder.", callFor("c1", "shell", `{"command":"rm -rf /tmp/old"}`)),
		answerStep("Shall I go on?"),
	}
}

// TestACallOnTheAskMeFirstListIsPreviewedAndThenRuns proves the permission step:
// the user sees exactly what is about to happen and answers before it runs.
func TestACallOnTheAskMeFirstListIsPreviewedAndThenRuns(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "shell", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the folder is gone")
	built := newHarness(t, oneShellCall(), shell)
	built.rulings.Rule("shell", contract.PermissionDecision{
		Ruling: contract.RulingAsk, Reason: "deleting many files at once", PreviewText: "rm -rf /tmp/old",
	})
	built.channel.AnswerPreviewsWith(contract.AnswerOnce)

	built.ask(t, "delete the old folder")

	previews := built.channel.Previews()
	if len(previews) != 1 || previews[0].Body != "rm -rf /tmp/old" {
		t.Fatalf("the user was shown %v, want one preview of exactly what was about to happen", previews)
	}
	if len(shell.Inputs()) != 1 {
		t.Errorf("the tool ran %d times, want the one the user approved", len(shell.Inputs()))
	}
	if answers := built.rulings.Answers(); len(answers) != 1 || answers[0].Answer != contract.AnswerOnce {
		t.Errorf("the permission function was told %v, want the answer the user gave", answers)
	}
}

// TestAPreviewTheUserRefusesStopsTheCallAndTellsTheModel proves the reject
// answer: the call never runs and the model is told why.
func TestAPreviewTheUserRefusesStopsTheCallAndTellsTheModel(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "shell", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the folder is gone")
	built := newHarness(t, oneShellCall(), shell)
	built.rulings.Rule("shell", contract.PermissionDecision{
		Ruling: contract.RulingAsk, Reason: "deleting many files at once", PreviewText: "rm -rf /tmp/old",
	})
	built.channel.AnswerPreviewsWith(contract.AnswerReject)

	built.ask(t, "delete the old folder")

	if len(shell.Inputs()) != 0 {
		t.Errorf("the tool ran %d times, and the user said no", len(shell.Inputs()))
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "was not allowed") {
		t.Error("the model was never told that the call was refused")
	}
}

// TestTheUsersOwnReasonForSayingNoReachesTheModel proves the words the user
// gave when they refused are what the model is told.
func TestTheUsersOwnReasonForSayingNoReachesTheModel(t *testing.T) {
	built := newHarness(t, oneShellCall(), scriptedTool("shell", "the folder is gone"))
	built.rulings.Rule("shell", contract.PermissionDecision{
		Ruling: contract.RulingAsk, Reason: "deleting many files at once", PreviewText: "rm -rf /tmp/old",
	})
	built.channel.AnswerPreviewsWithReason("that folder is the one I am working in")

	built.ask(t, "delete the old folder")

	if !strings.Contains(requestsJoined(built.model.Requests()), "that folder is the one I am working in") {
		t.Error("the model was never told the user's own reason for saying no")
	}
	answers := built.rulings.Answers()
	if len(answers) != 1 || answers[0].Reason != "that folder is the one I am working in" {
		t.Errorf("the permission function was told %v, want the user's own reason", answers)
	}
}

// TestADeniedCallNeverRuns proves a call the rulebook denies outright.
func TestADeniedCallNeverRuns(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "shell", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the folder is gone")
	built := newHarness(t, oneShellCall(), shell)
	built.rulings.Rule("shell", contract.PermissionDecision{
		Ruling: contract.RulingDeny, Reason: "this machine never runs rm -rf",
	})

	built.ask(t, "delete the old folder")

	if len(shell.Inputs()) != 0 {
		t.Errorf("the tool ran %d times, and the rulebook denied it", len(shell.Inputs()))
	}
	if len(built.channel.Previews()) != 0 {
		t.Error("a denied call was previewed, and only a call that asks is previewed")
	}
}

// TestAnUnattendedRunThatNeedsAnAnswerStopsAndReports proves the rule for a job
// nobody is watching: a call that would have asked stops the task instead.
func TestAnUnattendedRunThatNeedsAnAnswerStopsAndReports(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "shell", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the folder is gone")
	built := newHarness(t, oneShellCall(), shell)
	built.rulings.Rule("shell", contract.PermissionDecision{
		Ruling: contract.RulingStop, Reason: "deleting many files at once needs a yes and nobody is there",
	})

	task := built.task("delete the old folder")
	task.Unattended = true
	outcome, err := built.loop.Run(t.Context(), task)
	if err != nil {
		t.Fatalf("the loop could not run the unattended task: %v", err)
	}

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because nobody was there to answer", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "needs a yes and nobody is there") {
		t.Errorf("the user was sent %v, want a report of what needed an answer", built.channel.Sent())
	}
	if len(shell.Inputs()) != 0 {
		t.Error("the tool ran even though the run was unattended and the call needed a yes")
	}
}

// TestEveryRulingIsWrittenIntoTheLog proves the promise that everything is
// logged, which is what a permission decision has to be.
func TestEveryRulingIsWrittenIntoTheLog(t *testing.T) {
	built := newHarness(t, oneShellCall(), scriptedTool("shell", "the folder is gone"))

	built.ask(t, "delete the old folder")

	rulings := built.eventsOfKind(t, contract.EventPermissionDecision)
	if len(rulings) != 1 {
		t.Fatalf("the log holds %d permission decisions, want the one the call needed", len(rulings))
	}
	if !strings.Contains(string(rulings[0].Body), "allow") {
		t.Errorf("the ruling in the log reads %s, and it must say what was decided", rulings[0].Body)
	}
}

// TestTheUserIsAskedWithTheCallWhenTheRulebookGivesNoPreview proves the preview
// always shows something the user can judge.
func TestTheUserIsAskedWithTheCallWhenTheRulebookGivesNoPreview(t *testing.T) {
	built := newHarness(t, oneShellCall(), scriptedTool("shell", "the folder is gone"))
	built.rulings.Rule("shell", contract.PermissionDecision{Ruling: contract.RulingAsk, Reason: "spending money"})

	built.ask(t, "delete the old folder")

	previews := built.channel.Previews()
	if len(previews) != 1 || !strings.Contains(previews[0].Body, "rm -rf /tmp/old") {
		t.Errorf("the user was shown %v, want the call itself when the rulebook wrote no preview", previews)
	}
}
