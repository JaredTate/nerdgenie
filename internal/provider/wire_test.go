package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The tests in this file are the ones that used to need a stream written by
// hand, because the fake provider server did not yet send those shapes. It now
// sends all of them, so they are asked of the fake, which means the Anthropic
// provider and the OpenAI-compatible provider are both held to the same script.

// scriptFinishing is a one-step script that stops for the reason given, with the
// cache-creation count the Messages API reports separately.
func scriptFinishing(finish contract.FinishReason, cacheCreation int) testkit.Script {
	return testkit.Script{
		Name:          "fake provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Text:                "as far as it got",
			Finish:              finish,
			Usage:               contract.Usage{InputTokens: 900, CachedInputTokens: 5200, OutputTokens: 64},
			CacheCreationTokens: cacheCreation,
		}},
	}
}

func TestTheCacheCreationCountIsAddedToTheInputCount(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptFinishing(contract.FinishLength, 300))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
	}
	want := contract.Usage{InputTokens: 1200, CachedInputTokens: 5200, OutputTokens: 64}
	if reply.Usage != want {
		t.Errorf("the usage came back as %+v, want %+v with the cache-creation tokens added to the input", reply.Usage, want)
	}
	if reply.Finish != contract.FinishLength {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishLength)
	}
}

func TestEveryFinishReasonComesBackAsTheContractsOwnWord(t *testing.T) {
	for _, finish := range []contract.FinishReason{
		contract.FinishEnd, contract.FinishLength, contract.FinishStopped,
	} {
		t.Run(string(finish), func(t *testing.T) {
			server := testkit.NewFakeProviderServer(scriptFinishing(finish, 0))
			defer server.Close()
			anthropic, _, recorder := anthropicAgainst(t, server)

			reply, _, err := sendAndCollect(context.Background(), anthropic, requestWithEverything())

			if err != nil {
				t.Fatalf("one call to the Anthropic provider failed: %v", err)
			}
			if reply.Finish != finish {
				t.Errorf("the Anthropic provider read the stop reason as %q, want %q", reply.Finish, finish)
			}
			if finish != contract.FinishStopped {
				return
			}
			if recorder.count() != 1 || !strings.Contains(recorder.all()[0], "refusal") {
				t.Errorf("the reason the model gave was not written down: %v", recorder.all())
			}
		})
	}
}

func TestTheOpenAIProviderReadsEveryFinishReasonTheSameWay(t *testing.T) {
	for _, finish := range []contract.FinishReason{
		contract.FinishEnd, contract.FinishLength, contract.FinishStopped,
	} {
		t.Run(string(finish), func(t *testing.T) {
			server := testkit.NewFakeProviderServer(scriptFinishing(finish, 0))
			defer server.Close()
			model, _ := openAIAgainst(t, server)

			reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

			if err != nil {
				t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
			}
			if reply.Finish != finish {
				t.Errorf("the OpenAI-compatible provider read the finish reason as %q, want %q", reply.Finish, finish)
			}
		})
	}
}

// scriptGoingWrongMidStream is a step whose stream starts well and then carries
// an error instead of an ending.
func scriptGoingWrongMidStream(said string) testkit.Script {
	return testkit.Script{
		Name:          "fake provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Text:           "starting to answer",
			MidStreamError: said,
			Usage:          contract.Usage{InputTokens: 100, OutputTokens: 3},
		}},
	}
}

func TestAnErrorThatArrivesAfterTheReplyStartedIsReadByBothProviders(t *testing.T) {
	for _, wire := range []string{"anthropic", "openai"} {
		t.Run(wire, func(t *testing.T) {
			server := testkit.NewFakeProviderServer(scriptGoingWrongMidStream("the server is overloaded"))
			defer server.Close()
			model := contract.Model(nil)
			if wire == "anthropic" {
				model, _, _ = anthropicAgainst(t, server)
			} else {
				model, _ = openAIAgainst(t, server)
			}

			_, err := model.Send(context.Background(), requestWithEverything(), nil)

			if err == nil {
				t.Fatal("an error sent part way through the stream came back as a good reply")
			}
			if !strings.Contains(err.Error(), "the server is overloaded") {
				t.Errorf("the error does not say what the server said: %v", err)
			}
		})
	}
}

func TestAHarnessThatDroppedTheUsersWordsIsCaughtOnTheWire(t *testing.T) {
	script := testkit.Script{
		Name:          "fake provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Expect: []string{"a sentence the request never carried"},
			Text:   "never reached",
		}},
	}
	server := testkit.NewFakeProviderServer(script)
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("the server was satisfied although the request carried nothing it expected")
	}
	if errors.Is(err, contract.ErrContextOverflow) {
		t.Errorf("a missing expectation was read as an overflow: %v", err)
	}
}

func TestTheToolCallsTheFakeSendsInPiecesJoinOnBothWires(t *testing.T) {
	for _, wire := range []string{"anthropic", "openai"} {
		t.Run(wire, func(t *testing.T) {
			server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
			defer server.Close()
			model := contract.Model(nil)
			if wire == "anthropic" {
				model, _, _ = anthropicAgainst(t, server)
			} else {
				model, _ = openAIAgainst(t, server)
			}

			reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

			if err != nil {
				t.Fatalf("one call failed: %v", err)
			}
			if len(reply.ToolCalls) != 2 {
				t.Fatalf("the reply carries %d tool calls, want two: %+v", len(reply.ToolCalls), reply.ToolCalls)
			}
			if string(reply.ToolCalls[0].Input) != `{"path":"notes.md"}` {
				t.Errorf("the first call's arguments came back as %s, and the pieces must join to the whole", reply.ToolCalls[0].Input)
			}
			if string(reply.ToolCalls[1].Input) != `{"path":"draft.md","text":"hello"}` {
				t.Errorf("the second call's arguments came back as %s, and the pieces must join to the whole", reply.ToolCalls[1].Input)
			}
		})
	}
}
