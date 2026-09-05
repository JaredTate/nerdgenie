package loop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aRecordWithOneDoneLine is the record the reader is given in these tests, so
// that the operation which points a done line at its result has a line to point
// at.
func aRecordWithOneDoneLine() contract.Record {
	return contract.Record{Goal: contract.Goal{DoneWhen: []contract.DoneLine{{Text: "the notes are read"}}}}
}

// TestTheRecordReaderSaysWhatToWriteInstead proves every refusal of the loop's
// own record reader names what the model should write instead, which is rule 6
// of design section 3 read one call at a time.
func TestTheRecordReaderSaysWhatToWriteInstead(t *testing.T) {
	for _, one := range []struct {
		what      string
		arguments string
		holds     string
	}{
		{"a pin with no result", `{"operation":"pin_result","line":1}`, "such as r7"},
		{"a pin of a line that is not there", `{"operation":"pin_result","line":9,"result":"r1"}`, "numbered 9"},
		{"a pin of no line at all", `{"operation":"pin_result","result":"r1"}`, "numbered 0"},
		{"a call that writes nothing", `{}`, "says nothing this record can hold"},
		{"arguments that are not an object", `["why"]`, "do not read as an object"},
		{"a line number that is not a number", `{"operation":"pin_result","line":"soon","result":"r1"}`, "whole number"},
	} {
		_, err := readRecordUpdate(json.RawMessage(one.arguments), aRecordWithOneDoneLine())
		if err == nil {
			t.Errorf("%s was taken, and the record could not have held it", one.what)
			continue
		}
		if !strings.Contains(err.Error(), one.holds) {
			t.Errorf("%s was refused with %q, which does not say what to write instead", one.what, err)
		}
	}
}

// TestALineNumberIsReadAsANumberOrAsAQuotedOne proves the reading a model needs
// when it writes the line number as text, which small models do.
func TestALineNumberIsReadAsANumberOrAsAQuotedOne(t *testing.T) {
	for _, written := range []string{`{"operation":"pin_result","line":1,"result":"r1"}`,
		`{"operation":"pin_result","line":"1","result":"r1"}`} {
		update, err := readRecordUpdate(json.RawMessage(written), aRecordWithOneDoneLine())
		if err != nil {
			t.Fatalf("the call %s was refused: %v", written, err)
		}
		if len(update.DoneWhen) != 1 || !update.DoneWhen[0].Done || update.DoneWhen[0].ResultID != "r1" {
			t.Errorf("the call %s was read as %+v, want the one line ticked and pointing at r1", written, update.DoneWhen)
		}
	}
}

// TestTheLoopsOwnTaskToolReadsAStepDone holds the loop's copy of the task
// tool to the same door: step_done with a step and a result reads as a step
// mark and nothing else.
func TestTheLoopsOwnTaskToolReadsAStepDone(t *testing.T) {
	update, err := readRecordUpdate(json.RawMessage(`{"operation":"step_done","step":2,"result":"r1"}`), contract.Record{})
	if err != nil {
		t.Fatalf("a step_done write was refused: %v", err)
	}
	if update.StepDone == nil || update.StepDone.Number != 2 || update.StepDone.ResultID != "r1" {
		t.Errorf("the write reads %+v, want step 2 marked by r1", update.StepDone)
	}
	if update.Plan != nil || update.DoneWhen != nil {
		t.Errorf("the write also carries a plan or a done list: %+v", update)
	}
	if _, err := readRecordUpdate(json.RawMessage(`{"operation":"step_done","result":"r1"}`), contract.Record{}); err == nil {
		t.Error("a step_done naming no step was taken")
	}
}
