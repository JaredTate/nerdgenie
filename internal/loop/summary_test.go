package loop

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestARecordWriteIsSummarisedByThePartsItWrote proves the one line the record
// keeps about a record write names the parts that were written. The shipping
// task tool answers with a sentence and the whole update as JSON under it, and
// the reader used to try the whole answer as JSON and fall back to "one change"
// for every write there was.
func TestARecordWriteIsSummarisedByThePartsItWrote(t *testing.T) {
	answer := "the record's done when is written\n" +
		`{"done_when":["the notes are read"],"why":"the user wants the notes read"}`

	summary := summaryOfResult(contract.ToolTask, answer, false)

	if !strings.Contains(summary, "done_when") || !strings.Contains(summary, "why") {
		t.Errorf("the record kept the line %q, and it names the parts of the record the write touched", summary)
	}
	if strings.Contains(summary, "one change") {
		t.Errorf("the record kept the line %q, which says nothing about what was written", summary)
	}
}

// TestARecordWriteWithNoJSONBehindItIsStillOneChange proves the fallback is
// still there for an answer that carries no update at all.
func TestARecordWriteWithNoJSONBehindItIsStillOneChange(t *testing.T) {
	summary := summaryOfResult(contract.ToolTask, "the record's why is written", false)

	if !strings.Contains(summary, "one change") {
		t.Errorf("the record kept the line %q for an answer with no update behind it, want one change", summary)
	}
}

// TestARefusedRecordWriteIsNeverWrittenDownAsAChange is the sibling from brief
// 3.7 that is already on main, kept beside its neighbour: the model went on
// believing a write had happened that the record had refused.
func TestARefusedRecordWriteIsNeverWrittenDownAsAChange(t *testing.T) {
	summary := summaryOfResult(contract.ToolTask, "the record refused that change: a decision needs a reason", true)

	if !strings.Contains(summary, "was refused") {
		t.Errorf("the record kept the line %q for a refused write, and a refusal is never a change", summary)
	}
}

// TestAChangeWhoseTestsRanIsSummarisedByTheTestsFirst is the thirteenth
// nightly run's game job: forty-three of its two hundred and three rounds
// only ran the tests, and sixteen of those runs came right after an edit
// whose result already carried the tests' line, three lines down, where the
// one line the record keeps never showed it. The line now leads with the
// tests, so the model reads what its change did without asking again.
func TestAChangeWhoseTestsRanIsSummarisedByTheTestsFirst(t *testing.T) {
	text := "edited /game/src/engine.js by the exact matcher; the file now holds 4961 bytes\n\nparses\n" + TheTestsAfterAChange + "2 failing of 13: clears a full row; locks the piece"
	summary := summaryOfResult(contract.ToolEdit, text, false)
	if !strings.HasPrefix(summary, "edit: "+TheTestsAfterAChange+"2 failing of 13") || !strings.Contains(summary, "engine.js") {
		t.Errorf("the summary reads %q, want the tests after the change first and the file edited after them", summary)
	}
	plain := summaryOfResult(contract.ToolWrite, "created /game/src/engine.js, 120 bytes\n", false)
	if plain != "write: created /game/src/engine.js, 120 bytes" {
		t.Errorf("a change with no test run after it reads %q, want its own first line as before", plain)
	}
}
