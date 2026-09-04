package record

import (
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestUnprovenNamesTheLinesWithNothingBehindThem proves the done-check reads the
// done list and hands back the lines that would send the model back to work.
func TestUnprovenNamesTheLinesWithNothingBehindThem(t *testing.T) {
	held := contract.Record{
		Goal: contract.Goal{DoneWhen: []contract.DoneLine{
			{Text: "proved by a result", Done: true, ResultID: "r6"},
			{Text: "proved by the user", Done: true, UserReply: "yes, that is right"},
			{Text: "still waiting"},
			{Text: "has a result but is not marked done", ResultID: "r7"},
			{Text: "points at a result nobody wrote", Done: true, ResultID: "r99"},
		}},
		Work: contract.Work{Results: []contract.ResultLine{
			{ID: "r6", Summary: "the first result"},
			{ID: "r7", Summary: "the second result"},
		}},
	}

	waiting := Unproven(held)
	if len(waiting) != 3 {
		t.Fatalf("the done-check found %d lines with nothing behind them, and three are waiting: %+v", len(waiting), waiting)
	}
	wanted := []string{"still waiting", "has a result but is not marked done", "points at a result nobody wrote"}
	for at, line := range waiting {
		if line.Text != wanted[at] {
			t.Errorf("the done-check named %q where it should name %q", line.Text, wanted[at])
		}
	}
	if len(Unproven(contract.Record{})) != 0 {
		t.Error("the done-check found a waiting line in a record with no done list at all")
	}
}

// TestRefusesADoneLineWithTwoProofs proves a line names one thing that proves it,
// because a record only ever prints one and the other would be lost on reload.
func TestRefusesADoneLineWithTwoProofs(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if _, err := keeper.AddResult(ctx, "the post went up", "the whole page"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	both := []contract.DoneLine{{Text: "one post is up", Done: true, ResultID: "r1", UserReply: "yes, I saw it"}}
	if err := keeper.Apply(ctx, Update{DoneWhen: both}); err == nil {
		t.Error("a done line naming both a result and a reply was accepted")
	}
}

// TestRefusesToCloseARecordWhileALineIsWaiting proves the rule that stops the
// model from declaring victory early.
func TestRefusesToCloseARecordWhileALineIsWaiting(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if _, err := keeper.AddResult(ctx, "the post went up", "the whole page"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	waiting := []contract.DoneLine{
		{Text: "one post is up", Done: true, ResultID: "r1"},
		{Text: "it is under 280 characters"},
	}
	if err := keeper.Apply(ctx, Update{DoneWhen: waiting}); err != nil {
		t.Fatalf("cannot write the done list: %v", err)
	}

	err := keeper.SetStatus(ctx, contract.StatusDone)
	if !errors.Is(err, ErrDoneLineNeedsProof) {
		t.Fatalf("the record closed while a line was waiting: %v", err)
	}
	if !strings.Contains(err.Error(), "it is under 280 characters") {
		t.Errorf("the refusal does not name the line that is waiting: %v", err)
	}
	if keeper.Record().Header.Status != contract.StatusRunning {
		t.Errorf("the refused close left the record at %q", keeper.Record().Header.Status)
	}

	proved := []contract.DoneLine{
		{Text: "one post is up", Done: true, ResultID: "r1"},
		{Text: "it is under 280 characters", Done: true, UserReply: "yes, I counted"},
	}
	if err := keeper.Apply(ctx, Update{DoneWhen: proved}); err != nil {
		t.Fatalf("cannot prove the second line: %v", err)
	}
	if err := keeper.SetStatus(ctx, contract.StatusDone); err != nil {
		t.Fatalf("a record whose every line is proved would not close: %v", err)
	}
	if keeper.Record().Header.Status != contract.StatusDone {
		t.Errorf("the record stands at %q after it closed", keeper.Record().Header.Status)
	}
}

// TestRefusesToCloseARecordWithNoDoneList proves a record cannot be closed by
// leaving the done list empty, which would prove nothing at all.
func TestRefusesToCloseARecordWithNoDoneList(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	if err := keeper.SetStatus(t.Context(), contract.StatusDone); !errors.Is(err, ErrDoneLineNeedsProof) {
		t.Errorf("a record with no done list closed anyway: %v", err)
	}
}

// TestTakesTheOtherFourStatusesFreely proves only the close is guarded, because
// stopping, failing, and waiting on the user are not claims that the work is done.
func TestTakesTheOtherFourStatusesFreely(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	for _, status := range []contract.RecordStatus{
		contract.StatusWaiting, contract.StatusStopped, contract.StatusFailed, contract.StatusRunning,
	} {
		if err := keeper.SetStatus(ctx, status); err != nil {
			t.Errorf("the record would not stand at %q: %v", status, err)
		}
	}
	if err := keeper.SetStatus(ctx, "sprinting"); err == nil {
		t.Error("the record took a status nobody knows")
	}
}
