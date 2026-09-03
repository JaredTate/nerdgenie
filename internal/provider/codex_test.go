package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
)

// The tests in this file are the wire: what the Codex provider puts on it, and
// how it reads what comes back. The sign-in failures, the retries, and the
// quiet stream are in codexfailures_test.go.

func TestTheCodexProviderPostsToTheResponsesAddressWithTheSignInHeaders(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
	model, _ := codexAgainst(t, backend)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}

	call := backend.lastCall(t)
	if call.path != "/responses" {
		t.Errorf("the provider posted to %q, want the Responses API at /responses under the base address", call.path)
	}
	if got := call.header.Get("Authorization"); got != "Bearer "+codexTokenOnDisk(t) {
		t.Errorf("the Authorization header is not the bearer token from the sign-in file: %q", got)
	}
	if got := call.header.Get("ChatGPT-Account-Id"); got != codexAccountFromFile {
		t.Errorf("the account header is %q, want the account identifier the sign-in file names, %q", got, codexAccountFromFile)
	}
	if got := call.header.Get("originator"); got != "codex_cli_rs" {
		t.Errorf("the originator header is %q, want codex_cli_rs, which is what the backend lets through", got)
	}
	if got := call.header.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Errorf("the OpenAI-Beta header is %q, want responses=experimental", got)
	}
	if got := call.header.Get("session_id"); got == "" {
		t.Error("the request carries no session_id header, and the backend scopes its cache by one")
	}
	if got := call.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("the content type is %q, want application/json", got)
	}
	if got := call.header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("the accept header is %q, want text/event-stream", got)
	}
	if !strings.HasPrefix(call.header.Get("User-Agent"), "codex_cli_rs/") {
		t.Errorf("the user agent is %q, want one shaped like the codex program's", call.header.Get("User-Agent"))
	}
}

func TestTheCodexProviderSendsTheSystemPromptAsInstructionsAndTheConversationAsInputItems(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
	model, _ := codexAgainst(t, backend)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}

	body := backend.lastBody(t)
	if body["model"] != "gpt-5.6-sol" {
		t.Errorf("the request names the model %v, want gpt-5.6-sol", body["model"])
	}
	instructions, isText := body["instructions"].(string)
	if !isText || !strings.Contains(instructions, "You are the reasoning engine inside Coeus.\n\nYou have two tools.") {
		t.Errorf("the system blocks were not joined into the instructions with blank lines between them: %v", body["instructions"])
	}
	if body["store"] != false {
		t.Errorf("the request stores the reply with store=%v, and the backend takes only store=false", body["store"])
	}
	if body["stream"] != true {
		t.Errorf("the request asks for stream=%v, and this provider always streams", body["stream"])
	}
	for _, forbidden := range []string{"max_output_tokens", "temperature", "top_p", "cache_control"} {
		if _, has := body[forbidden]; has {
			t.Errorf("the request carries %q, which this backend refuses or which has no place on this wire", forbidden)
		}
	}

	items, isList := body["input"].([]any)
	if !isList || len(items) != 4 {
		t.Fatalf("the request carries %v as its input, want four items: the ask, the assistant's words, the call, and the result", body["input"])
	}
	first := items[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "Post the tweet about the launch." {
		t.Errorf("the first item is %v, want the user's ask", first)
	}
	second := items[1].(map[string]any)
	if second["role"] != "assistant" || second["content"] != "Reading the notes." {
		t.Errorf("the second item is %v, want the assistant's words", second)
	}
	third := items[2].(map[string]any)
	if third["type"] != "function_call" || third["call_id"] != "call_1" || third["name"] != contract.ToolRead || third["arguments"] != `{"path":"notes.md"}` {
		t.Errorf("the third item is %v, want a function_call for call_1 with the read tool's arguments", third)
	}
	fourth := items[3].(map[string]any)
	if fourth["type"] != "function_call_output" || fourth["call_id"] != "call_1" || fourth["output"] != "The launch is on Friday." {
		t.Errorf("the fourth item is %v, want a function_call_output answering call_1 with the result's text", fourth)
	}
}

func TestTheCodexProviderMarksAFailedToolResultAndFillsInEmptyArguments(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
	model, _ := codexAgainst(t, backend)
	request := contract.Request{
		Messages: []contract.Message{
			{Role: contract.RoleUser, Text: "Try it."},
			{Role: contract.RoleAssistant, ToolCalls: []contract.ToolCall{{ID: "call_9", Name: contract.ToolRead}}},
			{Role: contract.RoleUser, ToolResults: []contract.ToolResult{{CallID: "call_9", Text: "no such file", Failed: true}}},
		},
	}

	if _, err := model.Send(context.Background(), request, nil); err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}

	items, _ := backend.lastBody(t)["input"].([]any)
	if len(items) != 3 {
		t.Fatalf("the request carries %d input items, want three: the ask, the call, and the failed result", len(items))
	}
	call := items[1].(map[string]any)
	if call["arguments"] != "{}" {
		t.Errorf("a call with no arguments went over as %v, want an empty object", call["arguments"])
	}
	result := items[2].(map[string]any)
	output, _ := result["output"].(string)
	if !strings.HasPrefix(output, "This tool call failed.") || !strings.Contains(output, "no such file") {
		t.Errorf("the failed result went over as %q, and the model has to be told it failed", output)
	}
}

func TestTheCodexProviderSendsTheToolsAsFunctionTools(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
	model, _ := codexAgainst(t, backend)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}

	body := backend.lastBody(t)
	tools, isList := body["tools"].([]any)
	if !isList || len(tools) != 2 {
		t.Fatalf("the request carries %v as its tools, want the two the request named", body["tools"])
	}
	first := tools[0].(map[string]any)
	if first["type"] != "function" || first["name"] != contract.ToolRead {
		t.Errorf("the first tool is %v, want a function tool named read", first)
	}
	if first["strict"] != false {
		t.Errorf("the tool is sent with strict=%v, and the harness's schemas are not written for strict mode", first["strict"])
	}
	parameters, _ := first["parameters"].(map[string]any)
	properties, _ := parameters["properties"].(map[string]any)
	if _, has := properties["path"]; !has {
		t.Errorf("the read tool's parameters are %v, want a JSON schema naming path", first["parameters"])
	}
	if body["tool_choice"] != "auto" {
		t.Errorf("the request sets tool_choice to %v, want auto", body["tool_choice"])
	}
	if body["parallel_tool_calls"] != true {
		t.Errorf("the request sets parallel_tool_calls to %v, want true", body["parallel_tool_calls"])
	}
}

func TestTheCodexProviderSendsNoToolsWhenTheHarnessSwitchedThemOff(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
	model, _ := codexAgainst(t, backend)
	request := requestWithEverything()
	request.ToolsOff = true

	if _, err := model.Send(context.Background(), request, nil); err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}

	body := backend.lastBody(t)
	for _, key := range []string{"tools", "tool_choice", "parallel_tool_calls"} {
		if _, has := body[key]; has {
			t.Errorf("the request carries %q although the harness switched the tools off: %v", key, body[key])
		}
	}
}

func TestTheCodexProviderAsksForReasoningEffortOnlyWhenAsked(t *testing.T) {
	tests := []struct {
		name     string
		onAlias  contract.Think
		onCall   contract.Think
		wantSent bool
		want     string
	}{
		{"the empty default says nothing", contract.ThinkDefault, contract.ThinkDefault, false, ""},
		{"off says nothing", contract.ThinkOff, contract.ThinkDefault, false, ""},
		{"the alias level goes over", contract.ThinkMedium, contract.ThinkDefault, true, "medium"},
		{"the call's level wins over the alias", contract.ThinkMedium, contract.ThinkHigh, true, "high"},
		{"off on the call wins over the alias", contract.ThinkMedium, contract.ThinkOff, false, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
			goodCodexLogin(t)
			options, _ := testOptions(t, newTestClock())
			model, err := provider.New(codexAliasAt(backend.address(), test.onAlias), options)
			if err != nil {
				t.Fatalf("building the Codex provider failed: %v", err)
			}
			request := requestWithEverything()
			request.Think = test.onCall

			if _, err := model.Send(context.Background(), request, nil); err != nil {
				t.Fatalf("one call to the Codex provider failed: %v", err)
			}

			body := backend.lastBody(t)
			reasoning, has := body["reasoning"].(map[string]any)
			if has != test.wantSent {
				t.Fatalf("the request carries reasoning=%v, want sent=%v", body["reasoning"], test.wantSent)
			}
			if test.wantSent && reasoning["effort"] != test.want {
				t.Errorf("the request asks for the effort %v, want %q", reasoning["effort"], test.want)
			}
		})
	}
}

func TestTheCodexProviderStreamsTheTextInDeltasThatJoinToTheReply(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("The launch post ", "is ready to send.")})
	model, _ := codexAgainst(t, backend)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}
	if reply.Text != "The launch post is ready to send." {
		t.Errorf("the reply text is %q, want the two deltas joined", reply.Text)
	}
	if streamed != reply.Text {
		t.Errorf("the deltas joined to %q and the reply is %q, and the two must be the same", streamed, reply.Text)
	}
	if reply.Finish != contract.FinishEnd {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishEnd)
	}
	if reply.Model != "gpt" {
		t.Errorf("the reply says %q answered, want the alias that was asked", reply.Model)
	}
	if len(reply.ToolCalls) != 0 {
		t.Errorf("a plain answer came back with %d tool calls", len(reply.ToolCalls))
	}
}

func TestTheCodexProviderReturnsTheStreamedToolCalls(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTwoCallStream()})
	model, _ := codexAgainst(t, backend)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}
	if len(reply.ToolCalls) != 2 {
		t.Fatalf("the reply carries %d tool calls, want two: %+v", len(reply.ToolCalls), reply.ToolCalls)
	}
	if reply.ToolCalls[0].ID != "call_1" || reply.ToolCalls[0].Name != contract.ToolRead {
		t.Errorf("the first tool call is %+v, want call_1 asking for read", reply.ToolCalls[0])
	}
	if string(reply.ToolCalls[0].Input) != `{"path":"notes.md"}` {
		t.Errorf("the first call's arguments came back as %s, want the streamed pieces joined", reply.ToolCalls[0].Input)
	}
	if reply.ToolCalls[1].ID != "call_2" || reply.ToolCalls[1].Name != contract.ToolWrite {
		t.Errorf("the second tool call is %+v, want call_2 asking for write", reply.ToolCalls[1])
	}
	if string(reply.ToolCalls[1].Input) != `{"path":"draft.md","text":"hello"}` {
		t.Errorf("the second call's arguments came back as %s, want the whole object from the finished item", reply.ToolCalls[1].Input)
	}
	if strings.Contains(streamed, "draft.md") {
		t.Errorf("the tool-call JSON was streamed to the delta callback, and only text belongs there: %q", streamed)
	}
	if reply.Finish != contract.FinishToolCalls {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishToolCalls)
	}
}

func TestTheCodexProviderReportsTheUsageCounts(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTwoCallStream()})
	model, _ := codexAgainst(t, backend)

	reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}
	// The Responses API reports the whole prompt in input_tokens with the cached
	// part named inside it, so the counts come back as the stream wrote them,
	// and no money is reported, because a subscription has no price per call.
	want := contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}
	if reply.Usage != want {
		t.Errorf("the reply reports the usage as %+v, want %+v", reply.Usage, want)
	}
}

func TestTheCodexProviderReadsAReplyCutAtTheOutputCapAsLength(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexEvents(
		`{"type":"response.output_text.delta","delta":"half an answer","item_id":"msg_1"}`,
		`{"type":"response.incomplete","response":{"id":"resp_3","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":5}}}`,
	)})
	model, _ := codexAgainst(t, backend)

	reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}
	if reply.Finish != contract.FinishLength {
		t.Errorf("a reply the backend cut at the cap finished with %q, want %q", reply.Finish, contract.FinishLength)
	}
	if reply.Text != "half an answer" || reply.Usage.OutputTokens != 5 {
		t.Errorf("the cut reply came back as %+v, want its text and its usage kept", reply)
	}
}

func TestTheCodexProviderReadsAnyOtherIncompleteReasonAsStopped(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexEvents(
		`{"type":"response.incomplete","response":{"id":"resp_4","status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`,
	)})
	model, _ := codexAgainst(t, backend)

	reply, _, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}
	if reply.Finish != contract.FinishStopped {
		t.Errorf("a reply the backend stopped finished with %q, want %q", reply.Finish, contract.FinishStopped)
	}
}

func TestTheCodexProviderNamesTheModelWhenTheBackendFailsMidStream(t *testing.T) {
	tests := []struct {
		name   string
		events string
	}{
		{"a failed response", codexEvents(`{"type":"response.failed","response":{"id":"resp_5","status":"failed","error":{"code":"server_error","message":"the backend gave up"}}}`)},
		{"an error event", codexEvents(`{"type":"error","code":"server_error","message":"the backend gave up"}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newCodexBackend(t, codexAnswer{events: test.events})
			model, _ := codexAgainst(t, backend)

			_, err := model.Send(context.Background(), requestWithEverything(), nil)

			if err == nil {
				t.Fatal("a stream that failed part way through came back as a good reply")
			}
			if !strings.Contains(err.Error(), "gpt") || !strings.Contains(err.Error(), "the backend gave up") {
				t.Errorf("the error does not name the model and say what the backend said: %v", err)
			}
		})
	}
}

func TestTheCodexProviderRefusesAStreamThatEndsBeforeTheReplyIsComplete(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexEvents(
		`{"type":"response.created","response":{"id":"resp_6","status":"in_progress"}}`,
		`{"type":"response.output_text.delta","delta":"and then nothing","item_id":"msg_1"}`,
	)})
	model, _ := codexAgainst(t, backend)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a stream that ended with no completed event came back as a good reply")
	}
	if !strings.Contains(err.Error(), "gpt") || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("the error does not name the model and say the reply was incomplete: %v", err)
	}
}

func TestTheCodexProviderIgnoresTheEventsItDoesNotRead(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexEvents(
		`{"type":"response.created","response":{"id":"resp_7","status":"in_progress"}}`,
		`{"type":"response.output_item.added","item":{"id":"rs_1","type":"reasoning","summary":[]}}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"thinking about it","item_id":"rs_1"}`,
		`{"type":"response.output_item.done","item":{"id":"rs_1","type":"reasoning","summary":[]}}`,
		`{"type":"response.output_text.delta","delta":"ok","item_id":"msg_1"}`,
		`not json at all`,
		`{"type":"response.completed","response":{"id":"resp_7","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`,
	)})
	model, _ := codexAgainst(t, backend)

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call to the Codex provider failed: %v", err)
	}
	if reply.Text != "ok" || streamed != "ok" {
		t.Errorf("the reply is %q and the deltas joined to %q, and the reasoning summary belongs in neither", reply.Text, streamed)
	}
}

// codexTokenOnDisk reads the token back out of the fake sign-in file the test
// wrote, so that the header check compares against what was really there.
func codexTokenOnDisk(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json"))
	if err != nil {
		t.Fatalf("the fake sign-in file could not be read back: %v", err)
	}
	start := strings.Index(string(contents), `"access_token":"`) + len(`"access_token":"`)
	end := strings.Index(string(contents)[start:], `"`)
	return string(contents)[start : start+end]
}
