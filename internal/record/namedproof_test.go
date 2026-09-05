package record

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestADoneLineThatNamesAResultIsTakenAsDone holds that pointing a done line at
// a result the record holds is what proves it, and the mark follows. On a live
// job the model wrote all five of a task's done lines with the result that
// proved each and left every mark off, so the done-check found five unproven
// lines, the reply's trailing question put the whole job on hold, and a person
// had to type "continue" for work the record already held the proof of.
func TestADoneLineThatNamesAResultIsTakenAsDone(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	label, err := keeper.AddResult(ctx, "shell: 51 tests passing", "51 tests passing\nexit 0")
	if err != nil {
		t.Fatalf("cannot add a result to point the done line at: %v", err)
	}

	err = keeper.Apply(ctx, Update{DoneWhen: []contract.DoneLine{
		{Text: "the suite is green", ResultID: label},
		{Text: "the game is playable"},
	}})
	if err != nil {
		t.Fatalf("a done line naming the result %s was refused: %v", label, err)
	}

	held := keeper.Record()
	if !held.Goal.DoneWhen[0].Done || held.Goal.DoneWhen[0].ResultID != label {
		t.Errorf("the line naming %s reads %+v, and naming the result that proves a line is what marks it done",
			label, held.Goal.DoneWhen[0])
	}
	if held.Goal.DoneWhen[1].Done {
		t.Errorf("the line naming nothing reads %+v, and a line with nothing behind it stays open", held.Goal.DoneWhen[1])
	}
	if waiting := Unproven(held); len(waiting) != 1 || waiting[0].Text != "the game is playable" {
		t.Errorf("the done-check finds %v unproven, want only the line that names nothing", waiting)
	}
}

// TestADoneLineThatNamesAResultTheRecordNeverWroteIsStillRefused keeps the
// other half of the rule: the mark follows a result the record holds, and a
// result it never wrote is no proof at all.
func TestADoneLineThatNamesAResultTheRecordNeverWroteIsStillRefused(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	err := keeper.Apply(t.Context(), Update{DoneWhen: []contract.DoneLine{{Text: "the suite is green", ResultID: "r9"}}})
	if err == nil {
		t.Fatal("a done line naming r9, which the record never wrote, was taken")
	}
	if held := keeper.Record(); len(held.Goal.DoneWhen) != 0 {
		t.Errorf("the refused done list was written anyway: %+v", held.Goal.DoneWhen)
	}
}
