package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

// RecordedContextLength is the window the replayed model reports. It is large
// on purpose: the replies are already written down, so nothing is gained by
// making the working context shrink, and a window big enough for any record
// keeps the builder from dropping messages the recording had.
const RecordedContextLength = 200000

// recordedModel is the model as the event log remembers it. It answers each
// call with the next round of the recording and, once those have run out, with
// the last thing the task said to the user, which ends the turn.
type recordedModel struct {
	// guard keeps one call at a time, because the loop may be driven from more
	// than one place.
	guard sync.Mutex
	// recording is the run being played back.
	recording Recording
	// at is how many rounds have been played.
	at int
	// calls counts the calls handed out, so that each one gets its own
	// identifier the way a provider gives one.
	calls int
	// into is the loop the user's recorded mid-task messages are delivered to.
	into *loop.Loop
}

// newRecordedModel returns the model of one recording.
func newRecordedModel(recording Recording) *recordedModel {
	return &recordedModel{recording: recording}
}

// deliverInto tells the model which loop the user's recorded mid-task messages
// go to. It is set after the loop is built, because the loop is built from the
// model and so cannot be handed to it any earlier.
func (model *recordedModel) deliverInto(theLoop *loop.Loop) {
	model.guard.Lock()
	defer model.guard.Unlock()
	model.into = theLoop
}

// Name says which task's recording is answering.
func (model *recordedModel) Name() string {
	return "the recording of task " + model.recording.TaskID
}

// ContextLength is the window the replayed model reports.
func (model *recordedModel) ContextLength() int {
	return RecordedContextLength
}

// Send hands back the next recorded reply. Nothing about the request changes
// what comes back, because a replay asks the model nothing it has not already
// been asked.
func (model *recordedModel) Send(ctx context.Context, _ contract.Request, onDelta func(delta string)) (contract.Reply, error) {
	if err := ctx.Err(); err != nil {
		return contract.Reply{}, fmt.Errorf("the replay was given up on before the model call: %w", err)
	}
	reply, err := model.nextReply()
	if err != nil {
		return contract.Reply{}, err
	}
	if onDelta != nil && reply.Text != "" {
		onDelta(reply.Text)
	}
	return reply, nil
}

// nextReply plays one round, or answers when the recorded rounds have run out.
func (model *recordedModel) nextReply() (contract.Reply, error) {
	model.guard.Lock()
	defer model.guard.Unlock()

	if model.at >= len(model.recording.Rounds) {
		return contract.Reply{Text: model.recording.Answer, Finish: contract.FinishEnd}, nil
	}
	round := model.recording.Rounds[model.at]
	model.at++
	if err := model.deliver(round); err != nil {
		return contract.Reply{}, err
	}
	return contract.Reply{
		Text:      round.Orient,
		ToolCalls: model.callsOf(round),
		Finish:    contract.FinishToolCalls,
	}, nil
}

// deliver hands the loop the messages the user sent while this round's tools
// were running, so that a recorded correction reaches the record again at the
// point in the run where it first arrived. The caller holds the lock.
func (model *recordedModel) deliver(round Round) error {
	if model.into == nil {
		return nil
	}
	for _, message := range round.Delivered {
		if err := model.into.Deliver(message); err != nil {
			return fmt.Errorf("cannot hand the replayed task what the user said while it ran: %w", err)
		}
	}
	return nil
}

// callsOf turns one round's recorded calls back into calls the loop can run,
// each with an identifier of its own the way a provider gives one. The caller
// holds the lock.
func (model *recordedModel) callsOf(round Round) []contract.ToolCall {
	made := make([]contract.ToolCall, 0, len(round.Calls))
	for _, call := range round.Calls {
		model.calls++
		made = append(made, contract.ToolCall{
			ID:    "replayed-" + strconv.Itoa(model.calls),
			Name:  call.Name,
			Input: inputOf(call),
		})
	}
	return made
}

// inputOf is the arguments the model wrote, and an empty object when the
// recording has none, because a call with no arguments at all is refused before
// it reaches the tool.
func inputOf(call Call) json.RawMessage {
	if len(call.Input) == 0 {
		return json.RawMessage(`{}`)
	}
	return call.Input
}
