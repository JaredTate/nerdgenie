package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/task"
)

// newTool builds the task tool over a real record with a real event log behind
// it, because the rules this tool hands back are the record's own.
func newTool(t *testing.T) (*task.Tool, *record.Keeper) {
	t.Helper()
	keeper, err := record.New(context.Background(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: "17", Origin: "terminal",
		Ask: "write up the release notes", RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot start a record for the test: %v", err)
	}
	return task.New(task.Settings{Records: keeper}), keeper
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *task.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolTask {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolTask)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	if len(spec.Fields) == 0 || spec.Fields[0].Name != "operation" {
		t.Errorf("the tool's first field is %v, and every call says which operation it is", spec.Fields)
	}
	if len(spec.Classes) != 1 || spec.Classes[0] != contract.ClassWrite {
		t.Errorf("the tool claims the classes %v, and it writes the record", spec.Classes)
	}
}

func TestTheWhyIsWrittenOnceAndThenStands(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{"operation": "why", "why": "the release is on Friday"}); err != nil {
		t.Fatalf("setting the why failed: %v", err)
	}
	if keeper.Record().Goal.Why != "the release is on Friday" {
		t.Errorf("the why reads %q after the call", keeper.Record().Goal.Why)
	}

	_, err := run(t, tool, map[string]any{"operation": "why", "why": "something else entirely"})
	if err == nil {
		t.Fatalf("the why was written a second time")
	}
	if !errors.Is(err, record.ErrWhyIsSet) {
		t.Errorf("the refusal is %v, want the record's own rule about the why", err)
	}
	if !strings.Contains(err.Error(), "written once") {
		t.Errorf("the refusal reads %q and does not hand the model the rule it broke", err)
	}
}

func TestTheDoneListTheStopListAndThePlanAreWritten(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"operation": "done_when",
		"done_when": []any{map[string]any{"text": "every version has a page"}},
	}); err != nil {
		t.Fatalf("writing the done list failed: %v", err)
	}
	if _, err := run(t, tool, map[string]any{
		"operation": "stop_when", "stop_when": []any{"the release is cancelled"},
	}); err != nil {
		t.Fatalf("writing the stop list failed: %v", err)
	}
	if _, err := run(t, tool, map[string]any{
		"operation": "plan", "plan": []any{"read the tags", "write one page each"},
	}); err != nil {
		t.Fatalf("writing the plan failed: %v", err)
	}

	held := keeper.Record()
	if len(held.Goal.DoneWhen) != 1 || len(held.Rules.StopWhen) != 1 || len(held.Work.Plan) != 2 {
		t.Fatalf("the record holds %d done lines, %d stop lines, and %d plan steps",
			len(held.Goal.DoneWhen), len(held.Rules.StopWhen), len(held.Work.Plan))
	}
	if held.Work.Plan[1].Number != 2 {
		t.Errorf("the second plan step is numbered %d, and the steps count from one", held.Work.Plan[1].Number)
	}
}

func TestADecisionNeedsItsReasonAndAFailureItsCause(t *testing.T) {
	tool, keeper := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"operation": "decision", "text": "write the notes from the tags", "reason": "the tags are the only complete list",
	}); err != nil {
		t.Fatalf("adding a decision failed: %v", err)
	}
	if _, err := run(t, tool, map[string]any{
		"operation": "failure", "text": "the tag list was empty", "cause": "the repository was not fetched",
	}); err != nil {
		t.Fatalf("adding a failure failed: %v", err)
	}
	held := keeper.Record()
	if len(held.Lessons.Decisions) != 1 || len(held.Lessons.Failures) != 1 {
		t.Fatalf("the record holds %d decisions and %d failures", len(held.Lessons.Decisions), len(held.Lessons.Failures))
	}

	noReason, err := run(t, tool, map[string]any{"operation": "decision", "text": "a choice with no reason"})
	if err == nil {
		t.Fatalf("a decision with no reason was written, and it said %q", noReason.Text)
	}
	if !errors.Is(err, record.ErrDecisionNeedsReason) {
		t.Errorf("the refusal is %v, want the record's rule about a decision's reason", err)
	}
	if _, err := run(t, tool, map[string]any{"operation": "failure", "text": "something went wrong"}); !errors.Is(err, record.ErrFailureNeedsCause) {
		t.Errorf("the refusal is %v, want the record's rule about a failure's cause", err)
	}
}

func TestPinningAResultMarksTheDoneLineItProves(t *testing.T) {
	tool, keeper := newTool(t)
	if _, err := run(t, tool, map[string]any{
		"operation": "done_when", "done_when": []any{map[string]any{"text": "every version has a page"}},
	}); err != nil {
		t.Fatalf("writing the done list failed: %v", err)
	}
	id, err := keeper.AddResult(context.Background(), "the notes, 800 words", "the whole of the notes")
	if err != nil {
		t.Fatalf("cannot add a result to the record: %v", err)
	}

	output, err := run(t, tool, map[string]any{"operation": "pin_result", "line": 1, "result": id})
	if err != nil {
		t.Fatalf("pinning a result failed: %v", err)
	}
	testkit.Golden(t, "a_pinned_result.txt", []byte(output.Text))

	held := keeper.Record().Goal.DoneWhen[0]
	if !held.Done || held.ResultID != id {
		t.Errorf("the done line reads %+v after the pin, want it marked done by %s", held, id)
	}
}

func TestPinningAResultTheRecordNeverWroteIsRefused(t *testing.T) {
	tool, _ := newTool(t)
	if _, err := run(t, tool, map[string]any{
		"operation": "done_when", "done_when": []any{map[string]any{"text": "every version has a page"}},
	}); err != nil {
		t.Fatalf("writing the done list failed: %v", err)
	}

	_, err := run(t, tool, map[string]any{"operation": "pin_result", "line": 1, "result": "r9"})
	if err == nil {
		t.Fatalf("a done line was marked done by a result the record never wrote")
	}
	if !errors.Is(err, record.ErrNoSuchResult) {
		t.Errorf("the refusal is %v, want the record's rule about proof that points at nothing", err)
	}
}

func TestPinningToALineThatIsNotThereIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := run(t, tool, map[string]any{"operation": "pin_result", "line": 3, "result": "r1"}); err == nil {
		t.Errorf("a result was pinned to a done line that is not there")
	}
}

func TestADoneLineMarkedDoneWithNothingBehindItIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"operation": "done_when",
		"done_when": []any{map[string]any{"text": "every version has a page", "done": true}},
	})
	if err == nil {
		t.Fatalf("a done line was marked done with nothing behind it")
	}
	if !errors.Is(err, record.ErrDoneLineNeedsProof) {
		t.Errorf("the refusal is %v, want the record's rule about proof", err)
	}
}

func TestTheAskAndTheCorrectionsCannotBeWrittenThroughThisTool(t *testing.T) {
	tool, keeper := newTool(t)

	for _, forbidden := range []string{"ask", "correction", "situation", "budget", "result"} {
		if _, err := run(t, tool, map[string]any{"operation": forbidden, "text": "no"}); err == nil {
			t.Errorf("the operation %q was allowed, and it belongs to the user or the harness", forbidden)
		}
	}
	if keeper.Record().Goal.Ask != "write up the release notes" {
		t.Errorf("the ask reads %q, and nothing may ever change it", keeper.Record().Goal.Ask)
	}
}

// TestAnOperationInAnotherCaseOrWithACommonPrefixIsTaken is finding 26 of the
// wave 6 gate review, and the live run's first finding one layer up. The name
// was compared letter for letter, so DONE_WHEN, doneWhen, done-when, set_why,
// add_decision and set_plan were each refused outright, costing a full round
// apiece, and the refusal read the same list every time, so a model that went
// on writing set_why went on getting the same answer.
func TestAnOperationInAnotherCaseOrWithACommonPrefixIsTaken(t *testing.T) {
	for _, taken := range []struct {
		fields  map[string]any
		written string
	}{
		{map[string]any{"operation": "DONE_WHEN", "done_when": []any{"the page is up"}}, "the done list"},
		{map[string]any{"operation": "doneWhen", "done_when": []any{"the page is up"}}, "the done list"},
		{map[string]any{"operation": "done-when", "done_when": []any{"the page is up"}}, "the done list"},
		{map[string]any{"operation": "set_why", "why": "the release is on Friday"}, "the why"},
		{map[string]any{"operation": "add_decision", "text": "write from the tags", "reason": "they are complete"}, "a decision"},
		{map[string]any{"operation": "set_plan", "plan": []any{"read the tags"}}, "the plan"},
		{map[string]any{"operation": "update_stop_when", "stop_when": []any{"the release is cancelled"}}, "the stop list"},
	} {
		tool, keeper := newTool(t)
		if _, err := run(t, tool, taken.fields); err != nil {
			t.Errorf("the call %v was refused although it writes %s: %v", taken.fields, taken.written, err)
			continue
		}
		held := keeper.Record()
		if held.Goal.Why == "" && len(held.Goal.DoneWhen) == 0 && len(held.Rules.StopWhen) == 0 &&
			len(held.Work.Plan) == 0 && len(held.Lessons.Decisions) == 0 {
			t.Errorf("the call %v was taken and %s was not written", taken.fields, taken.written)
		}
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{},
		{"operation": "why"},
		{"operation": "plan", "plan": []any{}},
		{"operation": "plan", "plan": []any{""}},
		{"operation": "stop_when", "stop_when": []any{""}},
		{"operation": "done_when", "done_when": []any{map[string]any{"text": ""}}},
		{"operation": "decision", "reason": "a reason with no choice"},
		{"operation": "pin_result", "result": "r1"},
		{"operation": "pin_result", "line": 1},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoRecordWiredInSaysSo(t *testing.T) {
	tool := task.New(task.Settings{})

	_, err := run(t, tool, map[string]any{"operation": "why", "why": "anything"})
	if err == nil {
		t.Fatalf("the record was written with no record behind the tool")
	}
	if !strings.Contains(err.Error(), "record") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

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
		{map[string]any{"operation": "pin_result", "line": "one", "result": "r1"}, "whole number"},
		{map[string]any{"operation": "pin_result", "line": nil, "result": "r1"}, "no done line"},
	} {
		_, err := run(t, tool, broken.fields)
		if err == nil || !strings.Contains(err.Error(), broken.named) {
			t.Errorf("the call %v was not refused with %q named: %v", broken.fields, broken.named, err)
		}
	}
}
