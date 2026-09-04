package permission_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestAnUnattendedRunThatHitsTheListStopsAndShowsWhatItWouldHaveAsked(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	decision := decide(t, decider, request)

	if decision.Ruling != contract.RulingStop {
		t.Fatalf("an unattended run was ruled %q, want %q, because nothing may run and nobody can answer", decision.Ruling, contract.RulingStop)
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
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "git commit -m \"the nightly report\"")
	request.Unattended = true

	decision := decide(t, decider, request)

	if decision.Ruling != contract.RulingAllow {
		t.Errorf("an unattended run of an ordinary command was ruled %q, want %q, because only a call on the list stops", decision.Ruling, contract.RulingAllow)
	}
}

// An unattended run still uses the answer the user gave in this session, because
// that answer is the user's own word about this very call. A skill's standing
// approval is not the user's word, and reviewstanding_test.go holds the rule that
// an unattended run stops in spite of one.
func TestAnUnattendedRunUsesTheAnswerTheUserAlreadyGave(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	if err := decider.Remember(request, contract.AnswerAlways, ""); err != nil {
		t.Fatalf("remembering an answer of always failed: %v", err)
	}
	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAllow {
		t.Errorf("an unattended run the user already allowed was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
}

func TestARuleThatDeniesIsARefusalAndNotAStop(t *testing.T) {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolShell, Pattern: "*rm -r*", Action: contract.RulingDeny},
	}
	decider := newDecider(t, configuration)
	request := shellRequest(t, "rm -rf /tmp/x")
	request.Unattended = true

	decision := decide(t, decider, request)
	if decision.Ruling != contract.RulingDeny {
		t.Fatalf("a rule that denies gave %q, want %q", decision.Ruling, contract.RulingDeny)
	}
	if decision.Ruling == contract.RulingStop {
		t.Error("a rule that denies came back as a stop, and the two mean different things to the loop")
	}
}

func TestAnAttendedRunThatHitsTheListAsksRatherThanStopping(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("an attended run was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if decision.Ruling == contract.RulingStop {
		t.Error("a call that asks the user came back as a stop, and someone is there to answer it")
	}
}
