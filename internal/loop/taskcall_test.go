package loop

import (
	"encoding/json"
	"strings"
	"testing"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
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
// in order with only the result named taken out of it.
func TestUnpinningTakesTheOneResultAndLeavesTheRest(t *testing.T) {
	running := &run{pinned: []workingcontext.Pin{
		{ID: "r1", Text: "the brand rule"},
		{ID: "r4", Text: "the draft"},
	}}

	answer, refused := running.unpinEvidence(aTaskCall{Result: "r1"})

	if refused {
		t.Errorf("unpinning a pinned result was refused: %q", answer)
	}
	if len(running.pinned) != 1 || running.pinned[0].ID != "r4" {
		t.Errorf("the pins are now %v, want only the one that was not unpinned", running.pinned)
	}
	if !running.alreadyPinned("r4") || running.alreadyPinned("r1") {
		t.Error("the harness does not know which results are pinned after an unpin")
	}
}
