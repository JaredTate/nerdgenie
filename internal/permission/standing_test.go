package permission_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAStandingApprovalAllowsUpToItsLimitAndThenAsksAgain(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       2,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := shellRequest(t, "rm -rf /tmp/x")

	for use := 1; use <= 2; use++ {
		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAllow {
			t.Fatalf("use %d of the standing approval was ruled %q, want %q", use, decision.Ruling, contract.RulingAllow)
		}
		if !strings.Contains(decision.Reason, "clear-the-build-folder") {
			t.Errorf("the reason is %q, and it has to name the skill that holds the approval", decision.Reason)
		}
	}

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAsk {
		t.Errorf("the use after the limit was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestAnExpiredStandingApprovalAsksAgain(t *testing.T) {
	clock := testkit.NewFakeClock(theTestTime)
	decider, err := permission.New(contract.DefaultConfig(), clock)
	if err != nil {
		t.Fatalf("building the permission function failed: %v", err)
	}
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := shellRequest(t, "rm -rf /tmp/x")

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAllow {
		t.Fatalf("before the expiry the call was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}

	clock.Advance(2 * time.Hour)

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAsk {
		t.Errorf("after the expiry the call was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestAStandingApprovalCoversOnlyTheFormItNames(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})

	decision := decide(t, decider, shellRequest(t, "git reset --hard origin/main"))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("a call the standing approval does not name was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestARejectionTheUserGaveBeatsAStandingApproval(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := shellRequest(t, "rm -rf /tmp/x")

	if err := decider.Remember(request, contract.AnswerReject, "not that folder"); err != nil {
		t.Fatalf("remembering the rejection failed: %v", err)
	}

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingDeny {
		t.Errorf("after the user refused it, the call was ruled %q, want %q", decision.Ruling, contract.RulingDeny)
	}
}

// badStandingApprovals are the standing approvals that must be refused, each
// with a word the error has to carry.
var badStandingApprovals = []struct {
	named    string
	approval permission.StandingApproval
}{
	{"skill", permission.StandingApproval{ReducedForm: "rm -rf", Limit: 1, Expires: theTestTime.Add(time.Hour)}},
	{"readable form", permission.StandingApproval{Skill: "a-skill", Limit: 1, Expires: theTestTime.Add(time.Hour)}},
	{"a-skill", permission.StandingApproval{Skill: "a-skill", ReducedForm: "rm -rf", Limit: 0, Expires: theTestTime.Add(time.Hour)}},
	{"a-skill", permission.StandingApproval{Skill: "a-skill", ReducedForm: "rm -rf", Limit: 1}},
}

func TestAStandingApprovalTheHarnessCannotUseIsRefusedByName(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, bad := range badStandingApprovals {
		err := decider.RegisterStandingApproval(bad.approval)
		if err == nil {
			t.Errorf("the standing approval %+v was accepted, want an error saying what is missing", bad.approval)
			continue
		}
		if !strings.Contains(err.Error(), bad.named) {
			t.Errorf("the error for %+v is %q, and it has to name %q", bad.approval, err, bad.named)
		}
	}
}

func TestMoreStandingApprovalsThanTheCapAreRefused(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	var lastErr error
	for index := 0; index <= permission.MaxStandingApprovals; index++ {
		lastErr = decider.RegisterStandingApproval(permission.StandingApproval{
			Skill:       "a-skill",
			ReducedForm: "rm -rf",
			Limit:       1,
			Expires:     theTestTime.Add(time.Hour),
		})
	}

	if lastErr == nil {
		t.Fatalf("more than %d standing approvals were accepted, and the cap has to hold", permission.MaxStandingApprovals)
	}
}

// theShippedEntriesAndACallEachCatches is one call for each of the three entries
// the ask-me-first list ships with. A skill's standing approval is not the user's
// yes, so it may not cover any of them, however wide the readable form it names.
var theShippedEntriesAndACallEachCatches = []struct {
	entry    string
	toolName string
	fields   map[string]any
}{
	{contract.AskFirstBulkDelete, contract.ToolShell, map[string]any{"command": "rm -rf /home/jared/coeus"}},
	{contract.AskFirstSudo, contract.ToolShell, map[string]any{"command": "sudo apt install ripgrep"}},
	{contract.AskFirstSpendMoney, contract.ToolWeb, map[string]any{"url": "https://shop.example.com/checkout"}},
}

func TestAStandingApprovalNeverCoversTheAskMeFirstList(t *testing.T) {
	for _, one := range theShippedEntriesAndACallEachCatches {
		decider := newDecider(t, contract.DefaultConfig())
		registerStanding(t, decider, permission.StandingApproval{
			Skill:       "tidy-up",
			ReducedForm: "*",
			Limit:       1000,
			Expires:     theTestTime.Add(time.Hour),
		})
		request := contract.PermissionRequest{ToolName: one.toolName, Input: jsonInput(t, one.fields)}

		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAsk {
			t.Errorf("%s: %q was ruled %q because %q, want %q; the ask-me-first list is read before any standing approval,"+
				" so a skill cannot hand itself the three things the user always sees first",
				one.entry, permission.Reduce(request), decision.Ruling, decision.Reason, contract.RulingAsk)
		}
	}
}

// registerStanding gives the permission function one standing approval and fails
// the test when it will not take it.
func registerStanding(t *testing.T, decider *permission.Decider, approval permission.StandingApproval) {
	t.Helper()
	if err := decider.RegisterStandingApproval(approval); err != nil {
		t.Fatalf("registering the standing approval for %q failed: %v", approval.Skill, err)
	}
}
