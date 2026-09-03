package loop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/repair"
)

// ThreeOptions is how every error message to the model ends, which is rule 6 of
// design section 3.
const ThreeOptions = "You can answer the user, ask one question, or try different arguments."

// runTheCalls runs the tools one reply asked for, in order, and stops part way
// through when something ends the task.
func (running *run) runTheCalls(ctx context.Context, found repair.Result) (Outcome, bool, error) {
	running.remember(contract.Message{Role: contract.RoleAssistant, Text: found.Text, ToolCalls: found.Calls})
	if err := running.startTheRecord(ctx); err != nil {
		return Outcome{}, false, err
	}
	results := []contract.ToolResult{}
	var ending *Outcome
	for _, call := range found.Calls {
		result, ended, err := running.oneCall(ctx, call)
		if err != nil {
			return Outcome{}, false, err
		}
		results = append(results, result)
		if ended != nil {
			ending = ended
			break
		}
	}
	running.remember(contract.Message{Role: contract.RoleUser, ToolResults: results})
	if err := running.writeSituation(ctx); err != nil {
		return Outcome{}, false, err
	}
	if ending != nil {
		return *ending, false, nil
	}
	return running.readDelivered(ctx)
}

// oneCall puts one tool call through the guard, the permission function, and
// the tool, and writes what came back into the record. It returns an outcome
// when the call ended the task.
func (running *run) oneCall(ctx context.Context, call contract.ToolCall) (contract.ToolResult, *Outcome, error) {
	if err := running.theLoop.logEvent(ctx, running.taskID(), contract.EventToolCall, call); err != nil {
		return contract.ToolResult{}, nil, err
	}
	refusal, hadEnough := running.detectorRefuses(call)
	if hadEnough {
		ended, err := running.stopHere(ctx, "the model asked for the same thing over and over")
		return refusedResult(call, refusal), &ended, err
	}
	if refusal != "" {
		return refusedResult(call, refusal), nil, nil
	}
	allowed, denial, err := running.permit(ctx, call)
	if err != nil {
		return contract.ToolResult{}, nil, err
	}
	if !allowed {
		return running.afterADenial(ctx, call, denial)
	}
	return running.runAndRecord(ctx, call)
}

// afterADenial turns a refused call into a result the model can act on, and
// ends the task when nobody was there to answer.
func (running *run) afterADenial(ctx context.Context, call contract.ToolCall, denial denial) (contract.ToolResult, *Outcome, error) {
	if denial.stopsTheTask {
		ended, err := running.stopHere(ctx, "this needed a yes from the user and the run is unattended: "+denial.reason)
		return refusedResult(call, denial.reason), &ended, err
	}
	return refusedResult(call, denial.reason), nil, nil
}

// runAndRecord runs one tool, writes its result into the record and the log,
// and checks the result against the stop list.
func (running *run) runAndRecord(ctx context.Context, call contract.ToolCall) (contract.ToolResult, *Outcome, error) {
	text, failed := running.runOneTool(ctx, call)
	if _, err := running.keeper.AddResult(ctx, summaryOfResult(call.Name, text), text); err != nil {
		return contract.ToolResult{}, nil, fmt.Errorf("cannot write the result of %s into the record: %w", call.Name, err)
	}
	running.noteWhatTheResultShows(call, text, failed)
	result := contract.ToolResult{CallID: call.ID, Text: text, Failed: failed}
	if failed {
		result.Text = text + "\n" + ThreeOptions
	}
	// The record write is the harness's own words coming back, and a stop list
	// written into the record would otherwise fire on itself the moment the
	// model wrote it. Only what a tool found in the world is checked.
	if call.Name == contract.ToolTask {
		return result, nil, nil
	}
	if line := running.stopLineFiredBy(call.Name, text); line != "" {
		ended, err := running.stopHere(ctx, line)
		return result, &ended, err
	}
	return result, nil, nil
}

// runOneTool finds the tool and runs it under the tool time limit, and says
// whether what came back is a result or an error.
func (running *run) runOneTool(ctx context.Context, call contract.ToolCall) (string, bool) {
	if call.Name == contract.ToolTask {
		return running.applyRecordWrite(ctx, call)
	}
	tool, found := running.theLoop.options.Tools.Lookup(call.Name)
	if !found {
		return fmt.Sprintf("there is no tool called %q on this agent, so use one of the tools you were given", call.Name), true
	}
	output, err := running.underTheTimeLimit(ctx, tool, call)
	if err != nil {
		return err.Error(), true
	}
	if output.SpillPath != "" {
		return output.Text + "\nThe rest of this result is in " + output.SpillPath + ", which you can read.", false
	}
	return output.Text, false
}

// underTheTimeLimit runs one tool and gives up on it when its time is up. The
// waiting is measured on the harness's own clock, so no test waits on a real
// one.
func (running *run) underTheTimeLimit(ctx context.Context, tool contract.Tool, call contract.ToolCall) (contract.ToolOutput, error) {
	limit := running.theLoop.options.Caps.TimePerTool
	working, giveUp := context.WithCancel(ctx)
	defer giveUp()

	type answer struct {
		output contract.ToolOutput
		err    error
	}
	answers := make(chan answer, 1)
	go func() {
		output, err := tool.Run(working, call.Input)
		answers <- answer{output: output, err: err}
	}()
	tooLong := make(chan struct{})
	go func() {
		if running.theLoop.options.Clock.Sleep(working, limit) == nil {
			close(tooLong)
		}
	}()

	select {
	case got := <-answers:
		return got.output, got.err
	case <-tooLong:
		return contract.ToolOutput{}, fmt.Errorf("the tool %s ran for %s and its time was up, so run less at once or raise the limit",
			call.Name, limit)
	}
}

// applyRecordWrite is the model's half of the record. The task tool of wave 2
// needs the keeper of the task that is running, and the loop is the only thing
// that holds one, so the loop applies the write itself through record's rules
// and hands the whole update back as the result, where "read r2" can fetch it.
func (running *run) applyRecordWrite(ctx context.Context, call contract.ToolCall) (string, bool) {
	update, err := readRecordUpdate(call.Input)
	if err != nil {
		return err.Error(), true
	}
	if err := running.keeper.Apply(ctx, update); err != nil {
		return "the record refused that change: " + err.Error(), true
	}
	if update.Failure != nil {
		running.hadFailure = true
	}
	return string(call.Input), false
}

// summaryOfResult is the one line a result keeps in the record. The record cuts
// it to the length a line allows, and the whole text stays in the log.
func summaryOfResult(name string, text string) string {
	if name == contract.ToolTask {
		return "updated the record: " + fieldsWritten(text)
	}
	if first := firstLine(text); first != "" {
		return name + ": " + first
	}
	return name + ": nothing came back"
}

// refusedResult is what the model gets back for a call that never ran.
func refusedResult(call contract.ToolCall, reason string) contract.ToolResult {
	return contract.ToolResult{CallID: call.ID, Text: reason + "\n" + ThreeOptions, Failed: true}
}

// startTheRecord creates the task record on the first tool call, which is where
// the design says a record begins.
func (running *run) startTheRecord(ctx context.Context) error {
	if running.keeper != nil {
		return nil
	}
	taskID, err := running.theLoop.nextTaskNumber(ctx)
	if err != nil {
		return err
	}
	keeper, err := record.New(ctx, running.theLoop.options.Store, record.Start{
		Kind:        contract.RecordTask,
		ID:          taskID,
		Origin:      running.origin(),
		Ask:         running.task.Message.Text,
		RoundsLeft:  running.roundsLeft(),
		MinutesLeft: running.minutesLeft(),
	})
	if err != nil {
		return fmt.Errorf("cannot start the record of task %s: %w", taskID, err)
	}
	running.keeper = keeper
	running.theLoop.nowRunning(taskID)
	return nil
}

// origin is the channel the ask came in on, which the record's header carries.
func (running *run) origin() string {
	if running.task.Message.Channel != "" {
		return running.task.Message.Channel
	}
	return nameOf(running.channel)
}

// asJSON writes a value for the event log.
func asJSON(body any) (json.RawMessage, error) {
	written, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cannot write this event as JSON for the log: %w", err)
	}
	return written, nil
}
