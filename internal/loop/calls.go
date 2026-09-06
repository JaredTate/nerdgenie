package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/repair"
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
	running.rewindIfDue(ctx)
	running.sayTheProbeLine()
	if err := running.writeSituation(ctx); err != nil {
		return Outcome{}, false, err
	}
	if ending != nil {
		return *ending, false, nil
	}
	outcome, more, err := running.readDelivered(ctx)
	if err != nil || !more {
		return outcome, more, err
	}
	if running.jobToHandTo != "" && !running.theLoop.stopAsked() {
		handed, err := running.handTheWorkToTheJob(ctx, results)
		return handed, false, err
	}
	return outcome, more, nil
}

// oneCall puts one tool call through the guard, the permission function, and
// the tool, and writes what came back into the record. It returns an outcome
// when the call ended the task.
func (running *run) oneCall(ctx context.Context, call contract.ToolCall) (contract.ToolResult, *Outcome, error) {
	if err := running.theLoop.logEvent(ctx, running.taskID(), contract.EventToolCall, call); err != nil {
		return contract.ToolResult{}, nil, err
	}
	if running.rewindDue {
		return refusedResult(call, "The conversation is being cleared after this reply, so this call was not run."), nil, nil
	}
	refusal, hadEnough := running.detectorRefuses(call)
	if hadEnough && running.rewindsUsed < RewindsAllowed {
		running.rewindsUsed++
		running.rewindDue = true
		running.stalledOn = call.Name + " " + whatTheCallSays(call)
		return refusedResult(call, refusal), nil, nil
	}
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
	running.noteToolLine(toolLineFor(call, "", false))
	text, failed := running.runOneTool(ctx, call)
	running.noteTheResult(text)
	summary := summaryOfResult(call.Name, text, failed)
	label, err := running.keeper.AddResult(ctx, summary, text)
	if err != nil {
		return contract.ToolResult{}, nil, fmt.Errorf("cannot write the result of %s into the record: %w", call.Name, err)
	}
	running.noteToolLine(toolLineFor(call, label+" "+summary, failed))
	running.noteWhatTheResultShows(call, text, failed)
	running.writeWhatTheTestsShow(ctx, call, text, label)
	running.countTheProbe(call)
	result := contract.ToolResult{CallID: call.ID, Label: label, Text: text, Failed: failed}
	if failed {
		result.Text = text + "\n" + ThreeOptions
	}
	if line := running.theStopTheModelAskedFor(); line != "" {
		ended, err := running.stopHere(ctx, line)
		return result, &ended, err
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
	if answer, refused, mine := running.theLoopsOwnOperation(ctx, call); mine {
		return answer, refused
	}
	tool, found := running.tools().Lookup(call.Name)
	if !found && call.Name == contract.ToolTask {
		return running.applyRecordWrite(ctx, call)
	}
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
	working, giveUp := running.timeToRunOneTool(ctx)
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
		if got.err != nil && theDeadlineHasPassed(working) {
			return contract.ToolOutput{}, theDeadlineStoppedIt(call)
		}
		return got.output, got.err
	case <-tooLong:
		return contract.ToolOutput{}, fmt.Errorf("the tool %s ran for %s and its time was up, so run less at once or raise the limit",
			call.Name, limit)
	case <-working.Done():
		if theDeadlineHasPassed(working) {
			return contract.ToolOutput{}, theDeadlineStoppedIt(call)
		}
		return contract.ToolOutput{}, fmt.Errorf("the tool %s was stopped before it came back, so ask for it again when the task carries on",
			call.Name)
	}
}

// timeToRunOneTool is the context one tool call runs under: the task's own, with
// the deadline the reliability guard gives this call on top of it when there is
// one. Both are let go when the call is over.
func (running *run) timeToRunOneTool(ctx context.Context) (context.Context, context.CancelFunc) {
	if running.theLoop.options.ToolDeadline == nil {
		return context.WithCancel(ctx)
	}
	byThen, deadlineDone := running.theLoop.options.ToolDeadline(ctx)
	working, giveUp := context.WithCancel(byThen)
	return working, func() {
		giveUp()
		deadlineDone()
	}
}

// theDeadlineHasPassed says whether this call was stopped because the deadline
// it was given ran out, rather than because the task itself was stopped.
func theDeadlineHasPassed(working context.Context) bool {
	return errors.Is(working.Err(), context.DeadlineExceeded)
}

// theDeadlineStoppedIt is what the model is told about a call the deadline cut
// short, in words it can act on rather than the Go error.
func theDeadlineStoppedIt(call contract.ToolCall) error {
	return fmt.Errorf("the tool %s did not finish before the deadline this call was given, so ask for less at once or try it again",
		call.Name)
}

// applyRecordWrite is the model's half of the record. The task tool of wave 2
// needs the keeper of the task that is running, and the loop is the only thing
// that holds one, so the loop applies the write itself through record's rules
// and hands the whole update back as the result, where "read r2" can fetch it.
func (running *run) applyRecordWrite(ctx context.Context, call contract.ToolCall) (string, bool) {
	update, err := readRecordUpdate(call.Input, running.keeper.Record())
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
func summaryOfResult(name string, text string, failed bool) string {
	// A call the tool or the record refused is never written down as a change.
	// It was said as "updated the record: one change" once, and the model went
	// on believing a write had happened that the record had refused.
	if failed {
		return name + " was refused: " + firstLine(text)
	}
	if name == contract.ToolTask {
		return "updated the record: " + fieldsWritten(text)
	}
	// A test run's first line is its exit code, which says nothing a model can
	// use, so its line is the runner's own summary: the counts and the names.
	if name == contract.ToolShell {
		if state, found := testStateIn(text); found {
			return state.line()
		}
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
	taskID := running.number
	left := running.budgetLeft()
	keeper, err := record.New(ctx, running.theLoop.options.Store, record.Start{
		Kind:          contract.RecordTask,
		ID:            taskID,
		Origin:        running.origin(),
		Ask:           running.task.Message.Text,
		RoundsLeft:    left.RoundsLeft,
		NoRoundBudget: left.NoRoundBudget,
		MinutesLeft:   left.MinutesLeft,
		NoTimeBudget:  left.NoTimeBudget,
	})
	if err != nil {
		return fmt.Errorf("cannot start the record of task %s: %w", taskID, err)
	}
	keeper.SaveOncePerRound()
	running.keeper = keeper
	running.theLoop.nowRunning(taskID)
	return nil
}

// saveTheRound writes one checkpoint for the round that has just called the
// model and is about to run its tools. A checkpoint holds the whole record
// printed, and saving one for every change to it wrote the record into the log
// four times per tool call: the budget, the cost, each result, and the
// situation. One model call is one round, and a round is where a replay reads
// the boundary, so a round is one checkpoint, taken before the round's tools run
// and carrying both the budget that round is spending and everything the round
// before it left behind.
func (running *run) saveTheRound(ctx context.Context) error {
	if running.keeper == nil {
		return nil
	}
	if err := running.keeper.SaveTheRound(ctx); err != nil {
		return fmt.Errorf("cannot save the checkpoint of this round of task %s: %w", running.keeper.ID(), err)
	}
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
