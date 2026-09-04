package task_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFortyStepFixtureIsWrittenThroughTheRealTool is finding 25 of the wave
// 6 gate review: the design's own canonical task was refused four times out of
// four. The fixture writes its record with no operation at all, with the names
// `doneWhen` and `stopWhen`, and with the decision and the failure as nested
// objects, and until now it was played through record.Update directly and never
// through the shipping tool, so nothing caught it. Running it here is what keeps
// the loop's reader and this tool understanding the same calls.
func TestTheFortyStepFixtureIsWrittenThroughTheRealTool(t *testing.T) {
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	tool, keeper := newTool(t)

	written := 0
	for _, round := range fixture.Rounds {
		if len(round.TaskUpdate) == 0 {
			continue
		}
		arguments, err := json.Marshal(round.TaskUpdate)
		if err != nil {
			t.Fatalf("cannot write round %d's record write as JSON: %v", round.Number, err)
		}
		if _, err := tool.Run(context.Background(), arguments); err != nil {
			t.Errorf("round %d's record write was refused: %v\nthe call was %s", round.Number, err, arguments)
			continue
		}
		written++
	}
	if written != 4 {
		t.Fatalf("%d of the fixture's four record writes were taken", written)
	}

	held := keeper.Record()
	if held.Goal.Why != fixture.Why {
		t.Errorf("the record's why reads %q, want %q", held.Goal.Why, fixture.Why)
	}
	if len(held.Goal.DoneWhen) != 2 || held.Goal.DoneWhen[0].Text != fixture.DoneWhen[0].Text {
		t.Errorf("the done list came out as %+v", held.Goal.DoneWhen)
	}
	if len(held.Rules.StopWhen) != 2 || held.Rules.StopWhen[0] != fixture.StopWhen[0] {
		t.Errorf("the stop list came out as %v", held.Rules.StopWhen)
	}
	if len(held.Work.Plan) != 10 {
		t.Errorf("the plan came out with %d steps, want the fixture's ten", len(held.Work.Plan))
	}
	if len(held.Lessons.Failures) != 1 || held.Lessons.Failures[0].Cause == "" {
		t.Errorf("the failures came out as %+v, want the fixture's one with its cause", held.Lessons.Failures)
	}
	if len(held.Lessons.Decisions) != 1 || held.Lessons.Decisions[0].Reason == "" {
		t.Errorf("the decisions came out as %+v, want the fixture's one with its reason", held.Lessons.Decisions)
	}
}
