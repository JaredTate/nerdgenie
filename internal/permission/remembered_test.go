package permission_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
)

func TestAnswerOnceDoesNotStickAndTheSameCallAsksAgain(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")

	if err := decider.Remember(request, contract.AnswerOnce, ""); err != nil {
		t.Fatalf("remembering an answer of once failed: %v", err)
	}

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAsk {
		t.Errorf("after an answer of once, the same call was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestAnswerAlwaysSticksForTheRestOfTheSession(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")

	if err := decider.Remember(request, contract.AnswerAlways, ""); err != nil {
		t.Fatalf("remembering an answer of always failed: %v", err)
	}

	decision := decide(t, decider, request)
	if decision.Ruling != contract.RulingAllow {
		t.Fatalf("after an answer of always, the same call was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
	if !strings.Contains(decision.Reason, "session") {
		t.Errorf("the reason is %q, and it has to say the answer holds for this session", decision.Reason)
	}
}

func TestAnswerAlwaysCoversOnlyCallsThatReduceToTheSameThing(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	if err := decider.Remember(shellRequest(t, "rm -rf /tmp/x"), contract.AnswerAlways, ""); err != nil {
		t.Fatalf("remembering an answer of always failed: %v", err)
	}

	sameForm := decide(t, decider, shellRequest(t, "rm -rf /home/user/scratch"))
	if sameForm.Ruling != contract.RulingAllow {
		t.Errorf("another command with the same readable form was ruled %q, want %q", sameForm.Ruling, contract.RulingAllow)
	}

	otherForm := decide(t, decider, shellRequest(t, "git reset --hard origin/main"))
	if otherForm.Ruling != contract.RulingAsk {
		t.Errorf("a command with a different readable form was ruled %q, want %q", otherForm.Ruling, contract.RulingAsk)
	}
}

func TestAnswerRejectRefusesTheCallAndGivesTheModelTheReason(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")

	if err := decider.Remember(request, contract.AnswerReject, "that folder is the only copy of the photos"); err != nil {
		t.Fatalf("remembering an answer of reject failed: %v", err)
	}

	decision := decide(t, decider, request)
	if decision.Ruling != contract.RulingDeny {
		t.Fatalf("after a rejection, the same call was ruled %q, want %q", decision.Ruling, contract.RulingDeny)
	}
	if !strings.Contains(decision.Reason, "the only copy of the photos") {
		t.Errorf("the reason is %q, and it has to carry the words the user gave", decision.Reason)
	}
}

func TestAnswerRejectWithNoReasonStillSaysTheUserRefusedIt(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")

	if err := decider.Remember(request, contract.AnswerReject, ""); err != nil {
		t.Fatalf("remembering an answer of reject failed: %v", err)
	}

	decision := decide(t, decider, request)
	if decision.Ruling != contract.RulingDeny || decision.Reason == "" {
		t.Errorf("a rejection with no reason gave %q because %q, want a deny that still says who refused it", decision.Ruling, decision.Reason)
	}
}

func TestAnAnswerTheHarnessDoesNotUnderstandIsRefused(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	err := decider.Remember(shellRequest(t, "rm -rf /tmp/x"), "maybe", "")
	if err == nil {
		t.Fatal("an answer of \"maybe\" was accepted, want an error naming once, always, and reject")
	}
}

func TestMoreRememberedAnswersThanTheCapAreRefusedWithSomethingToDo(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	var lastErr error
	for index := 0; index <= permission.MaxRememberedAnswers; index++ {
		request := contract.PermissionRequest{
			ToolName: contract.ToolShell,
			Input:    jsonInput(t, map[string]any{"command": fmt.Sprintf("program%d -rf /tmp/x", index)}),
		}
		lastErr = decider.Remember(request, contract.AnswerAlways, "")
	}

	if lastErr == nil {
		t.Fatalf("more than %d remembered answers were accepted, and the cap has to hold", permission.MaxRememberedAnswers)
	}
}
