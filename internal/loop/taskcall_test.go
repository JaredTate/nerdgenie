package loop

import (
	"encoding/json"
	"strings"
	"testing"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestOnlyTheLoopsOwnOperationsAreAnsweredHere proves the reading that decides
// which task calls never reach the registry: the three the loop answers itself,
// and nothing else.
func TestOnlyTheLoopsOwnOperationsAreAnsweredHere(t *testing.T) {
	for _, one := range []struct {
		what  string
		call  contract.ToolCall
		mine  bool
		holds string
	}{
		{
			what: "a call to another tool",
			call: contract.ToolCall{Name: contract.ToolRead, Input: json.RawMessage(`{"operation":"stop_now"}`)},
		},
		{
			what: "a task call whose arguments are not an object",
			call: contract.ToolCall{Name: contract.ToolTask, Input: json.RawMessage(`"stop_now"`)},
		},
		{
			what: "a task call that writes the record",
			call: contract.ToolCall{Name: contract.ToolTask, Input: json.RawMessage(`{"why":"the user asked"}`)},
		},
		{
			what:  "a stop that names no line",
			call:  contract.ToolCall{Name: contract.ToolTask, Input: json.RawMessage(`{"operation":"stop_now","text":"  "}`)},
			mine:  true,
			holds: "which line of the stop list",
		},
		{
			what:  "a pin that names no result",
			call:  contract.ToolCall{Name: contract.ToolTask, Input: json.RawMessage(`{"operation":"pin_evidence"}`)},
			mine:  true,
			holds: "name the result to pin",
		},
		{
			what:  "an unpin of something that was never pinned",
			call:  contract.ToolCall{Name: contract.ToolTask, Input: json.RawMessage(`{"operation":"unpin_evidence","result":"r3"}`)},
			mine:  true,
			holds: "was not pinned",
		},
	} {
		running := &run{}
		answer, refused, mine := running.theLoopsOwnOperation(t.Context(), one.call)
		if mine != one.mine {
			t.Errorf("%s was read as the loop's own operation: %v, want %v", one.what, mine, one.mine)
			continue
		}
		if !mine {
			continue
		}
		if !refused {
			t.Errorf("%s was allowed, and it names nothing the loop can act on", one.what)
		}
		if !strings.Contains(answer, one.holds) {
			t.Errorf("%s was answered %q, and the answer says what to write instead", one.what, answer)
		}
	}
}

// TestUnpinningTakesTheOneResultAndLeavesTheRest proves the pinned list is kept
// in order with only the result named taken out of it, in the record as well as
// in the window.
func TestUnpinningTakesTheOneResultAndLeavesTheRest(t *testing.T) {
	keeper := aKeeperHoldingTwoPinnedResults(t)
	running := &run{keeper: keeper, pinned: []workingcontext.Pin{
		{ID: contract.ResultID(1), Text: "the brand rule"},
		{ID: contract.ResultID(2), Text: "the draft"},
	}}

	answer, refused := running.unpinEvidence(t.Context(), aTaskCall{Result: contract.ResultID(1)})

	if refused {
		t.Errorf("unpinning a pinned result was refused: %q", answer)
	}
	if len(running.pinned) != 1 || running.pinned[0].ID != contract.ResultID(2) {
		t.Errorf("the pins are now %v, want only the one that was not unpinned", running.pinned)
	}
	if !running.alreadyPinned(contract.ResultID(2)) || running.alreadyPinned(contract.ResultID(1)) {
		t.Error("the harness does not know which results are pinned after an unpin")
	}
	held := keeper.Record().Work.Results
	if held[0].Pinned || !held[1].Pinned {
		t.Errorf("the record marks %v pinned, and only the result that was not unpinned should be", held)
	}
}

// aKeeperHoldingTwoPinnedResults is a task record with two results in it, both
// marked pinned, which is what a task that had pinned two things is picked up
// as.
func aKeeperHoldingTwoPinnedResults(t *testing.T) *record.Keeper {
	t.Helper()
	keeper, err := record.New(t.Context(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: "17", Origin: "terminal",
		Ask: "write the post", RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot make the record the pins are written into: %v", err)
	}
	for _, summary := range []string{"the brand rule", "the draft"} {
		id, err := keeper.AddResult(t.Context(), summary, summary+", in full")
		if err != nil {
			t.Fatalf("cannot add the result %q: %v", summary, err)
		}
		if err := keeper.Pin(t.Context(), id, true); err != nil {
			t.Fatalf("cannot pin the result %q: %v", summary, err)
		}
	}
	return keeper
}
