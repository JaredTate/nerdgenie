package permission_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theTestTime is the moment every test in this package starts from, so that an
// expiry is always a plain number of hours away from something fixed.
var theTestTime = time.Date(2026, time.September, 2, 9, 0, 0, 0, time.UTC)

func TestADeciderWithTheShippedListAsksAboutABulkDeleteAndShowsTheCommand(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    jsonInput(t, map[string]any{"command": "rm -rf /tmp/x"}),
	})

	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("a bulk delete was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if decision.Reason != contract.AskFirstBulkDelete {
		t.Errorf("the reason is %q, want the entry's own name %q", decision.Reason, contract.AskFirstBulkDelete)
	}
	if !strings.Contains(decision.PreviewText, "rm -rf /tmp/x") {
		t.Errorf("the preview is %q, and it has to show the whole command", decision.PreviewText)
	}
}

func TestADeciderAllowsACallNoRuleCoversAndSaysSo(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolRead,
		Input:    jsonInput(t, map[string]any{"path": "/home/jared/notes.md"}),
	})

	if decision.Ruling != contract.RulingAllow {
		t.Fatalf("reading a file was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
	if !strings.Contains(decision.Reason, "no rule") {
		t.Errorf("the reason is %q, and it has to say that no rule covers the call", decision.Reason)
	}
}

func TestAnEmptyAskMeFirstListAllowsEverything(t *testing.T) {
	decider := newDecider(t, contract.Config{})

	for _, command := range []string{"rm -rf /tmp/x", "sudo apt install ripgrep", "git reset --hard"} {
		decision := decide(t, decider, contract.PermissionRequest{
			ToolName: contract.ToolShell,
			Input:    jsonInput(t, map[string]any{"command": command}),
		})
		if decision.Ruling != contract.RulingAllow {
			t.Errorf("with an empty list, %q was ruled %q, want %q", command, decision.Ruling, contract.RulingAllow)
		}
	}
}

func TestARuleFromTheConfigurationWinsOverAShippedEntryBecauseItComesLast(t *testing.T) {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolShell, Pattern: "*rm -r*", Action: contract.RulingAllow},
		{Tool: contract.ToolShell, Pattern: "*git reset --hard*", Action: contract.RulingDeny},
	}
	decider := newDecider(t, configuration)

	allowed := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if allowed.Ruling != contract.RulingAllow {
		t.Errorf("the rule in config.toml that allows gave %q, want %q", allowed.Ruling, contract.RulingAllow)
	}
	if !strings.Contains(allowed.Reason, "*rm -r*") {
		t.Errorf("the reason is %q, and it has to name the rule from config.toml that won", allowed.Reason)
	}

	denied := decide(t, decider, shellRequest(t, "git reset --hard origin/main"))
	if denied.Ruling != contract.RulingDeny {
		t.Errorf("the rule in config.toml that denies gave %q, want %q", denied.Ruling, contract.RulingDeny)
	}
}

func TestDroppingOneEntryFromTheConfigurationLeavesTheOthersOn(t *testing.T) {
	configuration := contract.DefaultConfig()
	configuration.AskMeFirst = []string{contract.AskFirstSpendMoney}
	decider := newDecider(t, configuration)

	dropped := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if dropped.Ruling != contract.RulingAllow {
		t.Errorf("an entry the user took off the list was ruled %q, want %q", dropped.Ruling, contract.RulingAllow)
	}

	kept := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolBrowserAct,
		Input:    jsonInput(t, map[string]any{"intent": "buy the blue kayak"}),
	})
	if kept.Ruling != contract.RulingAsk {
		t.Errorf("an entry the user kept was ruled %q, want %q", kept.Ruling, contract.RulingAsk)
	}
}

func TestADeciderRefusesAConfigurationItCannotUse(t *testing.T) {
	clock := testkit.NewFakeClock(theTestTime)

	unknownEntry := contract.DefaultConfig()
	unknownEntry.AskMeFirst = []string{"whatever I feel like"}
	_, err := permission.New(unknownEntry, clock)
	if err == nil {
		t.Error("an ask-me-first entry nobody shipped was accepted, want an error naming it")
	} else if !strings.Contains(err.Error(), "whatever I feel like") {
		t.Errorf("the error is %q, and it has to name the entry config.toml asked for", err)
	}

	badAction := contract.DefaultConfig()
	badAction.PermissionRules = []contract.PermissionRule{{Tool: "*", Pattern: "*", Action: "maybe"}}
	if _, err := permission.New(badAction, clock); err == nil {
		t.Error("a rule with an action that is not one of the three was accepted, want an error")
	}

	if _, err := permission.New(contract.DefaultConfig(), nil); err == nil {
		t.Error("a decider was built with no clock, and it needs one to know when a standing approval has expired")
	}
}

func TestADeciderStopsWhenTheContextIsAlreadyCancelled(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := decider.Decide(ctx, shellRequest(t, "rm -rf /tmp/x")); err == nil {
		t.Fatal("a cancelled turn still got a ruling, want the context's own error")
	}
}

func TestADeciderKeepsThePermissionContract(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	if err := testkit.CheckPermission(context.Background(), decider); err != nil {
		t.Fatalf("the permission function does not keep the permission contract: %v", err)
	}
}

func TestThePreviewOfAWriteThatEmptiesAFileNamesThePathAndTheSize(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.log")
	writeFileOfSize(t, big, permission.EmptiesFileOverBytes+1)
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": big, "content": ""}),
	})

	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("emptying a big file was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if !strings.Contains(decision.PreviewText, big) {
		t.Errorf("the preview is %q, and it has to name the file", decision.PreviewText)
	}
	if !strings.Contains(decision.PreviewText, "4097") {
		t.Errorf("the preview is %q, and it has to say how big the file is now", decision.PreviewText)
	}
}

func TestThePreviewOfABrowserActionNamesTheIntentAndTheElement(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolBrowserClick,
		Input:    jsonInput(t, map[string]any{"intent": "pay now", "element": "e12"}),
	})

	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("clicking to pay was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if !strings.Contains(decision.PreviewText, "pay now") || !strings.Contains(decision.PreviewText, "e12") {
		t.Errorf("the preview is %q, and it has to name both the intent and the element", decision.PreviewText)
	}
}

func TestThePreviewOfAnEscalatedCommandSaysItAsksForAdministratorPowers(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    jsonInput(t, map[string]any{"command": "apt install ripgrep", "escalate": true}),
	})

	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("an escalated command was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if !strings.Contains(decision.PreviewText, "administrator") {
		t.Errorf("the preview is %q, and it has to say that the command asks for administrator powers", decision.PreviewText)
	}
}

func TestARuleFromTheConfigurationIsDescribedByItself(t *testing.T) {
	configuration := contract.Config{
		PermissionRules: []contract.PermissionRule{{Tool: contract.ToolShell, Pattern: "*git push*", Action: contract.RulingDeny}},
	}
	decider := newDecider(t, configuration)

	decision := decide(t, decider, shellRequest(t, "git push --force"))

	if decision.Ruling != contract.RulingDeny {
		t.Fatalf("the rule gave %q, want %q", decision.Ruling, contract.RulingDeny)
	}
	for _, wanted := range []string{"*git push*", contract.ToolShell, string(contract.RulingDeny)} {
		if !strings.Contains(decision.Reason, wanted) {
			t.Errorf("the reason is %q, and a rule with no words of its own has to be described by %q", decision.Reason, wanted)
		}
	}
}

// newDecider builds a permission function from a configuration, on a clock a
// test controls.
func newDecider(t *testing.T, configuration contract.Config) *permission.Decider {
	t.Helper()
	decider, err := permission.New(configuration, testkit.NewFakeClock(theTestTime))
	if err != nil {
		t.Fatalf("building the permission function failed: %v", err)
	}
	return decider
}

// decide asks the permission function about one call and fails the test when the
// question itself could not be answered.
func decide(t *testing.T, decider *permission.Decider, request contract.PermissionRequest) contract.PermissionDecision {
	t.Helper()
	decision, err := decider.Decide(context.Background(), request)
	if err != nil {
		t.Fatalf("deciding on a %s call failed: %v", request.ToolName, err)
	}
	return decision
}

// shellRequest is one shell command put to the permission function.
func shellRequest(t *testing.T, command string) contract.PermissionRequest {
	t.Helper()
	return contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    jsonInput(t, map[string]any{"command": command}),
	}
}
