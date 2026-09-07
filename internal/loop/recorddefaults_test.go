package loop

import (
	"encoding/json"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A step mark or a pin that names no result takes the newest result the
// record holds, which is the one the model just read: 27 of the day's record
// writes were refused for a missing result, a round each, when the harness
// held the answer.

func TestAStepDoneWithNoResultTakesTheNewestResult(t *testing.T) {
	held := contract.Record{Work: contract.Work{Results: []contract.ResultLine{{ID: "r1", Summary: "read"}, {ID: "r2", Summary: "tests: all passing"}}}}

	update, err := readRecordUpdate(json.RawMessage(`{"operation":"step_done","step":2}`), held)
	if err != nil {
		t.Fatalf("a step_done with no result was refused though the record holds results: %v", err)
	}
	if update.StepDone == nil || update.StepDone.Number != 2 || update.StepDone.ResultID != "r2" {
		t.Errorf("the write reads %+v, want step 2 marked by the newest result r2", update.StepDone)
	}
	if _, err := readRecordUpdate(json.RawMessage(`{"operation":"step_done","step":2}`), contract.Record{}); err == nil {
		t.Error("a step_done with no result was taken on a record with no results to take")
	}
}

func TestAPinWithNoResultTakesTheNewestResult(t *testing.T) {
	held := contract.Record{
		Goal: contract.Goal{DoneWhen: []contract.DoneLine{{Text: "the tests pass"}}},
		Work: contract.Work{Results: []contract.ResultLine{{ID: "r1", Summary: "read"}, {ID: "r3", Summary: "tests: all passing"}}},
	}

	update, err := readRecordUpdate(json.RawMessage(`{"operation":"pin_result","line":1}`), held)
	if err != nil {
		t.Fatalf("a pin with no result was refused though the record holds results: %v", err)
	}
	if len(update.DoneWhen) != 1 || !update.DoneWhen[0].Done || update.DoneWhen[0].ResultID != "r3" {
		t.Errorf("the write reads %+v, want line 1 proven by the newest result r3", update.DoneWhen)
	}
}
