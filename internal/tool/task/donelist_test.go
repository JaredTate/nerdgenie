package task_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/record"
)

// aDoneListOf writes a done list of the length asked for, the way a model
// writes one: each line as plain text with nothing behind it yet.
func aDoneListOf(count int) []any {
	lines := make([]any, 0, count)
	for at := range count {
		lines = append(lines, "piece "+strconv.Itoa(at+1)+" of the game works")
	}
	return lines
}

// TestADoneListOfFiveLinesIsWrittenAndSixAreRefusedAsAJob is the five-line
// rule as the model meets it: the record refuses the sixth line, and this tool
// hands the refusal back word for word, so that the model reads which tool to
// use instead and what to do with it.
func TestADoneListOfFiveLinesIsWrittenAndSixAreRefusedAsAJob(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{"operation": "done_when", "done_when": aDoneListOf(record.MaxDoneLines)}); err != nil {
		t.Fatalf("a done list of %d lines was refused: %v", record.MaxDoneLines, err)
	}
	if held := len(keeper.Record().Goal.DoneWhen); held != record.MaxDoneLines {
		t.Fatalf("the record holds %d done lines after writing %d", held, record.MaxDoneLines)
	}

	_, err := run(t, tool, map[string]any{"operation": "done_when", "done_when": aDoneListOf(record.MaxDoneLines + 1)})
	if err == nil {
		t.Fatalf("a done list of %d lines was taken, and a task holds at most %d", record.MaxDoneLines+1, record.MaxDoneLines)
	}
	if !errors.Is(err, record.ErrDoneListTooLong) {
		t.Errorf("the refusal is %v, want the record's own rule about the length of a done list", err)
	}
	for _, told := range []string{"this ask is a job", "job tool", "one task per done line", "work the first task"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not tell the model %q", err, told)
		}
	}
	if held := len(keeper.Record().Goal.DoneWhen); held != record.MaxDoneLines {
		t.Errorf("the record holds %d done lines after the refusal, and the five that were written should still stand", held)
	}
}

// TestAWholeGameWrittenAsOneTaskIsRefusedBeforeAnyOfItIsWritten is the call
// the Tetris run made: the why, an eight-line done list and a plan in one call.
// The record is all or nothing, so none of it lands, and the model is sent to
// the job tool with the whole of the ask still in front of it.
func TestAWholeGameWrittenAsOneTaskIsRefusedBeforeAnyOfItIsWritten(t *testing.T) {
	tool, keeper := newTool(t)

	_, err := run(t, tool, map[string]any{
		"why":       "the user wants a playable game with tests",
		"done_when": aDoneListOf(8),
		"plan":      []any{"write the board", "write the pieces", "write the tests"},
	})
	if err == nil {
		t.Fatalf("an eight-line done list was taken as one task")
	}
	if !errors.Is(err, record.ErrDoneListTooLong) {
		t.Errorf("the refusal is %v, want the rule about the length of a done list", err)
	}
	held := keeper.Record()
	if held.Goal.Why != "" || len(held.Work.Plan) != 0 {
		t.Errorf("the refused call wrote the why %q and %d plan steps, and a refused update writes nothing", held.Goal.Why, len(held.Work.Plan))
	}
}

// TestTheDoneListFieldSaysFiveLinesIsTheMost holds that the model is told the
// rule where it reads the tool, and not only when it has already broken it.
func TestTheDoneListFieldSaysFiveLinesIsTheMost(t *testing.T) {
	tool, _ := newTool(t)
	for _, field := range tool.Spec().Fields {
		if field.Name != "done_when" {
			continue
		}
		if !strings.Contains(field.Description, "at most five lines") {
			t.Errorf("the done_when field reads %q and does not say that five lines is the most", field.Description)
		}
		return
	}
	t.Fatal("the tool has no done_when field")
}
