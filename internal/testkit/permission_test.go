package testkit_test

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
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

func TestTheFakePermissionKeepsThePermissionContract(t *testing.T) {
	if err := testkit.CheckPermission(context.Background(), testkit.NewFakePermission(contract.RulingAllow)); err != nil {
		t.Fatalf("the fake permission decider does not keep the permission contract: %v", err)
	}
}
