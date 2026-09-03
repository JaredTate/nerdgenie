package record

import (
	"fmt"
	"strings"
	"testing"
)

// aLongAsk is the shape of ask this rule exists for: one pasted specification,
// far past the quarter of a record's size the ask is allowed to fill.
func aLongAsk() string {
	return "Build the release pipeline. " + strings.Repeat("one more sentence of the specification the user pasted in. ", 400)
}

// TestARecordWithALongAskStillTakesWrites is the first half of the decision on
// finding 22. The ask is the user's own words and is never cut or rewritten in
// storage, so it cannot be what pushes a record over its size; if it were, a
// pasted specification would leave a task that can record nothing at all, which
// is worse than a task that is slow. So the size limit counts everything in the
// record except the ask.
func TestARecordWithALongAskStillTakesWrites(t *testing.T) {
	start := taskStart()
	start.Ask = aLongAsk()
	keeper, err := newKeeperOrError(t, start)
	if err != nil {
		t.Fatalf("a record with a long ask could not be opened at all: %v", err)
	}
	ctx := t.Context()

	if counted := EstimateTokens(keeper.Text()); counted <= MaxRecordTokens {
		t.Fatalf("the record with a long ask is only %d tokens, under the limit of %d, so this test is not measuring what it says it measures",
			counted, MaxRecordTokens)
	}
	if err := keeper.Apply(ctx, Update{Why: "the user wants a release that installs itself"}); err != nil {
		t.Errorf("a record with a long ask refused the model's own writing: %v", err)
	}
	for round := range 20 {
		if _, err := keeper.AddResult(ctx, fmt.Sprintf("round %d read a file", round+1), "the whole text"); err != nil {
			t.Fatalf("a record with a long ask refused the result of round %d: %v", round+1, err)
		}
	}
}

// TestThePrintedFormForTheModelPointsAtTheWholeAsk is the second half. What the
// model reads shows the first quarter of a long ask and then one line saying how
// much more there is and how to fetch it, because a specification the model
// cannot see the end of is still better read in full on purpose than pushed in
// front of it on every call.
func TestThePrintedFormForTheModelPointsAtTheWholeAsk(t *testing.T) {
	start := taskStart()
	start.Ask = aLongAsk()
	keeper, err := newKeeperOrError(t, start)
	if err != nil {
		t.Fatalf("cannot open the record: %v", err)
	}

	shown := string(PrintForTheModel(keeper.Record()))
	if strings.Contains(shown, start.Ask) {
		t.Errorf("the whole of a long ask is in front of the model on every call:\n%s", shown)
	}
	if !strings.Contains(shown, "Build the release pipeline.") {
		t.Errorf("the start of the ask is not in front of the model at all:\n%s", shown)
	}
	if !strings.Contains(shown, AskCutNote) {
		t.Errorf("nothing says the ask was shortened:\n%s", shown)
	}
	if !strings.Contains(shown, "read "+AskLabel) {
		t.Errorf("the note does not say how to read the whole ask:\n%s", shown)
	}
	if counted := EstimateTokens(shown); counted > MaxRecordTokens {
		t.Errorf("what the model is shown is %d tokens, over the limit of %d", counted, MaxRecordTokens)
	}
}

// TestAnAskInsideItsShareIsPrintedWordForWord proves the rule bites only on the
// long ones. Every ordinary task, the forty-step fixture included, reads the same
// in both forms.
func TestAnAskInsideItsShareIsPrintedWordForWord(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	held := keeper.Record()

	if shown, stored := string(PrintForTheModel(held)), string(Print(held)); shown != stored {
		t.Errorf("an ask well inside its share was changed on the way to the model.\n--- stored ---\n%s\n--- shown ---\n%s",
			stored, shown)
	}
}

// TestTheStoredFormKeepsEveryByteOfTheAsk is the promise the other two rest on:
// the record on disk, and every checkpoint of it, holds the user's words entire.
func TestTheStoredFormKeepsEveryByteOfTheAsk(t *testing.T) {
	start := taskStart()
	start.Ask = aLongAsk()
	keeper, store := newKeeper(t, start)
	ctx := t.Context()
	if _, err := keeper.AddResult(ctx, "read a file", "the whole text"); err != nil {
		t.Fatalf("cannot add a result: %v", err)
	}

	if held := keeper.Record(); held.Goal.Ask != start.Ask {
		t.Errorf("the record in hand no longer holds the user's words entire: %d bytes of %d", len(held.Goal.Ask), len(start.Ask))
	}
	if !strings.Contains(keeper.Text(), start.Ask) {
		t.Error("the stored form of the record no longer holds the user's words entire")
	}
	reloaded, err := Load(ctx, store, start.Kind, start.ID)
	if err != nil {
		t.Fatalf("cannot load the record back from its checkpoints: %v", err)
	}
	if reloaded.Record().Goal.Ask != start.Ask {
		t.Errorf("the ask came back from the checkpoint as %d bytes, and the user wrote %d",
			len(reloaded.Record().Goal.Ask), len(start.Ask))
	}
}

// TestReadingTheAskBringsBackAllOfIt proves the pointer line is not a dead end:
// the label it names reads the whole ask back through the same door as `read r7`.
func TestReadingTheAskBringsBackAllOfIt(t *testing.T) {
	start := taskStart()
	start.Ask = aLongAsk()
	keeper, err := newKeeperOrError(t, start)
	if err != nil {
		t.Fatalf("cannot open the record: %v", err)
	}

	text, err := keeper.Read(t.Context(), AskLabel)
	if err != nil {
		t.Fatalf("cannot read the ask back by its label: %v", err)
	}
	if text != start.Ask {
		t.Errorf("reading the ask gave back %d bytes, and the user wrote %d", len(text), len(start.Ask))
	}
}

// TestTheModelsFormIsNotARecordThatCanBeStored proves the shortened form can
// never be mistaken for the record. Parse refuses it, so there is no way for a
// view meant for one prompt to be read back and saved over the user's own words.
func TestTheModelsFormIsNotARecordThatCanBeStored(t *testing.T) {
	start := taskStart()
	start.Ask = aLongAsk()
	keeper, err := newKeeperOrError(t, start)
	if err != nil {
		t.Fatalf("cannot open the record: %v", err)
	}

	if _, err := Parse(PrintForTheModel(keeper.Record())); err == nil {
		t.Error("the shortened form read back as a record, so a cut ask could be saved over the user's own words")
	}
}
