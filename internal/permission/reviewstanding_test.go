package permission_test

// What the wave 6 security review found about the order the permission function
// checks things in. Design section 11 says the harness stops a run that has
// nobody to answer it, and the reliability table says the model never edits or
// escalates unattended. A standing approval checked before that stop turns the
// rule off for the one case it was written for.

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
)

func TestAnUnattendedRunStopsEvenWhenASkillHoldsAStandingApproval(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})

	request := shellRequest(t, "rm -rf /home/jared/coeus")
	request.Unattended = true

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingStop {
		t.Errorf("an unattended delete was ruled %q because %q, want %q; the standing approval is checked before the unattended stop,"+
			" so a scheduled job carries out the one kind of call the design says never runs with nobody there",
			decision.Ruling, decision.Reason, contract.RulingStop)
	}
}
