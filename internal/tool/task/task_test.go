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

// TestTheWholeUpdateComesBackAsTheResult is finding 28 of the wave 6 gate
// review. The tool answered "the record's why is written" and nothing else,
// where the loop's own path had handed back the whole update, so `read r2`
// fetched one sentence instead of the change, and the loop's summary of every
// record write read "updated the record: one change", because it tries to read
// the result as JSON and a sentence is not JSON.
func TestTheWholeUpdateComesBackAsTheResult(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{
		"operation": "done_when",
		"why":       "the release is on Friday",
		"done_when": []any{"every version has a page"},
		"plan":      []any{"read the tags", "write one page each"},
	})
	if err != nil {
		t.Fatalf("writing three sections failed: %v", err)
	}
	if !strings.Contains(output.Text, "why") || !strings.Contains(output.Text, "done_when") ||
		!strings.Contains(output.Text, "plan") {
		t.Errorf("the result reads %q and does not name every section it wrote", output.Text)
	}

	held := map[string]json.RawMessage{}
	starts := strings.Index(output.Text, "{")
	if starts < 0 {
		t.Fatalf("the result carries no update as JSON, so read r2 fetches a sentence: %q", output.Text)
	}
	written := output.Text[starts:]
	if err := json.Unmarshal([]byte(written), &held); err != nil {
		t.Fatalf("the result carries no update as JSON, so read r2 fetches a sentence: %q", output.Text)
	}
	for _, name := range []string{"why", "done_when", "plan"} {
		if _, wrote := held[name]; !wrote {
			t.Errorf("the update came back as %s and does not hold %q", written, name)
		}
	}
	if string(held["why"]) != `"the release is on Friday"` {
		t.Errorf("the update's why came back as %s", held["why"])
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

// TestTheOperationsTheLoopAnswersAreNamedToTheModelAndRefusedHere covers the
// three operations the turn loop answers before this tool is asked: the
// description names them, so the model learns they exist, and a call that
// somehow reaches the tool with one of them is refused naming the harness.
func TestTheOperationsTheLoopAnswersAreNamedToTheModelAndRefusedHere(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()
	words := ""
	for _, field := range spec.Fields {
		words += field.Description + " "
	}
	for _, name := range []string{"stop_now", "pin_evidence", "unpin_evidence", "\"reply\""} {
		if !strings.Contains(words, name) {
			t.Errorf("the description says nothing about %s", name)
		}
	}
	for _, operation := range []string{"stop_now", "pin_evidence", "unpin_evidence"} {
		_, err := run(t, tool, map[string]any{"operation": operation, "line": 1, "result": "r1"})
		if err == nil || !strings.Contains(err.Error(), "harness") {
			t.Errorf("the operation %s reached the tool and was not refused naming the harness: %v", operation, err)
		}
	}
}
