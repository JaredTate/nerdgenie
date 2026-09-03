package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/JaredTate/coeus/internal/channel"
	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
)

// The bounds this file works inside, because everything is bounded. The working
// context builder sizes the prompt to the model on its own; these two caps stop
// the conversation this file holds between turns from growing without end.
const (
	// maxConversationMessages is how many messages of the conversation are kept,
	// newest kept.
	maxConversationMessages = 20
	// maxConversationBytes is how much text of the conversation is kept, newest
	// kept.
	maxConversationBytes = 32 << 10
)

// conversation is the messages so far, capped, newest kept. It is the only state
// this file holds between turns, and the turn loop replaces it with the task
// record.
type conversation struct {
	guard    sync.Mutex
	messages []contract.Message
}

// add puts one message on the end and drops the oldest ones once either cap is
// passed.
func (talk *conversation) add(message contract.Message) {
	talk.guard.Lock()
	defer talk.guard.Unlock()
	talk.messages = append(talk.messages, message)

	held := 0
	for _, one := range talk.messages {
		held += len(one.Text)
	}
	for len(talk.messages) > maxConversationMessages || (held > maxConversationBytes && len(talk.messages) > 1) {
		held -= len(talk.messages[0].Text)
		talk.messages = talk.messages[1:]
	}
}

// soFar is a copy of the conversation as it stands, for one request.
func (talk *conversation) soFar() []contract.Message {
	talk.guard.Lock()
	defer talk.guard.Unlock()
	copied := make([]contract.Message, len(talk.messages))
	copy(copied, talk.messages)
	return copied
}

// firstTurn answers one message with one model call and no tools. It is the
// stand-in for internal/loop while that package is being written: serve.go hands
// its StartTask to the router, and the day the loop lands the orchestrator swaps
// that one line for loop.New(...).StartTask and deletes this file.
//
// The prompt itself is not this file's work. internal/context builds it, so the
// person talking to the agent today reads the same instruction text, the same
// persona, and the same layer order the turn loop will send tomorrow. What is
// left out is everything that needs the loop: there is no task record yet, no
// job, no pinned evidence, no memory hint, and no tools.
type firstTurn struct {
	// settings are the configuration the program loaded.
	settings contract.Config
	// builder makes the working context, sized to the model.
	builder *workingcontext.Builder
	// model is what answers, already wrapped in retries and the fallback chain.
	model contract.Model
	// stream is the feed every attached screen reads.
	stream *channel.Stream
	// eventLog is where the message and the reply are written down.
	eventLog contract.Store
	// talk is the conversation so far.
	talk *conversation
	// channels finds the channel a message came through, which is where its
	// reply goes.
	channels func(name string) (contract.Channel, bool)
}

// messageEvent is the body of a message event and of a reply event: what was
// said and who said it. The field names are the ones internal/memory already
// reads, so that a correction the user typed is captured with no model call.
type messageEvent struct {
	// Text is what was said.
	Text string `json:"text"`
	// Role says who said it.
	Role string `json:"role"`
}

// StartTask answers one message: write it down, ask the model, and send the
// reply back the way the message came. Nothing is run and no tool is offered.
func (turn *firstTurn) StartTask(ctx context.Context, message contract.Inbound) error {
	where, found := turn.channels(message.Channel)
	if !found {
		return fmt.Errorf("the message came through the channel %q, which is not one this agent is running, so the reply would have nowhere to go", message.Channel)
	}

	turn.writeDown(ctx, contract.EventMessage, contract.RoleUser, message.Text)
	turn.talk.add(contract.Message{Role: contract.RoleUser, Text: message.Text})

	reply, err := turn.ask(ctx)
	if err != nil {
		turn.report(contract.StateIdle, nil)
		return errors.Join(
			fmt.Errorf("the model could not answer the message, and the user has been told: %w", err),
			where.Send(ctx, "the model could not be reached, so there is no answer yet: "+err.Error()))
	}

	turn.talk.add(contract.Message{Role: contract.RoleAssistant, Text: reply.Text})
	turn.writeDown(ctx, contract.EventReply, contract.RoleAssistant, reply.Text)
	turn.report(contract.StateIdle, &reply.Usage)
	return where.Send(ctx, reply.Text)
}

// ask builds the working context and makes the one model call, inside the turn's
// own time budget.
//
// It asks for no deltas, and that is a decision rather than an omission. Both
// provider.WithRetries and provider.NewChain hold every piece of text back until
// the attempt has succeeded, so a delta today is not live text: the whole answer
// arrives in one lump a moment before the reply. Worse, the two travel to a
// screen by different paths, deltas on the event stream and the reply straight
// out of the channel, and nothing orders those two paths against each other. On
// this machine the reply wins every time, and the terminal screen, which draws a
// delta that arrives after a reply as the start of a new answer, then shows the
// same answer twice. Live text is worth having, and it becomes possible the day
// a channel's reply rides the same event stream its deltas do; until then a
// delta envelope costs the reader a duplicated answer and buys nothing.
func (turn *firstTurn) ask(ctx context.Context) (contract.Reply, error) {
	bounded, giveUp := context.WithTimeout(ctx, turn.settings.Caps.TimePerTurn)
	defer giveUp()

	request, err := turn.request(bounded)
	if err != nil {
		return contract.Reply{}, err
	}
	turn.report(contract.StateThinking, nil)
	return turn.model.Send(bounded, request, nil)
}

// request is the working context for this call, built by internal/context and
// sized to the model that is about to be asked. There is no record, no job, no
// pinned evidence, no memory hint, and no tool, because every one of those
// belongs to the turn loop; what is left is the instruction text, the persona,
// and the conversation so far.
func (turn *firstTurn) request(ctx context.Context) (contract.Request, error) {
	request, err := turn.builder.Build(ctx, workingcontext.BuildInput{
		ContextLength: turn.model.ContextLength(),
		Messages:      turn.talk.soFar(),
	})
	if err != nil {
		return contract.Request{}, err
	}
	// The tools are switched off rather than merely absent, so a model that
	// would otherwise invent a tool call is told there are none to call.
	request.ToolsOff = true
	return request, nil
}

// writeDown puts one message or reply in the event log. A log that refuses the
// write is not allowed to stop the answer, so the trouble rides on the stream as
// an error a screen can show.
func (turn *firstTurn) writeDown(ctx context.Context, kind contract.EventKind, role contract.Role, text string) {
	body, err := json.Marshal(messageEvent{Text: text, Role: string(role)})
	if err != nil {
		return
	}
	if _, err := turn.eventLog.Append(ctx, contract.Event{Kind: kind, Body: body}); err != nil {
		turn.publish(contract.SocketEnvelope{
			Type:   contract.SocketError,
			Text:   "the event log would not take this turn down, so nothing about it can be replayed later",
			Reason: err.Error(),
		})
	}
}

// report tells every screen what the agent is doing and, when a call has just
// finished, what it cost.
func (turn *firstTurn) report(state string, spent *contract.Usage) {
	fields := map[string]string{
		contract.StatusFieldState: state,
		contract.StatusFieldModel: turn.model.Name(),
	}
	if spent != nil {
		fields[contract.StatusFieldTokensIn] = fmt.Sprintf("%d", spent.InputTokens)
		fields[contract.StatusFieldTokensOut] = fmt.Sprintf("%d", spent.OutputTokens)
	}
	turn.publish(contract.SocketEnvelope{Type: contract.SocketStatus, Fields: fields})
}

// publish puts one envelope on the event stream. Nobody may be listening, and
// that is not a failure: the reply is in the event log either way and the turn
// must not stop for want of an audience.
func (turn *firstTurn) publish(envelope contract.SocketEnvelope) {
	_ = turn.stream.Publish(envelope)
}
