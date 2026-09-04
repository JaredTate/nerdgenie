package record

import (
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestAJobsNameSurvivesPrintingAndParsing proves the short name a job carries is
// written into the record text and read back out of it, so a job picked up on
// any model still knows what it is called.
func TestAJobsNameSurvivesPrintingAndParsing(t *testing.T) {
	held := contract.Record{
		Header: contract.Header{Kind: contract.RecordJob, ID: "4", Status: contract.StatusRunning, Origin: "terminal"},
		Goal:   contract.Goal{Ask: "Build a big polished game with tests and browser QA.", Name: "Tater Tots Tetris", Why: "the user wants the whole game built and tested"},
	}
	printed := string(Print(held))
	if !strings.Contains(printed, "Name: Tater Tots Tetris") {
		t.Fatalf("the printed record does not carry the job's name:\n%s", printed)
	}
	back, err := Parse([]byte(printed))
	if err != nil {
		t.Fatalf("the record with a name does not read back: %v", err)
	}
	if back.Goal.Name != "Tater Tots Tetris" {
		t.Errorf("the name read back as %q, want %q", back.Goal.Name, "Tater Tots Tetris")
	}
}

// TestARecordWithNoNamePrintsNoNameLine proves a task, which carries no name,
// prints exactly as before, so the forty-step fixture and every task record are
// left byte-for-byte unchanged.
func TestARecordWithNoNamePrintsNoNameLine(t *testing.T) {
	held := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning, Origin: "terminal", NoRoundBudget: true, NoTimeBudget: true},
		Goal:   contract.Goal{Ask: "post a tweet"},
	}
	if printed := string(Print(held)); strings.Contains(printed, "Name:") {
		t.Errorf("a record with no name printed a Name line:\n%s", printed)
	}
}

// TestTheModelSetsAJobsNameOnceAndItStands proves the name is written once
// through the same door the why is, and then cannot be changed, the way the ask
// and the why cannot.
func TestTheModelSetsAJobsNameOnceAndItStands(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Name: "Tater Tots Tetris"}); err != nil {
		t.Fatalf("the model could not name the job: %v", err)
	}
	if got := keeper.Record().Goal.Name; got != "Tater Tots Tetris" {
		t.Fatalf("the job's name reads %q after it was set", got)
	}
	// The same name again is no change and is allowed.
	if err := keeper.Apply(ctx, Update{Name: "Tater Tots Tetris"}); err != nil {
		t.Fatalf("writing the same name again was refused: %v", err)
	}
	// A different name is refused, because the name is written once.
	err := keeper.Apply(ctx, Update{Name: "Something Else"})
	if !errors.Is(err, ErrNameIsSet) {
		t.Fatalf("a second, different name was not refused with ErrNameIsSet: %v", err)
	}
	if got := keeper.Record().Goal.Name; got != "Tater Tots Tetris" {
		t.Errorf("the refused change altered the name to %q", got)
	}
}
