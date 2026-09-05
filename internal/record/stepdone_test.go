package record

import (
	"errors"
	"testing"
)

// TestAStepDoneMarksThePlanStepWithItsResult is the check mark the panel
// draws: the record has always held MarkPlanStep, and nothing ever called it,
// so a live build's plan stood at "0 of 9 done" after its engine, hazards and
// UI were finished. A step_done write marks one step and points it at the
// result that proves it, through the same rules as every other write.
func TestAStepDoneMarksThePlanStepWithItsResult(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if err := keeper.Apply(ctx, Update{Plan: planStepsOf(3)}); err != nil {
		t.Fatalf("cannot write the plan: %v", err)
	}
	label, err := keeper.AddResult(ctx, "shell: 12 tests passing", "12 tests passing\nexit 0")
	if err != nil {
		t.Fatalf("cannot add a result: %v", err)
	}

	if err := keeper.Apply(ctx, Update{StepDone: &StepDone{Number: 2, ResultID: label}}); err != nil {
		t.Fatalf("marking step 2 done with %s was refused: %v", label, err)
	}

	held := keeper.Record()
	if !held.Work.Plan[1].Done || held.Work.Plan[1].ResultID != label {
		t.Errorf("step 2 reads %+v, want it done and pointing at %s", held.Work.Plan[1], label)
	}
	if held.Work.Plan[0].Done || held.Work.Plan[2].Done {
		t.Errorf("the other steps read %+v and %+v, and only step 2 was marked", held.Work.Plan[0], held.Work.Plan[2])
	}
}

// TestAStepDoneIsRefusedForAStepOrAResultTheRecordDoesNotHold keeps the mark
// honest: a step past the plan's end and a result the record never wrote are
// both refused, and the plan stands as it was.
func TestAStepDoneIsRefusedForAStepOrAResultTheRecordDoesNotHold(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if err := keeper.Apply(ctx, Update{Plan: planStepsOf(2)}); err != nil {
		t.Fatalf("cannot write the plan: %v", err)
	}
	label, _ := keeper.AddResult(ctx, "read: the notes", "the notes")

	if err := keeper.Apply(ctx, Update{StepDone: &StepDone{Number: 9, ResultID: label}}); err == nil {
		t.Error("step 9 of a two-step plan was marked done")
	}
	if err := keeper.Apply(ctx, Update{StepDone: &StepDone{Number: 1, ResultID: "r9"}}); !errors.Is(err, ErrPlanStepNeedsResult) {
		t.Errorf("a step marked by r9, which the record never wrote, was refused with %v, want the rule about a step's result", err)
	}
	for at, step := range keeper.Record().Work.Plan {
		if step.Done {
			t.Errorf("step %d reads done after two refused writes", at+1)
		}
	}
}
