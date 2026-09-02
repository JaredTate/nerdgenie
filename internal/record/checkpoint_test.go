package record

import (
	"context"
	"errors"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// keeperWithFourCheckpoints makes a task record and changes it three times, so
// that the log holds checkpoints one to four.
func keeperWithFourCheckpoints(t *testing.T) (*Keeper, *testkit.FakeStore) {
	t.Helper()
	keeper, store := newKeeper(t, taskStart())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Why: "mark the anniversary publicly today"}); err != nil {
		t.Fatalf("cannot write the why: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "read memory/product.md", "the whole file"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	if _, err := keeper.AddCorrection(ctx, "no, lead with the date"); err != nil {
		t.Fatalf("cannot add the correction: %v", err)
	}
	if keeper.LatestCheckpoint() != 4 {
		t.Fatalf("three changes left the record at checkpoint %d", keeper.LatestCheckpoint())
	}
	return keeper, store
}

// TestLoadsTheLatestCheckpoint proves a task put down for days picks up where it
// left off, with nothing held in any model's memory in the meantime.
func TestLoadsTheLatestCheckpoint(t *testing.T) {
	keeper, store := keeperWithFourCheckpoints(t)
	ctx := t.Context()

	loaded, err := Load(ctx, store, contract.RecordTask, "17")
	if err != nil {
		t.Fatalf("cannot load task 17 back: %v", err)
	}
	if loaded.Text() != keeper.Text() {
		t.Errorf("the loaded record is not the one that was saved.\n--- saved ---\n%s\n--- loaded ---\n%s", keeper.Text(), loaded.Text())
	}
	if loaded.LatestCheckpoint() != 4 {
		t.Errorf("the loaded record stands at checkpoint %d, and four were saved", loaded.LatestCheckpoint())
	}

	next, err := loaded.AddResult(ctx, "the second result", "the whole thing")
	if err != nil {
		t.Fatalf("cannot go on working on the loaded record: %v", err)
	}
	if next != "r2" {
		t.Errorf("the next result of the loaded record is labelled %q, and r1 was already written", next)
	}
	if loaded.LatestCheckpoint() != 5 {
		t.Errorf("the loaded record saved checkpoint %d, and it should carry on from four", loaded.LatestCheckpoint())
	}
}

// TestLoadsACheckpointByItsNumber proves any one moment of a task can be read
// back, which is what a replay of a failed task starts from.
func TestLoadsACheckpointByItsNumber(t *testing.T) {
	_, store := keeperWithFourCheckpoints(t)
	ctx := t.Context()

	second, err := LoadCheckpoint(ctx, store, contract.RecordTask, "17", 2)
	if err != nil {
		t.Fatalf("cannot load checkpoint two: %v", err)
	}
	held := second.Record()
	if held.Goal.Why != "mark the anniversary publicly today" {
		t.Errorf("checkpoint two does not hold the why that was written into it: %+v", held.Goal)
	}
	if len(held.Work.Results) != 0 || len(held.Rules.Corrections) != 0 {
		t.Errorf("checkpoint two holds work that came after it: %+v", held.Work)
	}
	if second.LatestCheckpoint() != 2 {
		t.Errorf("checkpoint two loaded as number %d", second.LatestCheckpoint())
	}
}

// TestGoesBackByASetNumberOfSteps is what "/tasks 17 back 3" does: it reloads an
// earlier checkpoint and saves it as a new one, so nothing in the log is lost and
// the model can try another path.
func TestGoesBackByASetNumberOfSteps(t *testing.T) {
	_, store := keeperWithFourCheckpoints(t)
	ctx := t.Context()

	wound, err := Back(ctx, store, contract.RecordTask, "17", 3)
	if err != nil {
		t.Fatalf("cannot wind task 17 back three steps: %v", err)
	}
	if wound.LatestCheckpoint() != 5 {
		t.Errorf("winding back saved checkpoint %d, and it should save the next one after the latest", wound.LatestCheckpoint())
	}
	held := wound.Record()
	if held.Goal.Why != "" || len(held.Work.Results) != 0 || len(held.Rules.Corrections) != 0 {
		t.Errorf("winding back three steps left work behind that came later: %+v", held)
	}
	if held.Goal.Ask != taskStart().Ask {
		t.Errorf("winding back changed the ask to %q", held.Goal.Ask)
	}

	again, err := Load(ctx, store, contract.RecordTask, "17")
	if err != nil {
		t.Fatalf("cannot load task 17 after winding it back: %v", err)
	}
	if again.Text() != wound.Text() {
		t.Errorf("the record that was wound back was not the one saved as the latest checkpoint")
	}
}

// TestRefusesToGoBackPastTheFirstCheckpoint proves the wind-back is bounded at
// both ends, and says so.
func TestRefusesToGoBackPastTheFirstCheckpoint(t *testing.T) {
	_, store := keeperWithFourCheckpoints(t)
	ctx := t.Context()

	if _, err := Back(ctx, store, contract.RecordTask, "17", 4); !errors.Is(err, ErrBeforeTheFirstCheckpoint) {
		t.Errorf("winding back four steps from checkpoint four was allowed: %v", err)
	}
	if _, err := Back(ctx, store, contract.RecordTask, "17", 99); !errors.Is(err, ErrBeforeTheFirstCheckpoint) {
		t.Errorf("winding back further than the record goes was allowed: %v", err)
	}
	if _, err := Back(ctx, store, contract.RecordTask, "17", 0); err == nil {
		t.Error("winding back no steps at all was allowed, and a step back is at least one")
	}
	if _, err := Back(ctx, store, contract.RecordTask, "17", -1); err == nil {
		t.Error("winding back a negative number of steps was allowed")
	}
}

// TestAWindBackNeverHandsOutAResultLabelTwice proves the log stays readable after
// "/tasks 17 back 3". A result written on the new path takes a label no result on
// the abandoned path ever had, so a checkpoint on either path still reads back its
// own evidence, which is what lets a failed task be replayed as a test.
func TestAWindBackNeverHandsOutAResultLabelTwice(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())
	ctx := t.Context()

	if _, err := keeper.AddResult(ctx, "the first result", "the text of the first"); err != nil {
		t.Fatalf("cannot add the first result: %v", err)
	}
	second, err := keeper.AddResult(ctx, "the second result", "the text of the second")
	if err != nil {
		t.Fatalf("cannot add the second result: %v", err)
	}

	wound, err := Back(ctx, store, contract.RecordTask, "17", 1)
	if err != nil {
		t.Fatalf("cannot wind the record back one step: %v", err)
	}
	if len(wound.Record().Work.Results) != 1 {
		t.Fatalf("one step back left %d results, and the second came after", len(wound.Record().Work.Results))
	}

	third, err := wound.AddResult(ctx, "the third result", "the text of the third")
	if err != nil {
		t.Fatalf("cannot add a result after winding back: %v", err)
	}
	if third == second {
		t.Errorf("the result written after winding back took the label %q, which the abandoned path already used", third)
	}
	if text, err := wound.Read(ctx, second); err != nil || text != "the text of the second" {
		t.Errorf("%s no longer reads back as itself: %q with the error %v", second, text, err)
	}
	if text, err := wound.Read(ctx, third); err != nil || text != "the text of the third" {
		t.Errorf("%s does not read back: %q with the error %v", third, text, err)
	}
}

// TestRefusesToLoadWhatIsNotInTheLog covers the ways a load can find nothing.
func TestRefusesToLoadWhatIsNotInTheLog(t *testing.T) {
	_, store := keeperWithFourCheckpoints(t)
	ctx := t.Context()

	if _, err := Load(ctx, store, contract.RecordTask, "44"); err == nil {
		t.Error("a task that was never written loaded anyway")
	}
	if _, err := LoadCheckpoint(ctx, store, contract.RecordTask, "17", 9); err == nil {
		t.Error("a checkpoint that was never saved loaded anyway")
	}
	if _, err := LoadCheckpoint(ctx, store, contract.RecordTask, "17", 0); err == nil {
		t.Error("checkpoint zero loaded, and checkpoints count from one")
	}
	if _, err := Load(ctx, nil, contract.RecordTask, "17"); err == nil {
		t.Error("a record loaded with no log to load it from")
	}
}

// TestRefusesACheckpointThatWillNotRead proves a log holding something that is
// not a record says so rather than handing back a half-read one.
func TestRefusesACheckpointThatWillNotRead(t *testing.T) {
	store := testkit.NewFakeStore()
	ctx := context.Background()
	broken := contract.Event{TaskID: "17", Kind: contract.EventCheckpoint, Body: []byte(`{"number":1,"text":"not a record"}`)}
	if _, err := store.Append(ctx, broken); err != nil {
		t.Fatalf("cannot write the broken checkpoint: %v", err)
	}
	if _, err := Load(ctx, store, contract.RecordTask, "17"); err == nil {
		t.Error("a checkpoint holding something that is not a record loaded anyway")
	}
}
