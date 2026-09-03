package task_test

import (
	"context"
	"strings"
	"testing"
)

// The shapes a model plausibly writes, one test each. Every one of them was a
// stall in a real run: the first trial's seven refusals in a row, and the four
// refused record writes the wave 6 gate review found.

// TestADoneLineWrittenAsAPlainStringIsTakenAsItsText reproduces what the local
// model did in the first trial: it wrote the done list as a list of strings,
// the tool refused every one of seven tries, and the task went nowhere. A
// string is the line's text; an object still works; anything else is refused
// with the shape named.
func TestADoneLineWrittenAsAPlainStringIsTakenAsItsText(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"operation": "done_when",
		"done_when": []any{"the folder exists", map[string]any{"text": "the tests pass"}},
	}); err != nil {
		t.Fatalf("a done list written as strings was refused: %v", err)
	}
	lines := keeper.Record().Goal.DoneWhen
	if len(lines) != 2 || lines[0].Text != "the folder exists" || lines[1].Text != "the tests pass" {
		t.Errorf("the done list came out as %+v", lines)
	}

	_, err := run(t, tool, map[string]any{"operation": "done_when", "done_when": []any{42}})
	if err == nil || !strings.Contains(err.Error(), `"text"`) {
		t.Errorf("a done line that is a number was not refused with the shape named: %v", err)
	}
}

// TestTheShapesAModelPlausiblyWritesAreAllTaken reproduces the second stall of
// the first trial: the model wrote the why in the "text" field and was refused
// on every try. The why is taken from "text" when "why" is empty, a list may be
// one string, the done list may be one line, and a line number may be written
// as a string.
func TestTheShapesAModelPlausiblyWritesAreAllTaken(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{"operation": "why", "text": "the user wants a game"}); err != nil {
		t.Fatalf("a why written in the text field was refused: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"operation": "plan", "plan": "write the tests first"}); err != nil {
		t.Fatalf("a plan written as one string was refused: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"operation": "stop_when", "stop_when": "the user says stop"}); err != nil {
		t.Fatalf("a stop list written as one string was refused: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"operation": "done_when", "done_when": "the game runs"}); err != nil {
		t.Fatalf("a done list written as one string was refused: %v", err)
	}
	id, err := keeper.AddResult(context.Background(), "the game, 40 files", "the whole of the game")
	if err != nil {
		t.Fatalf("cannot add a result to the record: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"operation": "pin_result", "line": "1", "result": id}); err != nil {
		t.Fatalf("a line number written as a string was refused: %v", err)
	}
	held := keeper.Record()
	if held.Goal.Why != "the user wants a game" || len(held.Work.Plan) != 1 || len(held.Rules.StopWhen) != 1 ||
		len(held.Goal.DoneWhen) != 1 || !held.Goal.DoneWhen[0].Done {
		t.Errorf("the record came out as why=%q plan=%v stop=%v done=%+v", held.Goal.Why, held.Work.Plan, held.Rules.StopWhen, held.Goal.DoneWhen)
	}
}

// TestARecordWriteMayCarrySeveralSectionsAtOnce reproduces two more shapes
// from the trial's event log: one call carrying the why, the done list and the
// plan together, and a done line whose text is under "line". Both are taken,
// and a call with an operation that names one section while another section
// is the one written still writes what it carries.
func TestARecordWriteMayCarrySeveralSectionsAtOnce(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"operation": "done_when",
		"why":       "the user wants a game",
		"done_when": []any{map[string]any{"line": "the tests pass"}},
		"plan":      []any{"write the tests", "write the game"},
	}); err != nil {
		t.Fatalf("a call carrying three sections was refused: %v", err)
	}
	held := keeper.Record()
	if held.Goal.Why != "the user wants a game" || len(held.Work.Plan) != 2 ||
		len(held.Goal.DoneWhen) != 1 || held.Goal.DoneWhen[0].Text != "the tests pass" {
		t.Errorf("the record came out as why=%q plan=%v done=%+v", held.Goal.Why, held.Work.Plan, held.Goal.DoneWhen)
	}

	if _, err := run(t, tool, map[string]any{"operation": "done_when", "stop_when": []any{"the user says stop"}}); err != nil {
		t.Fatalf("a call naming one section and carrying another was refused: %v", err)
	}
	if len(keeper.Record().Rules.StopWhen) != 1 {
		t.Errorf("the stop list was not written: %v", keeper.Record().Rules.StopWhen)
	}
	if _, err := run(t, tool, map[string]any{"operation": "plan"}); err == nil {
		t.Errorf("a call that writes nothing was taken")
	}
}

// TestACallWithNoOperationIsReadFromTheFieldsItCarries covers the rest of
// finding 25's inference, which the forty-step fixture only half exercises: a
// text with a cause is a failure, a line with a result is a pin, a decision
// written beside a section is written with it rather than dropped, and a
// decision whose reason is written outside the object still carries it.
func TestACallWithNoOperationIsReadFromTheFieldsItCarries(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"text": "the tag list was empty", "cause": "the repository was not fetched",
	}); err != nil {
		t.Fatalf("a failure with no operation named was refused: %v", err)
	}
	if _, err := run(t, tool, map[string]any{
		"why":      "the release is on Friday",
		"decision": "write the notes from the tags",
		"reason":   "the tags are the only complete list",
	}); err != nil {
		t.Fatalf("a why written beside a decision was refused: %v", err)
	}
	held := keeper.Record()
	if len(held.Lessons.Failures) != 1 || len(held.Lessons.Decisions) != 1 || held.Goal.Why == "" {
		t.Fatalf("the record holds %d failures, %d decisions, and the why %q",
			len(held.Lessons.Failures), len(held.Lessons.Decisions), held.Goal.Why)
	}

	if _, err := run(t, tool, map[string]any{"done_when": []any{"every version has a page"}}); err != nil {
		t.Fatalf("a done list with no operation named was refused: %v", err)
	}
	id, err := keeper.AddResult(context.Background(), "the notes, 800 words", "the whole of the notes")
	if err != nil {
		t.Fatalf("cannot add a result to the record: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"line": 1, "resultId": id}); err != nil {
		t.Fatalf("a pin with no operation named and the result under resultId was refused: %v", err)
	}
	if line := keeper.Record().Goal.DoneWhen[0]; !line.Done || line.ResultID != id {
		t.Errorf("the done line reads %+v after the pin, want it marked done by %s", line, id)
	}
}

// TestADoneLineCarriesItsProofUnderEitherName covers the two names a model
// gives the result and the user's reply on a done line, which the loop's own
// reader has always taken and this tool did not.
func TestADoneLineCarriesItsProofUnderEitherName(t *testing.T) {
	tool, keeper := newTool(t)
	id, err := keeper.AddResult(context.Background(), "the notes, 800 words", "the whole of the notes")
	if err != nil {
		t.Fatalf("cannot add a result to the record: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"operation": "done_when", "done_when": []any{
		map[string]any{"text": "every version has a page", "done": true, "resultId": id},
		map[string]any{"text": "the user is happy with it", "done": true, "userReply": "that will do"},
	}}); err != nil {
		t.Fatalf("a done list whose proof is under resultId and userReply was refused: %v", err)
	}
	lines := keeper.Record().Goal.DoneWhen
	if len(lines) != 2 || lines[0].ResultID != id || lines[1].UserReply != "that will do" {
		t.Errorf("the done list came out as %+v", lines)
	}
}

// TestTheShapesTheTaskToolRefusesAreNamed covers the edges of the lenient
// reader: a null list is an empty one, a list that is neither a string nor a
// list is refused with the shape named, and so is a line number that is not a
// number.
func TestTheShapesTheTaskToolRefusesAreNamed(t *testing.T) {
	tool, _ := newTool(t)
	for _, broken := range []struct {
		fields map[string]any
		named  string
	}{
		{map[string]any{"operation": "plan", "plan": nil}, "writes nothing"},
		{map[string]any{"operation": "plan", "plan": 7}, "list"},
		{map[string]any{"operation": "done_when", "done_when": nil}, "writes nothing"},
		{map[string]any{"operation": "done_when", "done_when": 7}, `"text"`},
		{map[string]any{"operation": "pin_result", "line": "one", "result": "r1"}, "whole number"},
		{map[string]any{"operation": "pin_result", "line": nil, "result": "r1"}, "no done line"},
		{map[string]any{"operation": "pin_result", "line": "", "result": "r1"}, "no done line"},
		{map[string]any{"operation": "why", "why": []any{}}, "writes nothing"},
		{map[string]any{"operation": "decision", "decision": 7}, `"decision"`},
		{map[string]any{"operation": "failure", "failure": []any{7}}, `"failure"`},
	} {
		_, err := run(t, tool, broken.fields)
		if err == nil || !strings.Contains(err.Error(), broken.named) {
			t.Errorf("the call %v was not refused with %q named: %v", broken.fields, broken.named, err)
		}
	}
}
