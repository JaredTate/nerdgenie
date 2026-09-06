package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// openAIAgainst builds the OpenAI-compatible provider pointed at the fake
// server, whose base address already ends in the version folder.
func openAIAgainst(t *testing.T, server *testkit.FakeProviderServer) (contract.Model, *noteRecorder) {
	t.Helper()
	options, lines := testOptions(t, newTestClock())
	options.APIKey = "test-key"
	model, err := provider.New(contract.ModelAlias{
		Name:          contract.LocalModelAlias,
		Provider:      contract.ProviderOpenAI,
		BaseAddress:   server.Address() + "/v1",
		ModelName:     "local-coder",
		ContextLength: 262144,
	}, options)
	if err != nil {
		t.Fatalf("building the OpenAI-compatible provider failed: %v", err)
	}
	return model, lines
}

func TestTheOpenAIProviderStreamsTheTextInDeltasThatJoinToTheReply(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("The launch post is ready to send."))
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}
	if reply.Text != "The launch post is ready to send." {
		t.Errorf("the reply text is %q, want the script's line", reply.Text)
	}
	if streamed != reply.Text {
		t.Errorf("the deltas joined to %q and the reply is %q, and the two must be the same", streamed, reply.Text)
	}
	if reply.Finish != contract.FinishEnd {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishEnd)
	}
	if reply.Model != contract.LocalModelAlias {
		t.Errorf("the reply says %q answered, want the alias that was asked", reply.Model)
	}
}

func TestTheOpenAIProviderReturnsTheStreamedToolCalls(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}
	if len(reply.ToolCalls) != 2 {
		t.Fatalf("the reply carries %d tool calls, want two: %+v", len(reply.ToolCalls), reply.ToolCalls)
	}
	if reply.ToolCalls[0].ID != "call_1" || reply.ToolCalls[0].Name != contract.ToolRead {
		t.Errorf("the first tool call is %+v, want call_1 asking for read", reply.ToolCalls[0])
	}
	if string(reply.ToolCalls[1].Input) != `{"path":"draft.md","text":"hello"}` {
		t.Errorf("the second call's arguments came back as %s, want the whole object", reply.ToolCalls[1].Input)
	}
	if strings.Contains(streamed, "draft.md") {
		t.Errorf("the tool-call JSON was streamed to the delta callback, and only text belongs there: %q", streamed)
	}
	if reply.Finish != contract.FinishToolCalls {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishToolCalls)
	}
}

func TestTheOpenAIProviderJoinsTheSystemBlocksAndSendsNoCacheMarkers(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}

	body := bodyOfLastCallTo(t, server, testkit.OpenAIPath)
	messages, isList := body["messages"].([]any)
	if !isList || len(messages) == 0 {
		t.Fatalf("the request carries %v as its messages, want a list starting with the system message", body["messages"])
	}
	first, isObject := messages[0].(map[string]any)
	if !isObject || first["role"] != "system" {
		t.Fatalf("the first message is %v, want the joined system prompt", messages[0])
	}
	joined, isText := first["content"].(string)
	if !isText || !strings.Contains(joined, "You are the reasoning engine inside Nerd Genie.\n\nYou have two tools.") {
		t.Errorf("the system blocks were not joined with blank lines between them: %q", first["content"])
	}
	for _, forbidden := range []string{"cache_control", "temperature", "top_p", "top_k"} {
		if strings.Contains(string(server.Requests()[len(server.Requests())-1].Body), forbidden) {
			t.Errorf("the request carries %q, and this API has neither cache markers nor sampling fields", forbidden)
		}
	}
}

func TestTheOpenAIProviderCapsTheOutputAndAsksForTheUsage(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}

	body := bodyOfLastCallTo(t, server, testkit.OpenAIPath)
	if body["max_completion_tokens"] != float64(2048) {
		t.Errorf("the request caps the output with %v, want max_completion_tokens of 2048", body["max_completion_tokens"])
	}
	if _, has := body["max_tokens"]; has {
		t.Error("the request carries max_tokens, and the field this provider sends is max_completion_tokens")
	}
	options, isObject := body["stream_options"].(map[string]any)
	if !isObject || options["include_usage"] != true {
		t.Errorf("the request asks for the usage with %v, want stream_options include_usage", body["stream_options"])
	}
	last := server.Requests()[len(server.Requests())-1]
	if last.Header.Get("Authorization") != "Bearer test-key" {
		t.Errorf("the key header is %q, want a bearer token", last.Header.Get("Authorization"))
	}
}

func TestTheOpenAIProviderSendsAToolResultAsAToolMessage(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}

	body := bodyOfLastCallTo(t, server, testkit.OpenAIPath)
	messages, _ := body["messages"].([]any)
	found := false
	for _, written := range messages {
		message, _ := written.(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		found = true
		if message["tool_call_id"] != "call_1" {
			t.Errorf("the tool message answers %v, want call_1", message["tool_call_id"])
		}
		if message["content"] != "The launch is on Friday." {
			t.Errorf("the tool message carries %v, want the result's text", message["content"])
		}
	}
	if !found {
		t.Errorf("no message has the tool role, and that is how a result goes back: %v", messages)
	}
}

func TestTheOpenAIProviderReportsTheUsageCounts(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}
	// This wire reports the whole prompt in one field, with the cached part named
	// separately inside it, so the counts come back exactly as the script wrote
	// them.
	want := contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}
	if reply.Usage != want {
		t.Errorf("the reply reports the usage as %+v, want %+v", reply.Usage, want)
	}
}

func TestTheOpenAIProviderReturnsTheOverflowSentinel(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)
	server.MisbehaveNext(testkit.OverflowTheContext, 0)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if !errors.Is(err, contract.ErrContextOverflow) {
		t.Fatalf("a prompt that was too long came back as %v, want the overflow sentinel", err)
	}
}

func TestTheOpenAIProviderReturnsTheRateLimitSentinelWithTheRetryAfter(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)
	server.MisbehaveNext(testkit.RateLimitTheCall, 9*time.Second)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	limited := contract.RateLimitedError{}
	if !errors.As(err, &limited) {
		t.Fatalf("a rate-limited call came back as %v, want the rate-limit sentinel", err)
	}
	if limited.RetryAfter != 9*time.Second {
		t.Errorf("the provider asked the caller to wait %s, want the header's nine seconds", limited.RetryAfter)
	}
}

func TestTheOpenAIProviderNamesTheModelWhenTheStreamIsDropped(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)
	server.MisbehaveNext(testkit.DropTheStream, 0)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a stream that stopped part way through came back as a good reply")
	}
	if !strings.Contains(err.Error(), contract.LocalModelAlias) {
		t.Errorf("the error does not name the model that dropped the stream: %v", err)
	}
}

func TestTheOpenAIProviderPassesTheContractCheck(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("Anything at all."))
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	if err := testkit.CheckModel(context.Background(), model); err != nil {
		t.Fatalf("the OpenAI-compatible provider does not keep the model contract: %v", err)
	}
}

func TestTheOpenAIProviderSendsNoToolsWhenTheHarnessSwitchedThemOff(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)
	request := requestWithEverything()
	request.ToolsOff = true

	if _, err := model.Send(context.Background(), request, nil); err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}

	body := bodyOfLastCallTo(t, server, testkit.OpenAIPath)
	if _, has := body["tools"]; has {
		t.Errorf("the request carries tools although the harness switched them off: %v", body["tools"])
	}
}

// TestTheOpenAIProviderCountsTheThinkingAndToolCallsItWritesUnseen is the
// screen's "21 s · 0 tokens" through two minutes of the local model writing a
// file: only text reaches the delta callback, so a model thinking, or writing
// a tool call, was shown as writing nothing. The provider now tells the
// options how many characters it wrote that no delta shows.
func TestTheOpenAIProviderCountsTheThinkingAndToolCallsItWritesUnseen(t *testing.T) {
	server := testkit.NewFakeProviderServer(testkit.Script{
		Name:          "fake provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Thinking:  "The notes come first, then the draft.",
			Text:      "Reading the notes.",
			ToolCalls: []contract.ToolCall{{ID: "call_1", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"notes.md"}`)}},
			Finish:    contract.FinishToolCalls,
		}},
	})
	defer server.Close()
	options, _ := testOptions(t, newTestClock())
	options.APIKey = "test-key"
	unseen := 0
	options.Unseen = func(characters int) { unseen += characters }
	model, err := provider.New(contract.ModelAlias{
		Name: contract.LocalModelAlias, Provider: contract.ProviderOpenAI, BaseAddress: server.Address() + "/v1",
		ModelName: "local-coder", ContextLength: 262144,
	}, options)
	if err != nil {
		t.Fatalf("building the OpenAI-compatible provider failed: %v", err)
	}

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}
	if streamed != "Reading the notes." || reply.Text != "Reading the notes." {
		t.Errorf("the deltas joined to %q and the reply is %q, and the thinking belongs in neither", streamed, reply.Text)
	}
	if wanted := len("The notes come first, then the draft.") + len(`{"path":"notes.md"}`); unseen != wanted {
		t.Errorf("the provider counted %d characters written unseen, want %d: the thinking and the tool call's arguments", unseen, wanted)
	}
}
