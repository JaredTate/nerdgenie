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
