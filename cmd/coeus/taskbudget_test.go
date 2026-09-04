package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theWeeklyNoteBudget is the budget the weekly-note skill sets in these tests,
// which no cap in the configuration would give.
var theWeeklyNoteBudget = loop.Budget{Rounds: 40, Time: 30 * time.Minute}

// anAgentWithTheWeeklyNoteSkill is an agent whose skills box holds one skill
// with a budget of its own and one without.
func anAgentWithTheWeeklyNoteSkill() (*agent, *testkit.FakeSkill) {
	skills := testkit.NewFakeSkill()
	skills.Add(contract.SkillSummary{Name: "weekly-note", Description: "Writes the weekly note."}, "the steps", "weekly note")
	skills.SetBudget("weekly-note", theWeeklyNoteBudget.Rounds, theWeeklyNoteBudget.Time)
	skills.Add(contract.SkillSummary{Name: "quick-look", Description: "Looks something up."}, "the steps", "quick look")
	box := &skillsBox{}
	box.fill(skills)
	return &agent{settings: contract.DefaultConfig(), skillsBox: box}, skills
}

// TestATaskUnderASkillTakesTheSkillsBudget is design section 3, rule 3 in the
// wiring: loop.Task.Budget had no writer outside a test, so a skill's own
// budget was never what a task ran on.
func TestATaskUnderASkillTakesTheSkillsBudget(t *testing.T) {
	running, _ := anAgentWithTheWeeklyNoteSkill()

	budget := running.budgetForTheMessage(context.Background(), contract.Inbound{Text: "write the weekly note"})

	if budget != theWeeklyNoteBudget {
		t.Errorf("a message the weekly-note skill claims was budgeted %+v, want the skill's own %+v", budget, theWeeklyNoteBudget)
	}
}

// TestATaskNoSkillClaimsIsLeftToTheCaps proves the fallback: a zero budget is
// what the loop reads as the caps in the configuration.
func TestATaskNoSkillClaimsIsLeftToTheCaps(t *testing.T) {
	running, _ := anAgentWithTheWeeklyNoteSkill()

	for _, said := range []string{"write the release notes", "take a quick look at the shelf"} {
		if budget := running.budgetForTheMessage(context.Background(), contract.Inbound{Text: said}); budget != (loop.Budget{}) {
			t.Errorf("%q was budgeted %+v, want nothing, so that the caps apply", said, budget)
		}
	}
}

// TestASkillStoreThatCannotAnswerLeavesTheBudgetToTheCaps proves a broken
// skills folder costs the task its skill's budget and nothing more, because a
// task on the caps is better than no task.
func TestASkillStoreThatCannotAnswerLeavesTheBudgetToTheCaps(t *testing.T) {
	box := &skillsBox{}
	box.fill(unreadableSkills{FakeSkill: testkit.NewFakeSkill()})
	running := &agent{settings: contract.DefaultConfig(), skillsBox: box}

	if budget := running.budgetForTheMessage(context.Background(), contract.Inbound{Text: "write the weekly note"}); budget != (loop.Budget{}) {
		t.Errorf("a message the store could not match was budgeted %+v, want nothing", budget)
	}

	empty := &agent{settings: contract.DefaultConfig()}
	if budget := empty.budgetForTheMessage(context.Background(), contract.Inbound{Text: "write the weekly note"}); budget != (loop.Budget{}) {
		t.Errorf("an agent with no skills box budgeted a message %+v, want nothing", budget)
	}
}

// unreadableSkills is a skill store whose folder cannot be read, so every match
// fails.
type unreadableSkills struct {
	*testkit.FakeSkill
}

// Match always fails, the way a store over a folder that has gone would.
func (unreadableSkills) Match(context.Context, string) (contract.SkillMatch, error) {
	return contract.SkillMatch{}, errors.New("the skills folder cannot be read, so check that it is still there")
}
