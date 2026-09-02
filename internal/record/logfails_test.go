package record

import (
	"context"
	"errors"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// brokenStore is a log that will not take a write, which is what a full disk or a
// database that has gone away looks like from here.
type brokenStore struct {
	*testkit.FakeStore
	refuseWrites bool
	refuseReads  bool
}

// Append refuses every write once the log has gone wrong.
func (store *brokenStore) Append(ctx context.Context, event contract.Event) (int64, error) {
	if store.refuseWrites {
		return 0, errors.New("the log will not take a write, because this test says so")
	}
	return store.FakeStore.Append(ctx, event)
}

// ByTask refuses every read once the log has gone wrong.
func (store *brokenStore) ByTask(ctx context.Context, taskID string) ([]contract.Event, error) {
	if store.refuseReads {
		return nil, errors.New("the log will not take a read, because this test says so")
	}
	return store.FakeStore.ByTask(ctx, taskID)
}

// TestALogThatWillNotWriteLeavesTheRecordAsItWas proves the record and the log
// never disagree: when the checkpoint cannot be saved, the change is undone and
// the caller is told.
func TestALogThatWillNotWriteLeavesTheRecordAsItWas(t *testing.T) {
	store := &brokenStore{FakeStore: testkit.NewFakeStore()}
	ctx := t.Context()
	keeper, err := New(ctx, store, taskStart())
	if err != nil {
		t.Fatalf("cannot create the record: %v", err)
	}
	before := keeper.Text()
	store.refuseWrites = true

	refused := map[string]func() error{
		"the model's writing": func() error { return keeper.Apply(ctx, Update{Why: "a why that cannot be saved"}) },
		"a result":            func() error { _, err := keeper.AddResult(ctx, "a result", "the whole text"); return err },
		"a correction":        func() error { _, err := keeper.AddCorrection(ctx, "no, do it the other way"); return err },
		"the budget":          func() error { return keeper.SetBudget(ctx, 5, 5) },
		"the cost line":       func() error { return keeper.SetCost(ctx, contract.CostLine{InputTokens: 100}) },
		"the situation":       func() error { return keeper.SetSituation(ctx, []string{"the browser is open"}) },
		"where it stands":     func() error { return keeper.SetStatus(ctx, contract.StatusWaiting) },
		"a plan step's mark":  func() error { return keeper.MarkPlanStep(ctx, 1, "r1") },
	}
	for what, write := range refused {
		if err := write(); err == nil {
			t.Errorf("%s was written although the log refused it", what)
		}
		if keeper.Text() != before {
			t.Errorf("%s stayed in the record although the log refused it:\n%s", what, keeper.Text())
		}
	}
	if keeper.LatestCheckpoint() != 1 {
		t.Errorf("the record stands at checkpoint %d after every change was refused", keeper.LatestCheckpoint())
	}
}

// TestAJobKeepsItsShapeWhenTheLogRefusesAWrite covers the job's own writes the
// same way, because a job record must not drift from its log either.
func TestAJobKeepsItsShapeWhenTheLogRefusesAWrite(t *testing.T) {
	store := &brokenStore{FakeStore: testkit.NewFakeStore()}
	ctx := t.Context()
	keeper, err := New(ctx, store, jobStart())
	if err != nil {
		t.Fatalf("cannot create the job record: %v", err)
	}
	before := keeper.Text()
	store.refuseWrites = true

	if err := keeper.SetProgress(ctx, 1, 4, "task 31 today"); err == nil {
		t.Error("the progress line was written although the log refused it")
	}
	if _, err := keeper.AddReport(ctx, "a report", "the whole report"); err == nil {
		t.Error("a report was added although the log refused it")
	}
	if err := keeper.MarkJobTask(ctx, "t1", "j4.1"); err == nil {
		t.Error("a task was marked done although the log refused it")
	}
	if keeper.Text() != before {
		t.Errorf("a change the log refused stayed in the job record:\n%s", keeper.Text())
	}
}

// TestALogThatWillNotWriteRefusesToCreateARecord proves a record is never held in
// memory alone: if its first checkpoint cannot be saved, there is no record.
func TestALogThatWillNotWriteRefusesToCreateARecord(t *testing.T) {
	store := &brokenStore{FakeStore: testkit.NewFakeStore(), refuseWrites: true}
	if _, err := New(t.Context(), store, taskStart()); err == nil {
		t.Error("a record was created although its first checkpoint could not be saved")
	}
}

// TestALogThatWillNotReadSaysSo covers the other side: reading a result and
// loading a record both hand the reader's own words back.
func TestALogThatWillNotReadSaysSo(t *testing.T) {
	store := &brokenStore{FakeStore: testkit.NewFakeStore()}
	ctx := t.Context()
	keeper, err := New(ctx, store, taskStart())
	if err != nil {
		t.Fatalf("cannot create the record: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "a result", "the whole text"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	store.refuseReads = true

	if _, err := keeper.Read(ctx, "r1"); err == nil {
		t.Error("a result was read although the log refused the read")
	}
	if _, err := Load(ctx, store, "17"); err == nil {
		t.Error("a record loaded although the log refused the read")
	}
}

// TestSkipsAnEventThatSaysItIsAResultAndIsNot proves one broken line in the log
// cannot hide a result that is really there, which is the same idea as skipping a
// malformed line rather than giving up on the whole history.
func TestSkipsAnEventThatSaysItIsAResultAndIsNot(t *testing.T) {
	store := testkit.NewFakeStore()
	ctx := t.Context()
	broken := contract.Event{TaskID: "17", Kind: contract.EventToolResult, Body: []byte("not JSON at all")}
	if _, err := store.Append(ctx, broken); err != nil {
		t.Fatalf("cannot write the broken event: %v", err)
	}
	keeper, err := New(ctx, store, taskStart())
	if err != nil {
		t.Fatalf("cannot create the record: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "a result", "the whole text"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	if text, err := keeper.Read(ctx, "r1"); err != nil || text != "the whole text" {
		t.Errorf("the broken line hid the result: %q with the error %v", text, err)
	}
}

// TestRefusesToHoldARecordThatIsNotTiedToTheLog covers the ways a record can be
// handed to the keeper wrongly.
func TestRefusesToHoldARecordThatIsNotTiedToTheLog(t *testing.T) {
	held := goldenTaskRecord()
	store := testkit.NewFakeStore()

	if _, err := hold(nil, held, 1); err == nil {
		t.Error("a record was held with no log behind it")
	}
	if _, err := hold(store, contract.Record{}, 1); err == nil {
		t.Error("a record with no number was held")
	}
	if _, err := hold(store, held, 0); err == nil {
		t.Error("a record was held at checkpoint zero, and checkpoints count from one")
	}
	if _, err := hold(store, held, 1); err != nil {
		t.Errorf("a whole record would not be held: %v", err)
	}
}

// TestPrintsARecordWhoseNumbersMakeNoSense proves the printer never writes
// nonsense, even when it is handed a record nothing in this package would build.
func TestPrintsARecordWhoseNumbersMakeNoSense(t *testing.T) {
	held := contract.Record{
		Header: contract.Header{
			Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning,
			Cost: contract.CostLine{InputTokens: -5, CachedInputTokens: -1, OutputTokens: -100},
		},
		Goal: contract.Goal{Ask: "do the thing"},
	}
	text := string(Print(held))
	if parsed, err := Parse([]byte(text)); err != nil {
		t.Errorf("a record with a cost below zero printed something that will not read back: %v\n%s", err, text)
	} else if parsed.Header.Cost.InputTokens != 0 {
		t.Errorf("a cost below zero printed as %d tokens", parsed.Header.Cost.InputTokens)
	}
}
