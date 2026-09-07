package loop_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheEndOfEveryReplyIsSaidBeforeItsCallsRun: the socket holds back the
// last sixty-four runes of a streamed reply for the redactor, and nothing told
// it when the reply had ended, so the held tail reached the screen only when
// the next round's words pushed it out, after the tool line had landed. The
// loop says when a reply is complete, before it runs the reply's calls, so
// the socket can send the tail first.
func TestTheEndOfEveryReplyIsSaidBeforeItsCallsRun(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{
		Name:        contract.ToolShell,
		Description: "A tool the test scripted, which answers with what the test gave it.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}, "finished with exit code 0\nhello\n")
	built := newHarness(t, []testkit.Step{
		callStep("I will greet.", callFor("c1", contract.ToolShell, `{"command":"echo hello"}`)),
		answerStep("Greeted. What changed: nothing. What I checked: the greeting. What is left: nothing."),
	}, shell)
	toolRunsWhenEnded := []int{}
	built.onReplyEnded = func() {
		toolRunsWhenEnded = append(toolRunsWhenEnded, len(shell.Inputs()))
	}

	built.ask(t, "say hello")

	if len(toolRunsWhenEnded) != 2 {
		t.Fatalf("the end of a reply was said %d times over two rounds, want once a round", len(toolRunsWhenEnded))
	}
	if toolRunsWhenEnded[0] != 0 {
		t.Errorf("the first reply's end was said after its call had run (%d runs), and it has to come first", toolRunsWhenEnded[0])
	}
}
