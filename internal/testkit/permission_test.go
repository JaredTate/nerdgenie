package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheFakePermissionUsesItsDefaultRulingWhenNoRuleMatches(t *testing.T) {
	permission := testkit.NewFakePermission(contract.RulingAllow)

	decision, err := permission.Decide(context.Background(), contract.PermissionRequest{ToolName: contract.ToolRead})
	if err != nil {
		t.Fatalf("deciding on a read failed: %v", err)
	}
	if decision.Ruling != contract.RulingAllow {
		t.Errorf("the ruling on a read is %q, want %q", decision.Ruling, contract.RulingAllow)
	}
}

func TestTheFakePermissionUsesTheRuleScriptedForATool(t *testing.T) {
	permission := testkit.NewFakePermission(contract.RulingAllow)
	permission.Rule(contract.ToolShell, contract.PermissionDecision{
		Ruling:      contract.RulingAsk,
		Reason:      contract.AskFirstSudo,
		PreviewText: "sudo apt install ripgrep",
	})

	decision, err := permission.Decide(context.Background(), contract.PermissionRequest{ToolName: contract.ToolShell})
	if err != nil {
		t.Fatalf("deciding on a shell command failed: %v", err)
	}
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("the ruling on a shell command is %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if decision.PreviewText == "" {
		t.Error("a ruling of ask came back with no preview text, and the user must see what is about to happen")
	}
}

func TestTheFakePermissionRecordsEveryRequestAndEveryAnswer(t *testing.T) {
	ctx := context.Background()
	permission := testkit.NewFakePermission(contract.RulingAsk)
	request := contract.PermissionRequest{ToolName: contract.ToolWrite, CommandPrefix: "write notes.md"}

	if _, err := permission.Decide(ctx, request); err != nil {
		t.Fatalf("deciding failed: %v", err)
	}
	if err := permission.Remember(request, contract.AnswerAlways, "the user said always"); err != nil {
		t.Fatalf("remembering the answer failed: %v", err)
	}

	if len(permission.Requests()) != 1 {
		t.Errorf("the fake recorded %d requests, want 1", len(permission.Requests()))
	}
	answers := permission.Answers()
	if len(answers) != 1 || answers[0].Answer != contract.AnswerAlways {
		t.Errorf("the fake recorded the answers %+v, want one \"always\"", answers)
	}
}

func TestTheFakePermissionRefusesAnAnswerItDoesNotUnderstand(t *testing.T) {
	permission := testkit.NewFakePermission(contract.RulingAsk)

	err := permission.Remember(contract.PermissionRequest{ToolName: contract.ToolRead}, "maybe", "")
	if err == nil {
		t.Fatal("an answer of \"maybe\" was accepted, want an error naming the three answers")
	}
}

func TestAnAnswerOfAlwaysAllowsTheSameRequestForTheRestOfTheSession(t *testing.T) {
	ctx := context.Background()
	permission := testkit.NewFakePermission(contract.RulingAsk)
	request := contract.PermissionRequest{ToolName: contract.ToolShell, CommandPrefix: "git push"}

	if err := permission.Remember(request, contract.AnswerAlways, ""); err != nil {
		t.Fatalf("remembering an answer of always failed: %v", err)
	}
	decision, err := permission.Decide(ctx, request)

	if err != nil {
		t.Fatalf("deciding after an always failed: %v", err)
	}
	if decision.Ruling != contract.RulingAllow {
		t.Errorf("the ruling after the user said always is %q, want %q", decision.Ruling, contract.RulingAllow)
	}

	other := contract.PermissionRequest{ToolName: contract.ToolShell, CommandPrefix: "rm -rf"}
	if decision, err := permission.Decide(ctx, other); err != nil || decision.Ruling != contract.RulingAsk {
		t.Errorf("an always for %q also allowed %q, and it covers the same request only", request.CommandPrefix, other.CommandPrefix)
	}
}

func TestAnAnswerOfRejectDeniesTheSameRequestWithItsReason(t *testing.T) {
	ctx := context.Background()
	permission := testkit.NewFakePermission(contract.RulingAllow)
	request := contract.PermissionRequest{ToolName: contract.ToolShell, CommandPrefix: "rm -rf"}

	if err := permission.Remember(request, contract.AnswerReject, "never delete a folder without asking me"); err != nil {
		t.Fatalf("remembering an answer of reject failed: %v", err)
	}
	decision, err := permission.Decide(ctx, request)

	if err != nil {
		t.Fatalf("deciding after a reject failed: %v", err)
	}
	if decision.Ruling != contract.RulingDeny {
		t.Errorf("the ruling after the user rejected it is %q, want %q", decision.Ruling, contract.RulingDeny)
	}
	if !strings.Contains(decision.Reason, "never delete a folder without asking me") {
		t.Errorf("the refusal says %q, and it must carry the user's own reason", decision.Reason)
	}
}

func TestAnAnswerOfOnceChangesNothing(t *testing.T) {
	ctx := context.Background()
	permission := testkit.NewFakePermission(contract.RulingAsk)
	request := contract.PermissionRequest{ToolName: contract.ToolWrite, CommandPrefix: "write notes.md"}

	before, err := permission.Decide(ctx, request)
	if err != nil {
		t.Fatalf("deciding before the answer failed: %v", err)
	}
	if err := permission.Remember(request, contract.AnswerOnce, ""); err != nil {
		t.Fatalf("remembering an answer of once failed: %v", err)
	}
	after, err := permission.Decide(ctx, request)

	if err != nil {
		t.Fatalf("deciding after the answer failed: %v", err)
	}
	if after.Ruling != before.Ruling {
		t.Errorf("the ruling changed from %q to %q after an answer of once, and once covers one call only",
			before.Ruling, after.Ruling)
	}
}

func TestARejectedCallNeedsAReason(t *testing.T) {
	permission := testkit.NewFakePermission(contract.RulingAllow)

	err := permission.Remember(contract.PermissionRequest{ToolName: contract.ToolShell}, contract.AnswerReject, "")

	if err == nil {
		t.Fatal("a reject with no reason was accepted, and the model is always told why it was refused")
	}
}

func TestTheFakePermissionKeepsThePermissionContract(t *testing.T) {
	if err := testkit.CheckPermission(context.Background(), testkit.NewFakePermission(contract.RulingAllow)); err != nil {
		t.Fatalf("the fake permission decider does not keep the permission contract: %v", err)
	}
}
