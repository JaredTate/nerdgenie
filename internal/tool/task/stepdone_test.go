package task_test

import (
	"strings"
	"testing"
)

// TestStepDoneMarksAPlanStepThroughTheTool holds the door the model uses to
// check a step off: step_done with the step's number and the result that
// proves it, the number written as a number or as a word, and the answer
// naming what was written.
func TestStepDoneMarksAPlanStepThroughTheTool(t *testing.T) {
	tool, keeper := newTool(t)
	if _, err := run(t, tool, map[string]any{"operation": "plan", "plan": []any{"write the tests", "make them pass", "write the readme"}}); err != nil {
		t.Fatalf("writing the plan failed: %v", err)
	}
	id, err := keeper.AddResult(t.Context(), "shell: 12 tests passing", "12 tests passing")
	if err != nil {
		t.Fatalf("cannot add a result to the record: %v", err)
	}

	output, err := run(t, tool, map[string]any{"operation": "step_done", "step": 2, "result": id})
	if err != nil {
		t.Fatalf("marking step 2 done failed: %v", err)
	}
	if !strings.Contains(output.Text, "step_done") {
		t.Errorf("the tool answered %q, want it to name what was written", output.Text)
	}
	if plan := keeper.Record().Work.Plan; !plan[1].Done || plan[1].ResultID != id {
		t.Errorf("step 2 reads %+v, want it done and pointing at %s", plan[1], id)
	}
	if _, err := run(t, tool, map[string]any{"operation": "step_done", "step": "3", "result": id}); err != nil {
		t.Errorf("a step number written as a string was refused: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"operation": "step_done", "result": id}); err == nil || !strings.Contains(err.Error(), "step") {
		t.Errorf("a step_done naming no step was not refused with the word step: %v", err)
	}
}
