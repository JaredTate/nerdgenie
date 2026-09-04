package testkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// scriptWithAToolCall is the script both provider tests run against.
func scriptWithAToolCall() testkit.Script {
	return testkit.Script{
		Name:          "provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Text:      "Reading the product notes now.",
			ToolCalls: []contract.ToolCall{{ID: "call_1", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"memory/product.md"}`)}},
			Finish:    contract.FinishToolCalls,
			Usage:     contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400},
		}},
	}
}

// postJSON sends one request to the fake provider server and returns the whole
// answer, which is the stream read to its end.
func postJSON(t *testing.T, address string, body string) (int, http.Header, string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	answer, err := client.Post(address, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("sending the request to %s failed: %v", address, err)
	}
	defer answer.Body.Close()
	read, err := io.ReadAll(answer.Body)
	if err != nil {
		return answer.StatusCode, answer.Header, string(read)
	}
	return answer.StatusCode, answer.Header, string(read)
}

func TestTheFakeProviderSpeaksTheAnthropicStream(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()

	code, _, stream := postJSON(t, server.AnthropicAddress(), `{"model":"opus","messages":[]}`)

	if code != http.StatusOK {
		t.Fatalf("the server answered %d, want 200. It said:\n%s", code, stream)
	}
	for _, event := range []string{
		"event: message_start", "event: content_block_start", "event: content_block_delta",
		"text_delta", "input_json_delta", "event: content_block_stop",
		"event: message_delta", "event: message_stop",
	} {
		if !strings.Contains(stream, event) {
			t.Errorf("the Anthropic stream is missing %q:\n%s", event, stream)
		}
	}
	if !strings.Contains(stream, "cache_read_input_tokens") {
		t.Errorf("the Anthropic stream does not report the cached tokens:\n%s", stream)
	}
}

func TestTheFakeProviderSpeaksTheOpenAIStream(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()

	code, _, stream := postJSON(t, server.OpenAIAddress(), `{"model":"local-coder","messages":[],"stream":true}`)

	if code != http.StatusOK {
		t.Fatalf("the server answered %d, want 200. It said:\n%s", code, stream)
	}
	for _, piece := range []string{"data: ", "tool_calls", "finish_reason", "usage", "data: [DONE]"} {
		if !strings.Contains(stream, piece) {
			t.Errorf("the OpenAI stream is missing %q:\n%s", piece, stream)
		}
	}
}

func TestTheFakeProviderRecordsEveryRequestSoATestCanFindTheCacheMarkers(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()

	postJSON(t, server.AnthropicAddress(), `{"system":[{"type":"text","text":"rules","cache_control":{"type":"ephemeral"}}]}`)

	seen := server.Requests()
	if len(seen) != 1 {
		t.Fatalf("the server recorded %d requests, want 1", len(seen))
	}
	if !strings.Contains(string(seen[0].Body), "cache_control") {
		t.Errorf("the recorded body has no cache marker in it: %s", seen[0].Body)
	}
	if seen[0].Path != "/v1/messages" {
		t.Errorf("the recorded path is %q, want /v1/messages", seen[0].Path)
	}
}

func TestTheFakeProviderCanRateLimitWithARetryAfterHeader(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()
	server.MisbehaveNext(testkit.RateLimitTheCall, 30*time.Second)

	code, header, _ := postJSON(t, server.OpenAIAddress(), `{}`)

	if code != http.StatusTooManyRequests {
		t.Errorf("the server answered %d, want 429", code)
	}
	if header.Get("Retry-After") != "30" {
		t.Errorf("the Retry-After header is %q, want 30", header.Get("Retry-After"))
	}
}

func TestTheFakeProviderCanFailAndCanOverflowInEachApisOwnShape(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()

	server.MisbehaveNext(testkit.FailTheCall, 0)
	if code, _, _ := postJSON(t, server.OpenAIAddress(), `{}`); code != http.StatusInternalServerError {
		t.Errorf("the server answered %d, want 500", code)
	}

	server.MisbehaveNext(testkit.OverflowTheContext, 0)
	code, _, body := postJSON(t, server.AnthropicAddress(), `{}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "invalid_request_error") {
		t.Errorf("the Anthropic overflow answered %d with %q, want 400 and an invalid_request_error", code, body)
	}

	server.MisbehaveNext(testkit.OverflowTheContext, 0)
	code, _, body = postJSON(t, server.OpenAIAddress(), `{}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "context_length_exceeded") {
		t.Errorf("the OpenAI overflow answered %d with %q, want 400 and a context_length_exceeded", code, body)
	}
}

func TestTheFakeProviderCanDropTheStreamPartWayThrough(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()
	server.MisbehaveNext(testkit.DropTheStream, 0)

	_, _, stream := postJSON(t, server.OpenAIAddress(), `{}`)

	if strings.Contains(stream, "[DONE]") {
		t.Errorf("the dropped stream still ended properly:\n%s", stream)
	}
}

func TestTheFakeProviderCanStallBeforeItSaysAnything(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()
	server.MisbehaveNext(testkit.StallTheStream, 300*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.OpenAIAddress(), strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("cannot build the request: %v", err)
	}

	if _, err := http.DefaultClient.Do(request); err == nil {
		t.Error("a stalled stream answered before the deadline, and it was supposed to say nothing")
	}
}

func TestTheFakeProviderRefusesAPathItDoesNotServe(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithAToolCall())
	defer server.Close()

	code, _, _ := postJSON(t, server.Address()+"/v1/nonsense", `{}`)

	if code != http.StatusNotFound {
		t.Errorf("an unknown path answered %d, want 404", code)
	}
}
