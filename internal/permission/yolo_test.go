package permission_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
)

// TestYoloIsOffUntilSomebodyTurnsItOn holds that a fresh permission function
// asks about a bulk delete exactly as it did before the switch existed.
func TestYoloIsOffUntilSomebodyTurnsItOn(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	if decider.YoloIsOn() {
		t.Fatal("yolo is on in a fresh permission function, and it starts off")
	}
	decision := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("with yolo off, a bulk delete was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

// TestWithYoloOnAnAskBecomesAnAllowThatSaysSo holds the whole point of the
// switch: a call the ask-me-first list would have stopped for runs without
// asking, and the reason the log records says it was allowed by yolo and what
// it would have asked about.
func TestWithYoloOnAnAskBecomesAnAllowThatSaysSo(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	decider.UseYolo(true)

	if !decider.YoloIsOn() {
		t.Fatal("yolo was turned on and the permission function says it is off")
	}
	decision := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))

	if decision.Ruling != contract.RulingAllow {
		t.Fatalf("with yolo on, a bulk delete was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
	if !strings.Contains(decision.Reason, permission.AllowedByYolo) {
		t.Errorf("the reason is %q, and the log line has to say %q", decision.Reason, permission.AllowedByYolo)
	}
	if !strings.Contains(decision.Reason, contract.AskFirstBulkDelete) {
		t.Errorf("the reason is %q, and it has to say what would have been asked about, %q", decision.Reason, contract.AskFirstBulkDelete)
	}
	if !strings.Contains(decision.PreviewText, "rm -rf /tmp/x") {
		t.Errorf("the preview is %q, and the log holds the same preview an approved call holds", decision.PreviewText)
	}
}

// TestYoloCoversEverythingThatWouldHaveAsked holds that the switch is not about
// one entry on the list: sudo, spending money, and a command the harness cannot
// read to the end all run without asking while it is on.
func TestYoloCoversEverythingThatWouldHaveAsked(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	decider.UseYolo(true)

	for _, request := range []contract.PermissionRequest{
		shellRequest(t, "sudo apt install ripgrep"),
		shellRequest(t, "echo $(cat /etc/hostname)"),
		{ToolName: contract.ToolBrowserClick, Input: jsonInput(t, map[string]any{"intent": "buy the tickets", "element": "the checkout button"})},
	} {
		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAllow {
			t.Errorf("with yolo on, %s %s was ruled %q, want %q", request.ToolName, request.Input, decision.Ruling, contract.RulingAllow)
		}
		if !strings.Contains(decision.Reason, permission.AllowedByYolo) {
			t.Errorf("the reason for %s %s is %q, want it to say %q", request.ToolName, request.Input, decision.Reason, permission.AllowedByYolo)
		}
	}
}

// TestYoloDoesNotTouchARuleThatRefuses holds the one thing the switch never
// does: a rule the user wrote that says never is still never.
func TestYoloDoesNotTouchARuleThatRefuses(t *testing.T) {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolShell, Pattern: "*git reset --hard*", Action: contract.RulingDeny},
	}
	decider := newDecider(t, configuration)
	decider.UseYolo(true)

	decision := decide(t, decider, shellRequest(t, "git reset --hard origin/main"))

	if decision.Ruling != contract.RulingDeny {
		t.Errorf("with yolo on, a call the user's rule refuses was ruled %q, want %q", decision.Ruling, contract.RulingDeny)
	}
	if strings.Contains(decision.Reason, permission.AllowedByYolo) {
		t.Errorf("the reason %q says yolo allowed a call that was refused", decision.Reason)
	}
}

// TestYoloDoesNotUndoANoTheUserGaveThisSession holds that a reject the user
// answered earlier is the user's own word, which the switch does not overrule.
func TestYoloDoesNotUndoANoTheUserGaveThisSession(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")
	if err := decider.Remember(request, contract.AnswerReject, "not that folder"); err != nil {
		t.Fatalf("remembering the user's no failed: %v", err)
	}
	decider.UseYolo(true)

	decision := decide(t, decider, request)

	if decision.Ruling != contract.RulingDeny {
		t.Errorf("with yolo on, a call the user refused this session was ruled %q, want %q", decision.Ruling, contract.RulingDeny)
	}
}

// TestYoloLetsAnUnattendedRunThroughInsteadOfStoppingIt holds that a scheduled
// job, which would have stopped and reported what it needed, runs while the
// switch is on, because nothing needs an answer any more.
func TestYoloLetsAnUnattendedRunThroughInsteadOfStoppingIt(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	decider.UseYolo(true)
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	decision := decide(t, decider, request)

	if decision.Ruling != contract.RulingAllow {
		t.Errorf("with yolo on, an unattended bulk delete was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
}

// TestTurningYoloOffBringsTheAskBack holds that the switch is a switch: off
// again, the same call asks again, and the answer is not remembered from the
// time it ran without asking.
func TestTurningYoloOffBringsTheAskBack(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	decider.UseYolo(true)
	if ran := decide(t, decider, shellRequest(t, "rm -rf /tmp/x")); ran.Ruling != contract.RulingAllow {
		t.Fatalf("with yolo on, the call was ruled %q, want %q", ran.Ruling, contract.RulingAllow)
	}

	decider.UseYolo(false)

	if decider.YoloIsOn() {
		t.Fatal("yolo was turned off and the permission function says it is on")
	}
	decision := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("with yolo off again, the call was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

// TestYoloLeavesAnOrdinaryCallsReasonAlone holds that a call nothing would
// have asked about runs for the reason it always ran for, so the log does not
// say yolo allowed something that needed no allowing.
func TestYoloLeavesAnOrdinaryCallsReasonAlone(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	decider.UseYolo(true)

	decision := decide(t, decider, shellRequest(t, "git status"))

	if decision.Ruling != contract.RulingAllow {
		t.Fatalf("an ordinary command was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
	if strings.Contains(decision.Reason, permission.AllowedByYolo) {
		t.Errorf("the reason %q credits yolo for a call that never needed a yes", decision.Reason)
	}
}
