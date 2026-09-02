package permission_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
)

func TestAnUnattendedRunThatHitsTheListStopsAndShowsWhatItWouldHaveAsked(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	decision := decide(t, decider, request)

	if !permission.StoppedForNobodyToAsk(decision) {
		t.Fatalf("an unattended run was ruled %q with the preview %q, want the stop verdict", decision.Ruling, decision.PreviewText)
	}
	if decision.Ruling != contract.RulingDeny {
		t.Errorf("the stop verdict was ruled %q, want %q, because nothing may run when nobody can answer", decision.Ruling, contract.RulingDeny)
	}
	if !strings.Contains(decision.PreviewText, "rm -rf /tmp/x") {
		t.Errorf("the stop verdict carries the preview %q, and it has to say what would have been asked about", decision.PreviewText)
	}
	if !strings.Contains(decision.Reason, "nobody") {
		t.Errorf("the reason is %q, and it has to say that nobody was there to answer", decision.Reason)
	}
	if !strings.Contains(decision.Reason, contract.AskFirstBulkDelete) {
		t.Errorf("the reason is %q, and it has to name the ask-me-first entry that caught the call", decision.Reason)
	}
}

func TestAnUnattendedRunThatHitsNothingOnTheListJustRuns(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())
	request := shellRequest(t, "git commit -m \"the nightly report\"")
	request.Unattended = true

	decision := decide(t, decider, request)

	if decision.Ruling != contract.RulingAllow {
		t.Errorf("an unattended run of an ordinary command was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
	if permission.StoppedForNobodyToAsk(decision) {
		t.Error("an ordinary unattended command came back as the stop verdict, and only a call on the list stops")
	}
}

func TestAnUnattendedRunUsesTheAnswersAndApprovalsItAlreadyHas(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       1,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAllow {
		t.Errorf("an unattended run with a standing approval was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}

	if err := decider.Remember(request, contract.AnswerAlways, ""); err != nil {
		t.Fatalf("remembering an answer of always failed: %v", err)
	}
	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAllow {
		t.Errorf("an unattended run the user already allowed was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
}

func TestAPlainDenyIsNotTheStopVerdict(t *testing.T) {
	settings := permission.DefaultSettings()
	settings.Rules = []permission.Rule{
		{Tool: contract.ToolShell, Pattern: "*rm -r*", Action: contract.RulingDeny, Reason: "never delete folders"},
	}
	decider := newDecider(t, settings)
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	decision := decide(t, decider, request)
	if decision.Ruling != contract.RulingDeny {
		t.Fatalf("a rule that denies gave %q, want %q", decision.Ruling, contract.RulingDeny)
	}
	if permission.StoppedForNobodyToAsk(decision) {
		t.Error("a plain deny came back as the stop verdict, and the two mean different things to the loop")
	}
}

func TestAnAttendedRunThatHitsTheListIsNotTheStopVerdict(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("an attended run was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if permission.StoppedForNobodyToAsk(decision) {
		t.Error("a call that asks the user came back as the stop verdict, and someone is there to answer it")
	}
}
