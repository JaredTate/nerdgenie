package testkit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// deltasPerReply is how many pieces the fake model breaks its text into. More
// than one is what matters: a harness that only works when the whole reply
// arrives at once is broken, and this is how a test finds out.
const deltasPerReply = 4

// Step is one turn of a script: what the request must carry, and what the model
// says back.
type Step struct {
	// Expect is the text that must appear somewhere in the request, whether in a
	// system block or in a message. A step whose expectation is missing fails the
	// call with an error naming it, which is how a test catches a harness that
	// dropped a correction.
	Expect []string
	// Text is what the model says.
	Text string
	// ToolCalls are the tools it asks for.
	ToolCalls []contract.ToolCall
	// Finish says why it stopped.
	Finish contract.FinishReason
	// Usage is the token count to report.
	Usage contract.Usage
	// CacheCreationTokens is how many input tokens the provider wrote into its
	// cache on this call. It is separate from Usage because contract.Usage does
	// not carry it: the harness adds it to the input count rather than reporting
	// it on its own. Only the Anthropic wire protocol reports it.
	CacheCreationTokens int
	// MidStreamError makes the stream go wrong after its first block, carrying
	// this text in the protocol's own error shape and then stopping. It is how a
	// test proves the harness reads an error that arrives after the reply has
	// already started.
	MidStreamError string
}

// Script is an ordered list of steps a fake model plays, one per call.
type Script struct {
	// Name is what the fake model calls itself.
	Name string
	// ContextLength is the window the fake model reports.
	ContextLength int
	// Steps are played in order, one per call.
	Steps []Step
}

// FakeModel plays a script, one step per call, and records every request it was
// given.
type FakeModel struct {
	guard    sync.Mutex
	script   Script
	played   int
	requests []contract.Request
}

// NewFakeModel returns a model that plays the script.
func NewFakeModel(script Script) *FakeModel {
	return &FakeModel{script: script}
}

// Name is the script's name.
func (model *FakeModel) Name() string {
	return model.script.Name
}

// ContextLength is the window the script says the model holds.
func (model *FakeModel) ContextLength() int {
	return model.script.ContextLength
}

// Requests is every request the model was given, in order.
func (model *FakeModel) Requests() []contract.Request {
	model.guard.Lock()
	defer model.guard.Unlock()
	copied := make([]contract.Request, len(model.requests))
	copy(copied, model.requests)
	return copied
}

// StepsLeft is how many steps of the script have not been played.
func (model *FakeModel) StepsLeft() int {
	model.guard.Lock()
	defer model.guard.Unlock()
	return len(model.script.Steps) - model.played
}

// Send plays the next step, after checking that the request carries everything
// the step said it must.
func (model *FakeModel) Send(ctx context.Context, request contract.Request, onDelta func(delta string)) (contract.Reply, error) {
	if err := ctx.Err(); err != nil {
		return contract.Reply{}, fmt.Errorf("the model call was given up on before it started: %w", err)
	}
	step, err := model.nextStep(request)
	if err != nil {
		return contract.Reply{}, err
	}

	if onDelta != nil {
		for _, delta := range splitIntoDeltas(step.Text, deltasPerReply) {
			if err := ctx.Err(); err != nil {
				return contract.Reply{}, fmt.Errorf("the model call was given up on part way through the reply: %w", err)
			}
			onDelta(delta)
		}
	}
	return contract.Reply{
		Text:      step.Text,
		ToolCalls: step.ToolCalls,
		Finish:    finishOf(step),
		Usage:     step.Usage,
	}, nil
}

// nextStep takes the next step off the script, refusing a request that lost
// something the step expected.
func (model *FakeModel) nextStep(request contract.Request) (Step, error) {
	model.guard.Lock()
	defer model.guard.Unlock()

	model.requests = append(model.requests, request)
	if model.played >= len(model.script.Steps) {
		return Step{}, fmt.Errorf("the script %q has %d steps and this is call %d, so add another step",
			model.script.Name, len(model.script.Steps), model.played+1)
	}
	step := model.script.Steps[model.played]

	whole := WholeRequestText(request)
	for _, wanted := range step.Expect {
		if !strings.Contains(whole, wanted) {
			return Step{}, fmt.Errorf("step %d of the script %q expects the request to carry %q, and it does not, so the harness lost it",
				model.played+1, model.script.Name, wanted)
		}
	}
	model.played++
	return step, nil
}

// WholeRequestText is everything the model would read on one call, joined into
// one string: the system blocks, the messages, and the tool results. A test uses
// it to assert that something reached the model at all.
func WholeRequestText(request contract.Request) string {
	pieces := []string{}
	for _, block := range request.SystemBlocks {
		pieces = append(pieces, block.Text)
	}
	for _, message := range request.Messages {
		pieces = append(pieces, message.Text)
		for _, call := range message.ToolCalls {
			pieces = append(pieces, call.Name, string(call.Input))
		}
		for _, result := range message.ToolResults {
			pieces = append(pieces, result.CallID, result.Text)
		}
	}
	for _, spec := range request.Tools {
		pieces = append(pieces, spec.Name, spec.Description)
	}
	return strings.Join(pieces, "\n")
}

// splitIntoDeltas cuts text into roughly equal pieces, so that a test sees a
// reply arrive the way a real stream sends it.
func splitIntoDeltas(text string, pieces int) []string {
	if text == "" {
		return nil
	}
	if pieces < 1 {
		pieces = 1
	}
	runes := []rune(text)
	size := (len(runes) + pieces - 1) / pieces
	if size < 1 {
		size = 1
	}
	deltas := []string{}
	for at := 0; at < len(runes); at += size {
		end := min(at+size, len(runes))
		deltas = append(deltas, string(runes[at:end]))
	}
	return deltas
}
