package loop_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// theBudgetASkillSets is a budget nothing in the configuration would give, so
// that the record can only have got it from the task.
var theBudgetASkillSets = loop.Budget{Rounds: 7, Time: 12 * time.Minute}

// TestASkillsBudgetIsTheBudgetLineOfTheRecord is design section 3, rule 3 from
// the record's side: a task run under a skill that sets its own budget carries
// that budget in its header, less what it has spent, and not the caps.
func TestASkillsBudgetIsTheBudgetLineOfTheRecord(t *testing.T) {
	built := newHarness(t, scriptThatPins(), scriptedTool("read", theBrandRule))
	task := built.task("write the post")
	task.Budget = theBudgetASkillSets

	outcome, err := built.loop.Run(t.Context(), task)
	if err != nil {
		t.Fatalf("the loop could not run the task under the skill's budget: %v", err)
	}

	held := built.held(t, outcome.TaskID)
	// The script makes two model calls, and a call is a round, so the header
	// carries the skill's seven rounds less the two that were spent. The fake
	// clock does not move, so every one of the twelve minutes is still there.
	if held.Header.RoundsLeft != theBudgetASkillSets.Rounds-2 {
		t.Errorf("the record has %d rounds left after two calls on a budget of %d, want %d",
			held.Header.RoundsLeft, theBudgetASkillSets.Rounds, theBudgetASkillSets.Rounds-2)
	}
	if held.Header.MinutesLeft != 12 {
		t.Errorf("the record has %d minutes left on a budget of twelve with no time spent, want 12", held.Header.MinutesLeft)
	}
	if printed := string(record.Print(held)); !strings.Contains(printed, "budget left: 5 rounds, 12 minutes") {
		t.Errorf("the printed header does not carry the skill's budget:\n%s", printed)
	}
}

// TestATaskUnderNoSkillIsBudgetedByTheCaps is the other half of the rule: a
// zero budget means the caps in the configuration, which is what every task
// that no skill claims runs on. The caps ship with no budget at all, so such a
// task carries none.
func TestATaskUnderNoSkillIsBudgetedByTheCaps(t *testing.T) {
	built := newHarness(t, scriptThatPins(), scriptedTool("read", theBrandRule))

	outcome := built.ask(t, "write the post")

	held := built.held(t, outcome.TaskID)
	if caps := contract.DefaultConfig().Caps; caps.RoundsPerTask != 0 || caps.TimePerTask != 0 {
		t.Fatalf("the caps ship with a budget of %d rounds and %s, and this test is written for none", caps.RoundsPerTask, caps.TimePerTask)
	}
	if !held.Header.NoRoundBudget || !held.Header.NoTimeBudget {
		t.Errorf("the record's header is %+v, want no budget, because the caps set none", held.Header)
	}
	if printed := string(record.Print(held)); !strings.Contains(printed, "   no budget\n") {
		t.Errorf("the printed header does not say the task has no budget:\n%s", printed)
	}
}
