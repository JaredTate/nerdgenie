package provider

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// The streams below are written by hand because no fake server speaks the
// Responses API yet. Each is the smallest stream that asks one question of the
// reader: the text, the tool calls, the usage, the three ways a stream can end,
// and the three ways it can be broken.

// codexCompleted is the closing event of a good reply, with a usage that has
// both a cached count inside the input and a reasoning count inside the
// output, and an output list that is null, which is what the backend sends.
const codexCompleted = `{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.5","status":"completed","output":null,` +
	`"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":50,"output_tokens_details":{"reasoning_tokens":30}}}}`

// codexStream joins events into the data lines of a Responses API stream.
func codexStream(events ...string) string {
	joined := strings.Builder{}
	for _, event := range events {
		joined.WriteString("data: " + event + "\n\n")
	}
	return joined.String()
}

// readCodex runs the reader over a stream and keeps the deltas it handed on.
func readCodex(t *testing.T, stream string) (codexResult, string, error) {
	t.Helper()
	written := strings.Builder{}
	result, err := readCodexStream(strings.NewReader(stream), func(delta string) {
		written.WriteString(delta)
	})
	return result, written.String(), err
}

func TestTheCodexReaderJoinsTheTextDeltasIntoTheReply(t *testing.T) {
	stream := codexStream(
		`{"type":"response.created","response":{"id":"resp_1"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`,
		`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"thinking"}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"delta":"Hello, "}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"delta":"world"}`,
		`{"type":"response.output_text.done","item_id":"msg_1","text":"Hello, world"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"Hello, world"}]}}`,
		codexCompleted,
	)

	result, written, err := readCodex(t, stream)

	if err != nil {
		t.Fatalf("a good text stream failed: %v", err)
	}
	if result.Text != "Hello, world" || written != "Hello, world" {
		t.Errorf("the text is %q and the deltas joined to %q, want \"Hello, world\" for both", result.Text, written)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("a text reply came with %d tool calls, want none", len(result.ToolCalls))
	}
	if result.Finish != contract.FinishEnd {
		t.Errorf("the finish is %q, want %q", result.Finish, contract.FinishEnd)
	}
	if result.Model != "gpt-5.5" {
		t.Errorf("the model is %q, want the one the completed event named", result.Model)
	}
}

func TestTheCodexReaderReturnsTwoFunctionCallsOneStreamedAndOneWhole(t *testing.T) {
	stream := codexStream(
		`{"type":"response.output_text.delta","delta":"Let me look."}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"{\"path\":"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"\"a.txt\"}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":1,"arguments":"{\"path\":\"a.txt\"}"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"list_dir","arguments":"{\"path\":\".\"}"}}`,
		codexCompleted,
	)

	result, written, err := readCodex(t, stream)

	if err != nil {
		t.Fatalf("a stream with two function calls failed: %v", err)
	}
	if result.Text != "Let me look." || written != result.Text {
		t.Errorf("the text is %q and the deltas joined to %q, want the one delta for both", result.Text, written)
	}
	want := []contract.ToolCall{
		{ID: "call_1", Name: "read_file", Input: []byte(`{"path":"a.txt"}`)},
		{ID: "call_2", Name: "list_dir", Input: []byte(`{"path":"."}`)},
	}
	if len(result.ToolCalls) != len(want) {
		t.Fatalf("the reply has %d tool calls, want %d: %+v", len(result.ToolCalls), len(want), result.ToolCalls)
	}
	for at, call := range result.ToolCalls {
		if call.ID != want[at].ID || call.Name != want[at].Name || string(call.Input) != string(want[at].Input) {
			t.Errorf("tool call %d is %s %s %s, want %s %s %s", at, call.ID, call.Name, call.Input, want[at].ID, want[at].Name, want[at].Input)
		}
	}
	if result.Finish != contract.FinishToolCalls {
		t.Errorf("the finish is %q, want %q", result.Finish, contract.FinishToolCalls)
	}
}

func TestTheCodexReaderKeepsTheStreamedArgumentsWhenTheDoneEventHasNone(t *testing.T) {
	stream := codexStream(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":\"b.txt\"}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","call_id":"call_2","name":"list_dir"}}`,
		codexCompleted,
	)

	result, _, err := readCodex(t, stream)

	if err != nil {
		t.Fatalf("the stream failed: %v", err)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("the reply has %d tool calls, want 2", len(result.ToolCalls))
	}
	if string(result.ToolCalls[0].Input) != `{"path":"b.txt"}` {
		t.Errorf("the first call's arguments are %s, want the streamed ones kept", result.ToolCalls[0].Input)
	}
	if string(result.ToolCalls[1].Input) != "{}" {
		t.Errorf("a call with no arguments at all has %s, want an empty object", result.ToolCalls[1].Input)
	}
}

func TestTheCodexReaderReportsTheCachedAndReasoningTokens(t *testing.T) {
	result, err := readCodexStream(strings.NewReader(codexStream(codexCompleted)), nil)

	if err != nil {
		t.Fatalf("a stream with only the completed event failed: %v", err)
	}
	want := contract.Usage{InputTokens: 100, CachedInputTokens: 40, OutputTokens: 50}
	if result.Usage != want {
		t.Errorf("the usage is %+v, want %+v: the cached count sits inside the input and the reasoning count inside the output", result.Usage, want)
	}
}

func TestTheCodexReaderStopsAtTheDoneMarkerAfterTheCompletedEvent(t *testing.T) {
	stream := codexStream(`{"type":"response.output_text.delta","delta":"done"}`, codexCompleted) +
		"data: [DONE]\n\n" + codexStream(`{"type":"error","message":"this line must never be read"}`)

	result, _, err := readCodex(t, stream)

	if err != nil {
		t.Fatalf("a stream ending in the done marker failed: %v", err)
	}
	if result.Text != "done" {
		t.Errorf("the text is %q, want \"done\"", result.Text)
	}
}

func TestTheCodexReaderReadsAnIncompleteReplyAsLengthOrStopped(t *testing.T) {
	for reason, want := range map[string]contract.FinishReason{
		"max_output_tokens": contract.FinishLength,
		"content_filter":    contract.FinishStopped,
	} {
		stream := codexStream(
			`{"type":"response.output_text.delta","delta":"part of"}`,
			`{"type":"response.incomplete","response":{"model":"gpt-5.5","status":"incomplete","incomplete_details":{"reason":"`+reason+`"},`+
				`"usage":{"input_tokens":7,"output_tokens":3}}}`,
		)

		result, written, err := readCodex(t, stream)

		if err != nil {
			t.Fatalf("an incomplete reply for %q failed: %v", reason, err)
		}
		if result.Finish != want {
			t.Errorf("an incomplete reply for %q finished as %q, want %q", reason, result.Finish, want)
		}
		if result.Text != "part of" || written != "part of" {
			t.Errorf("an incomplete reply lost its text: %q and %q", result.Text, written)
		}
		if result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 3 {
			t.Errorf("an incomplete reply reported the usage %+v, want 7 in and 3 out", result.Usage)
		}
	}
}

func TestTheCodexReaderReturnsTheMessageOfAnErrorEvent(t *testing.T) {
	for name, event := range map[string]string{
		"a flat error":   `{"type":"error","code":"usage_limit_reached","message":"the usage limit was reached"}`,
		"a nested error": `{"type":"error","error":{"code":"usage_limit_reached","message":"the usage limit was reached"}}`,
		"a failed reply": `{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"the usage limit was reached"}}}`,
	} {
		_, _, err := readCodex(t, codexStream(`{"type":"response.output_text.delta","delta":"so far"}`, event))

		if err == nil || !strings.Contains(err.Error(), "the usage limit was reached") {
			t.Errorf("%s came back as %v, want an error carrying the backend's message", name, err)
		}
	}
}

func TestTheCodexReaderSaysWhenAnErrorEventCarriesNoMessage(t *testing.T) {
	_, _, err := readCodex(t, codexStream(`{"type":"response.failed","response":{"status":"failed"}}`))

	if err == nil || !strings.Contains(err.Error(), "no reason") {
		t.Fatalf("a failure with no message came back as %v, want an error saying no reason was given", err)
	}
}

func TestTheCodexReaderRefusesAStreamThatEndsBeforeCompleted(t *testing.T) {
	for name, stream := range map[string]string{
		"a cut stream":           codexStream(`{"type":"response.output_text.delta","delta":"Hel"}`),
		"an empty stream":        "",
		"done before completed":  codexStream(`{"type":"response.output_text.delta","delta":"Hel"}`) + "data: [DONE]\n\n",
		"only the opening event": codexStream(`{"type":"response.created","response":{"id":"resp_1"}}`),
	} {
		_, _, err := readCodex(t, stream)

		if err == nil || !strings.Contains(err.Error(), "response.completed") {
			t.Errorf("%s came back as %v, want an error naming the missing completed event", name, err)
		}
	}
}

func TestTheCodexReaderRefusesALineThatIsNotJSON(t *testing.T) {
	_, _, err := readCodex(t, "data: {\"type\":\"response.output_text.delta\",\"delta\":\n\n"+codexStream(codexCompleted))

	if err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("a broken line came back as %v, want an error saying the line was not JSON", err)
	}
}

func TestTheCodexReaderRefusesALineLongerThanTheLimit(t *testing.T) {
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + strings.Repeat("x", maxEventLineBytes) + "\"}\n\n"

	_, _, err := readCodex(t, stream)

	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("a line past the cap came back as %v, want an error naming the limit", err)
	}
}

func TestTheCodexReaderRefusesTooManyToolCalls(t *testing.T) {
	events := []string{}
	for count := 0; count <= maxToolCallsPerReply; count++ {
		events = append(events, `{"type":"response.output_item.done","item":{"type":"function_call","name":"noop","arguments":"{}"}}`)
	}
	events = append(events, codexCompleted)

	_, _, err := readCodex(t, codexStream(events...))

	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("a reply with too many tool calls came back as %v, want an error naming the cap", err)
	}
}

func TestTheCodexReaderRefusesArgumentsLongerThanTheLimit(t *testing.T) {
	half := strings.Repeat("x", maxToolCallJSONBytes/2+1)
	stream := codexStream(
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"big"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"`+half+`"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"`+half+`"}`,
		codexCompleted,
	)

	_, _, err := readCodex(t, stream)

	if err == nil || !strings.Contains(err.Error(), "bytes of arguments") {
		t.Fatalf("arguments past the cap came back as %v, want an error naming the cap", err)
	}
}

func TestTheCodexReaderDropsArgumentPiecesForACallItNeverSaw(t *testing.T) {
	stream := codexStream(
		`{"type":"response.function_call_arguments.delta","item_id":"fc_ghost","delta":"{"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_ghost","arguments":"{}"}`,
		`{"type":"response.function_call_arguments.delta","delta":"{"}`,
		codexCompleted,
	)

	result, _, err := readCodex(t, stream)

	if err != nil {
		t.Fatalf("pieces for an unknown call failed the stream: %v", err)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("pieces for an unknown call became %d tool calls, want none", len(result.ToolCalls))
	}
}
