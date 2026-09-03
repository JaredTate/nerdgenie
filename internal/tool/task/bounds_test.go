package task_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/task"
)

// TestOneFieldInTheWrongShapeDoesNotKillTheWholeCall is finding 27 of the wave
// 6 gate review. A why written as a list, as an object, or as a number all came
// back as "cannot read this call's arguments as JSON, so write an object with
// an operation in it: json: cannot unmarshal array into Go struct field
// input.why of type string": the call did have an operation, the message said
// it did not, and the one good sentence inside it was buried behind a
// misleading prefix. A list and an object are now read for the line they hold,
// and anything else is refused by the field's own name in plain words.
func TestOneFieldInTheWrongShapeDoesNotKillTheWholeCall(t *testing.T) {
	asAList, keeper := newTool(t)
	if _, err := run(t, asAList, map[string]any{"operation": "why", "why": []any{"the release is on Friday"}}); err != nil {
		t.Fatalf("a why written as a list was refused: %v", err)
	}
	if keeper.Record().Goal.Why != "the release is on Friday" {
		t.Errorf("the why reads %q after a why written as a list", keeper.Record().Goal.Why)
	}

	asAnObject, held := newTool(t)
	if _, err := run(t, asAnObject, map[string]any{
		"operation": "decision",
		"text":      map[string]any{"text": "write from the tags"},
		"reason":    "the tags are the only complete list",
	}); err != nil {
		t.Fatalf("a decision whose text was written as an object was refused: %v", err)
	}
	if decisions := held.Record().Lessons.Decisions; len(decisions) != 1 || decisions[0].Text != "write from the tags" {
		t.Errorf("the decisions came out as %+v", held.Record().Lessons.Decisions)
	}

	asANumber, _ := newTool(t)
	_, err := run(t, asANumber, map[string]any{"operation": "why", "why": 42})
	if err == nil {
		t.Fatalf("a why written as a number was taken")
	}
	if !strings.Contains(err.Error(), `"why"`) {
		t.Errorf("the refusal reads %q and does not name the field that is wrong", err)
	}
	for _, jargon := range []string{"json:", "Go struct", "cannot read this call's arguments"} {
		if strings.Contains(err.Error(), jargon) {
			t.Errorf("the refusal reads %q, which says %q rather than what to write", err, jargon)
		}
	}
}

// TestTheNumberThisToolIsBoundedByIsTheOneTheDesignNames pins MaxLines, which
// finding 27 found could be halved with the package still green, and walks the
// branch that refuses a list over the cap.
func TestTheNumberThisToolIsBoundedByIsTheOneTheDesignNames(t *testing.T) {
	// Fifty lines is a long done list and a long plan; work that needs more
	// steps than that is a job made of tasks rather than one task.
	if task.MaxLines != 50 {
		t.Errorf("a list may hold %d lines, want 50", task.MaxLines)
	}

	tool, _ := newTool(t)
	tooMany := []any{}
	for at := range task.MaxLines + 1 {
		tooMany = append(tooMany, "step "+strconv.Itoa(at+1))
	}
	_, err := run(t, tool, map[string]any{"operation": "plan", "plan": tooMany})
	if err == nil {
		t.Fatalf("a plan of %d steps was taken, and the cap is %d", len(tooMany), task.MaxLines)
	}
	if !strings.Contains(err.Error(), "the cap is 50") {
		t.Errorf("the refusal reads %q and does not say what the cap is", err)
	}
}
