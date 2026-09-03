package skill_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/skill"
)

// aBudgetBlock is the section of SKILL.md that sets how much a task run under
// the skill may spend, written the way a person would write it.
const aBudgetBlock = "\n## Budget\n\n- rounds: 40\n- time: 30m\n"

// TestASkillsBudgetIsReadFromItsDescriptionFile is design section 3, rule 3:
// every task has a budget, and a skill can set its own. Before this the skill
// format had nowhere to write one, so loop.Task.Budget had no writer outside a
// test.
func TestASkillsBudgetIsReadFromItsDescriptionFile(t *testing.T) {
	definition, err := skill.ParseDescriptionFile(describing("budgeted", aBudgetBlock))
	if err != nil {
		t.Fatalf("a description file with a budget block did not parse: %v", err)
	}
	if definition.Rounds != 40 {
		t.Errorf("the rounds read back as %d, want the 40 the file says", definition.Rounds)
	}
	if definition.Time != 30*time.Minute {
		t.Errorf("the time read back as %s, want the thirty minutes the file says", definition.Time)
	}
}

// TestASkillThatSetsNoBudgetLeavesTheCapsToApply proves both halves are
// optional: a skill that says nothing about its budget runs on the caps in the
// configuration, which is what a zero budget means to the loop.
func TestASkillThatSetsNoBudgetLeavesTheCapsToApply(t *testing.T) {
	definition, err := skill.ParseDescriptionFile(describing("plain", "\n## Triggers\n\n- a word\n"))
	if err != nil {
		t.Fatalf("a description file with no budget block did not parse: %v", err)
	}
	if definition.Rounds != 0 || definition.Time != 0 {
		t.Errorf("a skill that set no budget read back with %d rounds and %s, want nothing, so that the caps apply",
			definition.Rounds, definition.Time)
	}
}

// TestEitherHalfOfTheBudgetMayBeLeftOut proves the two lines are independent,
// because a skill that only knows it is slow should not have to guess at a
// number of rounds.
func TestEitherHalfOfTheBudgetMayBeLeftOut(t *testing.T) {
	onlyRounds, err := skill.ParseDescriptionFile(describing("rounds-only", "\n## Budget\n\n- rounds: 12\n"))
	if err != nil {
		t.Fatalf("a budget block with only rounds did not parse: %v", err)
	}
	if onlyRounds.Rounds != 12 || onlyRounds.Time != 0 {
		t.Errorf("a block with only rounds read back as %d rounds and %s", onlyRounds.Rounds, onlyRounds.Time)
	}
	onlyTime, err := skill.ParseDescriptionFile(describing("time-only", "\n## Budget\n\n- time: 2h\n"))
	if err != nil {
		t.Fatalf("a budget block with only time did not parse: %v", err)
	}
	if onlyTime.Rounds != 0 || onlyTime.Time != 2*time.Hour {
		t.Errorf("a block with only time read back as %d rounds and %s", onlyTime.Rounds, onlyTime.Time)
	}
}

// TestABudgetLineTheReaderDoesNotKnowIsRefusedByName holds the budget block to
// the same rule as the permissions block: a typo would quietly set nothing, so
// it is refused with the key quoted back.
func TestABudgetLineTheReaderDoesNotKnowIsRefusedByName(t *testing.T) {
	_, err := skill.ParseDescriptionFile(describing("typo", "\n## Budget\n\n- minutes: 30\n"))
	if err == nil {
		t.Fatal("a budget line about \"minutes\" was allowed, and the two keys are rounds and time")
	}
	if !strings.Contains(err.Error(), "minutes") {
		t.Errorf("the refusal reads %q, and it should quote the key that is not one of the two", err)
	}
}

// TestTheBoundsOnABudget pins every way a budget line can be wrong: nothing,
// less than nothing, over the cap, and not a number or a length of time at all.
func TestTheBoundsOnABudget(t *testing.T) {
	cases := []struct {
		what string
		line string
	}{
		{"rounds of nothing", "- rounds: 0"},
		{"rounds below nothing", "- rounds: -3"},
		{"rounds over the cap", fmt.Sprintf("- rounds: %d", skill.MaxBudgetRounds+1)},
		{"rounds that are not a number", "- rounds: forty"},
		{"time of nothing", "- time: 0s"},
		{"time below nothing", "- time: -5m"},
		{"time over the cap", (skill.MaxBudgetTime + time.Minute).String()},
		{"time that is not a length", "- time: soon"},
		{"a budget line with no value", "- rounds:"},
	}
	for _, test := range cases {
		line := test.line
		if !strings.HasPrefix(line, "- ") {
			line = "- time: " + line
		}
		if _, err := skill.ParseDescriptionFile(describing("bounded", "\n## Budget\n\n"+line+"\n")); err == nil {
			t.Errorf("%s was allowed, and it must be refused", test.what)
		}
	}
}

// TestTheLargestBudgetAllowedIsAllowed pins the other side of the two caps, so
// that a bound cannot quietly move down to refuse a budget a person may write.
func TestTheLargestBudgetAllowedIsAllowed(t *testing.T) {
	largest := fmt.Sprintf("\n## Budget\n\n- rounds: %d\n- time: %s\n", skill.MaxBudgetRounds, skill.MaxBudgetTime)
	definition, err := skill.ParseDescriptionFile(describing("largest", largest))
	if err != nil {
		t.Fatalf("the largest budget allowed was refused: %v", err)
	}
	if definition.Rounds != skill.MaxBudgetRounds || definition.Time != skill.MaxBudgetTime {
		t.Errorf("the largest budget read back as %d rounds and %s", definition.Rounds, definition.Time)
	}
}

// TestABudgetSurvivesBeingWrittenOutAndReadBack holds the promise the format
// makes for every other block: a folder saved from a definition says what the
// definition said.
func TestABudgetSurvivesBeingWrittenOutAndReadBack(t *testing.T) {
	definition, err := skill.ParseDescriptionFile(describing("whole", "\n## Triggers\n\n- a word\n"+aBudgetBlock))
	if err != nil {
		t.Fatalf("the description file did not parse: %v", err)
	}
	again, err := skill.ParseDescriptionFile(skill.RenderDescriptionFile(definition))
	if err != nil {
		t.Fatalf("the description file did not survive being written out: %v", err)
	}
	if again.Rounds != 40 || again.Time != 30*time.Minute {
		t.Errorf("the budget read back as %d rounds and %s after being written out, want 40 rounds and thirty minutes",
			again.Rounds, again.Time)
	}
	// A definition with no budget is written out without a budget block, so a
	// folder never says the caps in numbers that could go stale.
	bare := skill.RenderDescriptionFile(skill.Definition{Name: "bare", Description: "Sets no budget."})
	if strings.Contains(string(bare), "Budget") {
		t.Errorf("a definition with no budget was written out with a budget block:\n%s", bare)
	}
}

// TestMatchHandsBackTheSkillsBudget is what the wiring needs: the match that
// says which skill a message runs under carries that skill's budget, so the
// task the message starts can be given it.
func TestMatchHandsBackTheSkillsBudget(t *testing.T) {
	built := newHarness(t)
	built.writeFiles(t, "weekly-note", map[string]string{
		skill.DescriptionFile: "# weekly-note\n\nWrites the weekly note.\n\n## Triggers\n\n- weekly note\n" + aBudgetBlock,
	})
	built.writeFiles(t, "quick-look", map[string]string{
		skill.DescriptionFile: "# quick-look\n\nLooks something up.\n\n## Triggers\n\n- quick look\n",
	})

	budgeted, err := built.store.Match(t.Context(), "write the weekly note")
	if err != nil {
		t.Fatalf("matching the budgeted skill failed: %v", err)
	}
	if !budgeted.Matched || budgeted.Name != "weekly-note" {
		t.Fatalf("the match is %+v, want the weekly-note skill", budgeted)
	}
	if budgeted.Rounds != 40 || budgeted.Time != 30*time.Minute {
		t.Errorf("the match carries %d rounds and %s, want the 40 rounds and thirty minutes the skill sets",
			budgeted.Rounds, budgeted.Time)
	}

	plain, err := built.store.Match(t.Context(), "take a quick look")
	if err != nil {
		t.Fatalf("matching the plain skill failed: %v", err)
	}
	if plain.Rounds != 0 || plain.Time != 0 {
		t.Errorf("a skill that sets no budget matched with %d rounds and %s, want nothing, so that the caps apply",
			plain.Rounds, plain.Time)
	}
}
