package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// A plan step is marked done, and a done line proved, by the result that
// shows it. The model can say so on its first line or through the task tool,
// and on run twenty-one it used the tool: sixteen of eighty-six calls did
// nothing else, a round each. So a call to a file tool may carry the marks
// itself: "done", the plan step this call finishes, and "proves", the done
// line it proves. The harness takes them off before the tool and the guard
// see the call, and marks them with the call's own result once it succeeds.
// A failed call marks nothing, and the result says so.

// TheMarkFields are the two fields a file tool call may carry.
var TheMarkFields = []contract.ToolField{
	{Name: "done", Type: "integer", Description: "The plan step this call finishes, counting from one; the harness marks it done by this call's result."},
	{Name: "proves", Type: "integer", Description: "The done line this call proves, counting from one; the harness points the line at this call's result."},
}

// theToolsThatCarryMarks are the tools whose calls do work a step or a line
// can be marked by.
var theToolsThatCarryMarks = []string{contract.ToolRead, contract.ToolWrite, contract.ToolEdit, contract.ToolShell, contract.ToolSearch}

// marks are what one call asked to have marked by its result.
type marks struct {
	step int
	line int
}

// theMarksOf takes the two fields off a call and returns the call without
// them, so that the tool and the same-call guard see the call itself.
func theMarksOf(call contract.ToolCall) (contract.ToolCall, marks) {
	if !slices.Contains(theToolsThatCarryMarks, call.Name) {
		return call, marks{}
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(call.Input, &fields); err != nil {
		return call, marks{}
	}
	asked := marks{step: numberIn(fields, "done"), line: numberIn(fields, "proves")}
	if asked == (marks{}) {
		return call, marks{}
	}
	delete(fields, "done")
	delete(fields, "proves")
	input, err := json.Marshal(fields)
	if err != nil {
		return call, marks{}
	}
	call.Input = input
	return call, asked
}

// numberIn reads one whole number off a field, or zero when the field is not
// there or is not a number.
func numberIn(fields map[string]json.RawMessage, name string) int {
	raw, there := fields[name]
	if !there {
		return 0
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err != nil || number < 1 {
		return 0
	}
	return int(number)
}

// applyTheMarks marks the step and the line with the result's label and
// returns the lines the result carries back to say what happened. A failed
// call marks nothing; a number the record does not have is refused in the
// record's own words.
func (running *run) applyTheMarks(ctx context.Context, asked marks, label string, failed bool) []string {
	var said []string
	if asked.step > 0 {
		said = append(said, running.markTheStep(ctx, asked.step, label, failed))
	}
	if asked.line > 0 {
		said = append(said, running.proveTheLine(ctx, asked.line, label, failed))
	}
	return said
}

// markTheStep marks one plan step done by the label.
func (running *run) markTheStep(ctx context.Context, step int, label string, failed bool) string {
	if failed {
		return fmt.Sprintf("step %d not marked: the call failed", step)
	}
	if err := running.keeper.Apply(ctx, record.Update{StepDone: &record.StepDone{Number: step, ResultID: label}}); err != nil {
		return fmt.Sprintf("step %d not marked: %s", step, err.Error())
	}
	return fmt.Sprintf("step %d done by %s", step, label)
}

// proveTheLine points one done line at the label, by writing the done list
// back with that line changed, the way the task tool's pin_result does.
func (running *run) proveTheLine(ctx context.Context, line int, label string, failed bool) string {
	if failed {
		return fmt.Sprintf("done line %d not proved: the call failed", line)
	}
	update, err := pinTheResultToItsLine(recordWrite{Line: wholeNumber(line), Result: label}, running.keeper.Record())
	if err == nil {
		err = running.keeper.Apply(ctx, update)
	}
	if err != nil {
		return fmt.Sprintf("done line %d not proved: %s", line, err.Error())
	}
	return fmt.Sprintf("done line %d proved by %s", line, label)
}

// withTheMarkFields adds the two fields to the file tools' specifications, so
// the model is told they exist where it reads what a tool takes.
func withTheMarkFields(specs []contract.ToolSpec) []contract.ToolSpec {
	told := make([]contract.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		if slices.Contains(theToolsThatCarryMarks, spec.Name) {
			spec.Fields = append(slices.Clone(spec.Fields), TheMarkFields...)
		}
		told = append(told, spec)
	}
	return told
}
