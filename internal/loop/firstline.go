package loop

import (
	"context"
	"regexp"
	"strconv"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The first line of a reply says where the work stands, and a mark on it
// marks the record: "step 2 done: r1" marks plan step 2 with r1, and "line 1
// done: r1" marks the first done line the same way, with no tool call and no
// round spent on it. In the fifth game build's last three tasks a fifth of
// all rounds did nothing but mark a step or pin a line through the task tool.

// TheMarkHint is what the model reads once, after a round it spent on nothing
// but a step_done or a pin_result call: the nightly set's second run showed
// the rule in the instructions was not enough on its own.
const TheMarkHint = "That round only marked the record. Next time put the mark on your first line, as \"step 3 done: r41\" or \"line 2 done: r41\", and spend the round on the work."

// TheMarkNeedsAResult is what the model reads after a mark on its first line
// that names no result, because a step is done by the result that proves it.
const TheMarkNeedsAResult = "The mark on your first line names no result, so write it as \"step 2 done: r7\" or \"line 1 done: r7\", naming the result that proves it."

// The two marks a first line may carry: the number of the step or the line,
// and the result label right after it when there is one.
var (
	theStepMark = regexp.MustCompile(`(?i)\bstep (\d+) done[:,]?\s*(?:with |by |proven by )?(r\d+)?`)
	theLineMark = regexp.MustCompile(`(?i)\bline (\d+) done[:,]?\s*(?:with |by |proven by )?(r\d+)?`)
)

// markFromTheFirstLine reads the marks on the first line of a reply and makes
// them on the record. A mark the record refuses, or one with no result, marks
// nothing, and the model reads why on its next call.
func (running *run) markFromTheFirstLine(ctx context.Context, line string) {
	if running.keeper == nil || line == "" {
		return
	}
	for _, mark := range theStepMark.FindAllStringSubmatch(line, -1) {
		number, _ := strconv.Atoi(mark[1])
		if mark[2] == "" {
			running.remember(contract.Message{Role: contract.RoleUser, Text: TheMarkNeedsAResult})
			continue
		}
		if err := running.keeper.MarkPlanStep(ctx, number, mark[2]); err != nil {
			running.remember(contract.Message{Role: contract.RoleUser, Text: "The mark on your first line was not made: " + err.Error()})
		}
	}
	for _, mark := range theLineMark.FindAllStringSubmatch(line, -1) {
		number, _ := strconv.Atoi(mark[1])
		if mark[2] == "" {
			running.remember(contract.Message{Role: contract.RoleUser, Text: TheMarkNeedsAResult})
			continue
		}
		if err := running.markTheDoneLine(ctx, number, mark[2]); err != nil {
			running.remember(contract.Message{Role: contract.RoleUser, Text: "The mark on your first line was not made: " + err.Error()})
		}
	}
}

// markTheDoneLine marks one line of the done list with its result, through
// the record's own rules, the way the task tool's pin does.
func (running *run) markTheDoneLine(ctx context.Context, number int, resultID string) error {
	lines := append([]contract.DoneLine(nil), running.keeper.Record().Goal.DoneWhen...)
	if number < 1 || number > len(lines) {
		return record.ErrNoSuchResult
	}
	lines[number-1].Done = true
	lines[number-1].ResultID = resultID
	return running.keeper.Apply(ctx, record.Update{DoneWhen: lines})
}

// theMarkOperations are the task tool's operations a first line can carry.
var theMarkOperations = map[string]bool{"step_done": true, "pin_result": true}

// sayTheMarkHint puts the hint into the conversation after a round whose calls
// were all marks the first line could have carried, once for the task.
func (running *run) sayTheMarkHint(calls []contract.ToolCall) {
	if running.saidTheMarkHint || len(calls) == 0 {
		return
	}
	for _, call := range calls {
		if call.Name != contract.ToolTask || !theMarkOperations[fieldOfCall(call, "operation")] {
			return
		}
	}
	running.saidTheMarkHint = true
	running.remember(contract.Message{Role: contract.RoleUser, Text: TheMarkHint})
}
