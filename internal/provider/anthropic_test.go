package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// anthropicAgainst builds the Anthropic provider pointed at the fake server.
func anthropicAgainst(t *testing.T, server *testkit.FakeProviderServer) (contract.Model, provider.Options, *noteRecorder) {
	t.Helper()
	options, lines := testOptions(t, newTestClock())
	options.APIKey = "test-key"
	model, err := provider.New(contract.ModelAlias{
		Name:          "opus",
		Provider:      contract.ProviderAnthropic,
		BaseAddress:   server.Address(),
		ModelName:     "claude-opus-4-8",
		ContextLength: 200000,
	}, options)
	if err != nil {
		t.Fatalf("building the Anthropic provider failed: %v", err)
	}
	return model, options, lines
}

func TestTheAnthropicProviderStreamsTheTextInDeltasThatJoinToTheReply(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("The launch post is ready to send."))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
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
	if model.Name() != "opus" || model.ContextLength() != 200000 {
		t.Errorf("the model calls itself %q with a window of %d, want opus with 200000", model.Name(), model.ContextLength())
	}
	if reply.Model != "opus" {
		t.Errorf("the reply says %q answered, want the alias that was asked", reply.Model)
	}
}

func TestTheAnthropicProviderReturnsTwoToolCallsWhoseJSONArrivedInPieces(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
	}
	if len(reply.ToolCalls) != 2 {
		t.Fatalf("the reply carries %d tool calls, want two: %+v", len(reply.ToolCalls), reply.ToolCalls)
	}
	if reply.ToolCalls[0].ID != "call_1" || reply.ToolCalls[0].Name != contract.ToolRead {
		t.Errorf("the first tool call is %+v, want call_1 asking for read", reply.ToolCalls[0])
	}
	if string(reply.ToolCalls[1].Input) != `{"path":"draft.md","text":"hello"}` {
		t.Errorf("the second call's arguments came back as %s, and the pieces must join to the whole", reply.ToolCalls[1].Input)
	}
	if strings.Contains(streamed, "draft.md") {
		t.Errorf("the tool-call JSON was streamed to the delta callback, and only text belongs there: %q", streamed)
	}
	if reply.Finish != contract.FinishToolCalls {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishToolCalls)
	}
}

func TestTheAnthropicProviderPutsTheCacheMarkersWhereTheBoundariesSay(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
	}

	body := bodyOfLastCallTo(t, server, testkit.AnthropicPath)
	system, isList := body["system"].([]any)
	if !isList || len(system) != 4 {
		t.Fatalf("the request carries %v as its system prompt, want four text blocks", body["system"])
	}
	marked := []int{}
	for at, block := range system {
		if _, has := block.(map[string]any)["cache_control"]; has {
			marked = append(marked, at)
		}
	}
	if len(marked) != 2 || marked[0] != 0 || marked[1] != 2 {
		t.Errorf("the system blocks carrying a cache marker are %v, want the persona block and the record's goal block", marked)
	}
	tools, isList := body["tools"].([]any)
	if !isList || len(tools) != 2 {
		t.Fatalf("the request carries %v as its tools, want two", body["tools"])
	}
	if _, has := tools[1].(map[string]any)["cache_control"]; !has {
		t.Error("the last tool carries no cache marker, and boundary B is expressed there")
	}
	if _, has := tools[0].(map[string]any)["cache_control"]; has {
		t.Error("a tool other than the last one carries a cache marker, and only the last one may")
	}
}

func TestTheAnthropicProviderNeverSendsTheSamplingFieldsOrThinking(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
	}

	body := bodyOfLastCallTo(t, server, testkit.AnthropicPath)
	for _, forbidden := range []string{"temperature", "top_p", "top_k", "thinking"} {
		if _, has := body[forbidden]; has {
			t.Errorf("the request carries %q, and Opus 4.8 rejects the sampling fields", forbidden)
		}
	}
	if body["max_tokens"] != float64(2048) {
		t.Errorf("the request caps the output at %v, want the request's own 2048", body["max_tokens"])
	}
	if body["stream"] != true {
		t.Error("the request does not ask for a stream, and this provider always streams")
	}
	last := server.Requests()[len(server.Requests())-1]
	if last.Header.Get("x-api-key") != "test-key" {
		t.Errorf("the key header is %q, want the key the options carried", last.Header.Get("x-api-key"))
	}
	if last.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("the version header is %q, want 2023-06-01", last.Header.Get("anthropic-version"))
	}
}

func TestTheAnthropicProviderReportsTheUsageCounts(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
	}
	// The input count is everything the model read, so on this wire it is the
	// plain input tokens plus what was written into the cache plus what was read
	// back out of it. The fake sends the plain field as the remainder, so the
	// three add back up to the script's own 6100 and one script reports the same
	// usage on this wire as on the OpenAI one.
	want := contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}
	if reply.Usage != want {
		t.Errorf("the reply reports the usage as %+v, want %+v", reply.Usage, want)
	}
	if reply.Usage.CachedInputTokens > reply.Usage.InputTokens {
		t.Errorf("the reply says %d of %d input tokens were cached, and the cached count is a part of the input count",
			reply.Usage.CachedInputTokens, reply.Usage.InputTokens)
	}
}

func TestTheAnthropicProviderReturnsTheOverflowSentinelAndNeverRetriesIt(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)
	server.MisbehaveNext(testkit.OverflowTheContext, 0)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if !errors.Is(err, contract.ErrContextOverflow) {
		t.Fatalf("a prompt that was too long came back as %v, want the overflow sentinel", err)
	}
	if !strings.Contains(err.Error(), "opus") {
		t.Errorf("the overflow error does not name the model: %v", err)
	}
}

func TestTheAnthropicProviderReturnsTheRateLimitSentinelWithTheRetryAfter(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)
	server.MisbehaveNext(testkit.RateLimitTheCall, 17*time.Second)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	limited := contract.RateLimitedError{}
	if !errors.As(err, &limited) {
		t.Fatalf("a rate-limited call came back as %v, want the rate-limit sentinel", err)
	}
	if limited.RetryAfter != 17*time.Second {
		t.Errorf("the provider asked the caller to wait %s, want the header's seventeen seconds", limited.RetryAfter)
	}
}

func TestTheAnthropicProviderNamesTheModelWhenTheStreamIsDropped(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)
	server.MisbehaveNext(testkit.DropTheStream, 0)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a stream that stopped part way through came back as a good reply")
	}
	if !strings.Contains(err.Error(), "opus") {
		t.Errorf("the error does not name the model that dropped the stream: %v", err)
	}
}

func TestTheAnthropicProviderPassesTheContractCheck(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("Anything at all."))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	if err := testkit.CheckModel(context.Background(), model); err != nil {
		t.Fatalf("the Anthropic provider does not keep the model contract: %v", err)
	}
}

func TestTheAnthropicProviderRefusesAnAliasWithNoModelName(t *testing.T) {
	options, _ := testOptions(t, newTestClock())

	_, err := provider.New(contract.ModelAlias{
		Name:          "opus",
		Provider:      contract.ProviderAnthropic,
		ContextLength: 200000,
	}, options)

	if err == nil {
		t.Fatal("an alias with no model name was accepted, and the request needs one")
	}
	if !strings.Contains(err.Error(), "opus") {
		t.Errorf("the error does not name the alias it is about: %v", err)
	}
}

// TestTheAnthropicProviderCountsTheThinkingAndToolCallsItWritesUnseen is the
// screen's "21 s · 0 tokens" through two minutes of the local model writing a
// file: only text reaches the delta callback, so a model thinking, or writing
// a tool call, was shown as writing nothing. The provider now tells the
// options how many characters it wrote that no delta shows.
func TestTheAnthropicProviderCountsTheThinkingAndToolCallsItWritesUnseen(t *testing.T) {
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
		Name: "opus", Provider: contract.ProviderAnthropic, BaseAddress: server.Address(),
		ModelName: "claude-opus-4-8", ContextLength: 200000,
	}, options)
	if err != nil {
		t.Fatalf("building the Anthropic provider failed: %v", err)
	}

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Anthropic provider failed: %v", err)
	}
	if streamed != "Reading the notes." || reply.Text != "Reading the notes." {
		t.Errorf("the deltas joined to %q and the reply is %q, and the thinking belongs in neither", streamed, reply.Text)
	}
	if wanted := len("The notes come first, then the draft.") + len(`{"path":"notes.md"}`); unseen != wanted {
		t.Errorf("the provider counted %d characters written unseen, want %d: the thinking and the tool call's arguments", unseen, wanted)
	}
}
