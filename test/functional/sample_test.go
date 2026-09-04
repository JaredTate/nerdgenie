// Package functional holds the whole-feature tests: a message goes in, the right
// reply comes out, and the right things happen on disk.
//
// This file is the template every later functional test follows. There is no
// agent loop yet, so this test wires the two fakes to each other directly. From
// wave 3 the same shape drives the real loop through the local socket the
// terminal uses, and the assertions stay where they are: what the user sent, and
// what the user got back.
package functional

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestAMessageThroughTheFakeChannelGetsAScriptedReplyInUnderASecond(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	channel := testkit.NewFakeChannel("terminal")
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "sample",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Expect: []string{"when did DigiByte launch"},
			Text:   "DigiByte launched on 10 January 2014.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 120, CachedInputTokens: 100, OutputTokens: 9},
		}},
	})

	inbound, err := channel.Receive(ctx)
	if err != nil {
		t.Fatalf("attaching to the channel failed: %v", err)
	}

	if err := channel.Push(contract.Inbound{ID: "1", Sender: "jared", Text: "when did DigiByte launch?"}); err != nil {
		t.Fatalf("pushing the message failed: %v", err)
	}

	// The bound is enforced while the exchange runs, not measured after it: a
	// model that hangs would otherwise hang every functional test copied from
	// this one, and the failure would arrive as a test timeout minutes later.
	if err := within(t, time.Second, func() error {
		return answerOneMessage(ctx, inbound, model, channel)
	}); err != nil {
		t.Fatalf("answering the message failed: %v", err)
	}

	sent := channel.Sent()
	if len(sent) != 1 {
		t.Fatalf("the channel carried %d replies, want 1", len(sent))
	}
	if sent[0] != "DigiByte launched on 10 January 2014." {
		t.Errorf("the reply was %q, want the one the script wrote", sent[0])
	}
}

func TestAScriptThatExpectsSomethingTheMessageLostFailsLoudly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	channel := testkit.NewFakeChannel("terminal")
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "sample",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Expect: []string{"no, lead with the date not the features"},
			Text:   "Rewriting with the date first.",
			Finish: contract.FinishEnd,
		}},
	})

	inbound, err := channel.Receive(ctx)
	if err != nil {
		t.Fatalf("attaching to the channel failed: %v", err)
	}
	if err := channel.Push(contract.Inbound{ID: "1", Sender: "jared", Text: "what is the date?"}); err != nil {
		t.Fatalf("pushing the message failed: %v", err)
	}

	err = within(t, time.Second, func() error {
		return answerOneMessage(ctx, inbound, model, channel)
	})
	if err == nil {
		t.Fatal("a message that lost the correction was answered anyway, want an error naming what went missing")
	}
	if len(channel.Sent()) != 0 {
		t.Errorf("a reply went out after the model refused: %v", channel.Sent())
	}
}

// within runs the work and gives back what it returned, failing the test at once
// when the work is still going after the bound. Every functional test is written
// this way, so that a step which hangs fails in a second with a message rather
// than in ten minutes with a test timeout.
func within(t *testing.T, bound time.Duration, work func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- work() }()
	select {
	case err := <-done:
		return err
	case <-time.After(bound):
		t.Fatalf("the exchange was still running after %s, and the whole of it must take less than that", bound)
		return nil
	}
}

// answerOneMessage is the smallest possible stand-in for the agent loop: take one
// message off the channel, ask the model, and send the reply back. Wave 3
// replaces it with internal/loop, and this test keeps its assertions.
func answerOneMessage(ctx context.Context, inbound <-chan contract.Inbound, model contract.Model, channel contract.Channel) error {
	var message contract.Inbound
	select {
	case message = <-inbound:
	case <-ctx.Done():
		return ctx.Err()
	}

	reply, err := model.Send(ctx, contract.Request{
		SystemBlocks: []contract.SystemBlock{{
			Name:     "harness rules and persona",
			Text:     "You are the reasoning engine inside Nerd Genie.",
			Boundary: contract.CacheBoundaryA,
		}},
		Messages: []contract.Message{{Role: contract.RoleUser, Text: message.Text}},
	}, nil)
	if err != nil {
		return err
	}
	return channel.Send(ctx, reply.Text)
}
