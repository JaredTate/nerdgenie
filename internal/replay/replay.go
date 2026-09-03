// The idea that a run worth keeping is a run you can play back against new code
// comes from ZeroClaw's recorded turn fixtures at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/agent/turn/mod.rs, where a turn is
// driven by a scripted model so that the engine is the only thing under test.
// The Go here is written fresh, and the script is the event log rather than a
// file somebody wrote by hand.

package replay

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/record"
)

// Options is everything a replay needs. Nothing here reaches the world: the
// model and the tools answer out of the log, the log written into is a
// throwaway, and the sandbox is left out unless a caller wants the done-check's
// commands run for real.
type Options struct {
	// From is the event log the recording is read out of.
	From contract.Store
	// Into is the event log the replayed run writes into, which must be a
	// throwaway rather than the agent's own, because a replay is not a task the
	// user asked for.
	Into contract.Store
	// Context builds the working context, and is the same builder the running
	// program uses, so that a replay tests the prompt as well as the loop.
	Context loop.ContextBuilder
	// Permission is the rulebook as it stands now, which is the thing a replay
	// most often finds has changed.
	Permission contract.Permission
	// Clock is where the replayed run reads the time.
	Clock contract.Clock
	// Caps are the limits from the configuration, and the defaults when zero.
	Caps contract.Caps
	// Sandbox runs a command a done line names in backticks, and is nil in a
	// plain replay, which then leaves such a line unchecked rather than running
	// anything on the machine.
	Sandbox contract.Sandbox
}

// Result is what one replay found.
type Result struct {
	// TaskID is the task that was replayed.
	TaskID string
	// Passed says the replay ended where the recording ended: the same record,
	// down to the line, and the same answer from the done-check.
	Passed bool
	// Difference names the first step where the two runs parted company, and is
	// empty when they did not.
	Difference string
	// Status is where the replayed task ended.
	Status contract.RecordStatus
	// WasStatus is where the recorded task ended.
	WasStatus contract.RecordStatus
	// DoneCheck is what the done-check says about the replayed record, and is
	// empty when it passes.
	DoneCheck string
	// WasDoneCheck is what it said about the recorded one.
	WasDoneCheck string
	// Rounds is how many recorded rounds the replay played.
	Rounds int
	// Report is the whole of the above in plain words, which is what the replay
	// subcommand prints.
	Report string
}

// Run reads one task's recording out of the log and replays it against the code
// as it stands now.
func Run(ctx context.Context, options Options, taskID string) (Result, error) {
	if options.From == nil {
		return Result{}, errors.New("cannot replay a task without the event log to read the recording out of")
	}
	if err := options.check(); err != nil {
		return Result{}, err
	}
	recording, err := Read(ctx, options.From, taskID)
	if err != nil {
		return Result{}, err
	}
	return RunRecording(ctx, options, recording)
}

// RunRecording replays a recording that has already been read, which is what a
// generated test does with the fixture beside it.
func RunRecording(ctx context.Context, options Options, recording Recording) (Result, error) {
	if err := options.check(); err != nil {
		return Result{}, err
	}
	model := newRecordedModel(recording)
	tools := newRecordedTools(recording)
	theLoop, err := loop.New(loop.Options{
		Model: model, Tools: tools, Permission: options.Permission, Store: options.Into,
		Clock: options.Clock, Context: options.Context, Caps: options.Caps, Sandbox: options.Sandbox,
	})
	if err != nil {
		return Result{}, fmt.Errorf("cannot build the loop that replays task %s: %w", recording.TaskID, err)
	}
	model.deliverInto(theLoop)
	outcome, err := theLoop.Run(ctx, loop.Task{
		Message: contract.Inbound{ID: "replay-" + recording.TaskID, Text: recording.Ask, Channel: recording.Origin},
		Channel: quietChannel{name: recording.Origin},
	})
	if err != nil {
		return Result{}, fmt.Errorf("the replay of task %s did not finish: %w", recording.TaskID, err)
	}
	return options.verdict(ctx, recording, outcome, tools)
}

// check says which of the options a replay cannot run without.
func (options Options) check() error {
	for _, needed := range []struct {
		name  string
		there bool
	}{
		{"a throwaway event log to write the replayed run into", options.Into != nil},
		{"a working-context builder", options.Context != nil},
		{"the permission function as it stands now", options.Permission != nil},
		{"a clock to read the time from", options.Clock != nil},
	} {
		if !needed.there {
			return fmt.Errorf("a replay needs %s, so pass one in its options", needed.name)
		}
	}
	return nil
}

// verdict compares the replayed run with the recording and writes the report.
func (options Options) verdict(ctx context.Context, recording Recording,
	outcome loop.Outcome, tools *recordedTools) (Result, error) {
	result := Result{
		TaskID:       recording.TaskID,
		Status:       outcome.Status,
		WasStatus:    recording.Final.Header.Status,
		WasDoneCheck: doneCheckSays(recording.Final),
		Rounds:       len(recording.Rounds),
		Difference:   tools.firstDifference(),
	}
	if outcome.TaskID == "" {
		result.Difference = "the replayed task made no record at all, and the recording has one"
		result.Report = writeReport(result)
		return result, nil
	}
	keeper, err := record.Load(ctx, options.Into, contract.RecordTask, outcome.TaskID)
	if err != nil {
		return Result{}, fmt.Errorf("cannot read back the record the replay of task %s wrote: %w", recording.TaskID, err)
	}
	replayed := keeper.Record()
	result.DoneCheck = doneCheckSays(replayed)
	if result.Difference == "" {
		result.Difference = differenceBetween(recording.Final, replayed)
	}
	result.Passed = result.Difference == "" && result.DoneCheck == result.WasDoneCheck
	result.Report = writeReport(result)
	return result, nil
}
