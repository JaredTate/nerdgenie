package record

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// startWithALongAsk is a task record whose ask is the pasted specification of
// ask_test.go: the user's own words, never shortened where they are kept, and
// so the part of a record that costs the most to copy.
func startWithALongAsk() Start {
	start := taskStart()
	start.Ask = aLongAsk()
	return start
}

// TestOnlyTheFirstCheckpointCarriesTheAsk is brief 6.6's finding about the size
// of a checkpoint. The ask never changes and is never shortened where it is
// kept, so copying it into every checkpoint of a long task copies the same page
// and a half a hundred times over. Every checkpoint after the first names the
// one that carries it instead.
func TestOnlyTheFirstCheckpointCarriesTheAsk(t *testing.T) {
	keeper, store := newKeeper(t, startWithALongAsk())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Why: "mark the anniversary publicly today"}); err != nil {
		t.Fatalf("cannot write the why: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "read memory/product.md", "the whole file"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}

	saved := checkpointsSavedInto(t, store)
	if len(saved) != 3 {
		t.Fatalf("the log holds %d checkpoints and the record was created and changed twice", len(saved))
	}
	if saved[0].AskFrom != 0 || !strings.Contains(saved[0].Text, aLongAsk()) {
		t.Error("the first checkpoint does not carry the ask, and it is the one that has to")
	}
	for _, one := range saved[1:] {
		if one.AskFrom != saved[0].Number {
			t.Errorf("checkpoint %d names checkpoint %d for the ask, and the first one carries it",
				one.Number, one.AskFrom)
		}
		if strings.Contains(one.Text, aLongAsk()) {
			t.Errorf("checkpoint %d carries the whole ask as well as naming the checkpoint that holds it", one.Number)
		}
	}
}

// TestARecordLoadedFromALaterCheckpointStillHoldsTheWholeAsk is the other half:
// nothing may be lost by keeping the ask in one place. A record read back out of
// the log holds the user's words exactly as the user wrote them, and its printed
// form carries them, because that is what goes in front of the model.
func TestARecordLoadedFromALaterCheckpointStillHoldsTheWholeAsk(t *testing.T) {
	keeper, store := newKeeper(t, startWithALongAsk())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Why: "mark the anniversary publicly today"}); err != nil {
		t.Fatalf("cannot write the why: %v", err)
	}

	loaded, err := Load(ctx, store, contract.RecordTask, "17")
	if err != nil {
		t.Fatalf("cannot load task 17 back: %v", err)
	}
	if held := loaded.Record().Goal.Ask; held != aLongAsk() {
		t.Errorf("the loaded ask is %d characters and the user wrote %d", len(held), len(aLongAsk()))
	}
	if loaded.Text() != keeper.Text() {
		t.Error("the record loaded from the log is not the one that was saved")
	}

	// A record that goes on being worked on after it was loaded still writes its
	// checkpoints against the one that carries the ask.
	if _, err := loaded.AddResult(ctx, "read memory/brand.md", "the whole file"); err != nil {
		t.Fatalf("cannot go on working on the loaded record: %v", err)
	}
	again, err := Load(ctx, store, contract.RecordTask, "17")
	if err != nil {
		t.Fatalf("cannot load task 17 back a second time: %v", err)
	}
	if again.Record().Goal.Ask != aLongAsk() {
		t.Error("the ask was lost by working on a record that had been loaded from the log")
	}
}

// TestACheckpointThatNamesAMissingOneForTheAskSaysSo proves the reader does not
// hand back a record with a stand-in where the user's words belong. A log read
// that begins after the checkpoint carrying the ask is a log this package cannot
// read whole, and it says which checkpoint it is missing.
func TestACheckpointThatNamesAMissingOneForTheAskSaysSo(t *testing.T) {
	one := Checkpoint{Number: 7, Text: string(Print(contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "17", Status: contract.StatusRunning},
		Goal:   contract.Goal{Ask: AskHeldElsewhere},
	})), AskFrom: 1}

	if _, err := one.Read(""); err == nil {
		t.Error("a checkpoint whose ask is in a checkpoint nobody passed in read back without a word about it")
	}
	held, err := one.Read("the user's own words")
	if err != nil {
		t.Fatalf("a checkpoint given the ask it names did not read back: %v", err)
	}
	if held.Goal.Ask != "the user's own words" {
		t.Errorf("the ask read back as %q, and the checkpoint was given %q", held.Goal.Ask, "the user's own words")
	}
}

// checkpointsSavedInto is every checkpoint in a log, in the order it was
// written.
func checkpointsSavedInto(t *testing.T, store *testkit.FakeStore) []Checkpoint {
	t.Helper()
	events, err := store.ByTask(t.Context(), contract.RecordLogKey(contract.RecordTask, "17"))
	if err != nil {
		t.Fatalf("cannot read the log of task 17: %v", err)
	}
	saved := []Checkpoint{}
	for _, event := range events {
		if event.Kind != contract.EventCheckpoint {
			continue
		}
		one := Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			t.Fatalf("an event of the checkpoint kind does not read back as one: %v", err)
		}
		saved = append(saved, one)
	}
	return saved
}
