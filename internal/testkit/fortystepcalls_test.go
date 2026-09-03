package testkit_test

import (
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

// TestOneRoundOfTheFixtureAsksForTwoToolsInOneReply is brief 6.6's loop item on
// the fixture's side. The instruction text now tells the model it may ask for
// several tools in one reply when they do not depend on each other, so the
// fixture the whole design is proved against has to contain a round that does,
// or nothing ever proves the harness runs them in the order they were written.
func TestOneRoundOfTheFixtureAsksForTwoToolsInOneReply(t *testing.T) {
	task := loadFixture(t)

	rounds := 0
	for _, round := range task.Rounds {
		if len(round.AlsoCalls) == 0 {
			continue
		}
		rounds++
		checkTheSecondCallOfTheRound(t, task, round)
	}
	if rounds != 1 {
		t.Errorf("%d rounds of the fixture ask for several tools in one reply, and the fixture is written with one",
			rounds)
	}
}

// checkTheSecondCallOfTheRound proves the round's own call and the second one
// are two different tools with two results, in the order the round writes them.
func checkTheSecondCallOfTheRound(t *testing.T, task testkit.FortyStepTask, round testkit.FortyStepRound) {
	t.Helper()
	step := task.Script().Steps[round.Number-1]
	if len(step.ToolCalls) != 1+len(round.AlsoCalls) {
		t.Fatalf("round %d asks for %d tools and the script's step for it has %d calls",
			round.Number, 1+len(round.AlsoCalls), len(step.ToolCalls))
	}
	for at, also := range round.AlsoCalls {
		if also.ToolName == "" || also.ResultSummary == "" || also.ResultText == "" {
			t.Errorf("the further call %d of round %d has no tool, no summary, or no result text", at+1, round.Number)
		}
		if step.ToolCalls[at+1].Name != also.ToolName {
			t.Errorf("call %d of round %d is %q in the script and %q in the fixture",
				at+2, round.Number, step.ToolCalls[at+1].Name, also.ToolName)
		}
	}
	produced := task.ResultsOfRound(round.Number)
	if len(produced) != len(step.ToolCalls) {
		t.Fatalf("round %d asks for %d tools and produces %d results", round.Number, len(step.ToolCalls), len(produced))
	}
	if produced[0].Summary != round.ResultSummary || produced[1].Summary != round.AlsoCalls[0].ResultSummary {
		t.Errorf("the results of round %d are %q then %q, and the calls were written the other way round",
			round.Number, produced[0].Summary, produced[1].Summary)
	}
}
