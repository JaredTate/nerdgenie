package testkit_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// streamEvent is one server-sent event read back off a stream: the name on the
// event line, and the object on the data line.
type streamEvent struct {
	name string
	data map[string]any
}

// readAnthropicStream splits an Anthropic event stream into its events, in
// order, so that a test can assert on the shape the wave 1 provider must parse.
func readAnthropicStream(t *testing.T, stream string) []streamEvent {
	t.Helper()
	events := []streamEvent{}
	for _, block := range strings.Split(stream, "\n\n") {
		name, data := "", ""
		for _, line := range strings.Split(block, "\n") {
			if rest, found := strings.CutPrefix(line, "event: "); found {
				name = rest
			}
			if rest, found := strings.CutPrefix(line, "data: "); found {
				data = rest
			}
		}
		if name == "" {
			continue
		}
		body := map[string]any{}
		if err := json.Unmarshal([]byte(data), &body); err != nil {
			t.Fatalf("the data line of the %s event is not JSON: %v\n%s", name, err, data)
		}
		events = append(events, streamEvent{name: name, data: body})
	}
	return events
}

// readOpenAIChunks splits a Chat Completions stream into its data lines, in
// order, leaving out the done marker.
func readOpenAIChunks(t *testing.T, stream string) []map[string]any {
	t.Helper()
	chunks := []map[string]any{}
	for _, line := range strings.Split(stream, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "data: ")
		if !found || rest == "[DONE]" {
			continue
		}
		chunk := map[string]any{}
		if err := json.Unmarshal([]byte(rest), &chunk); err != nil {
			t.Fatalf("a data line of the OpenAI stream is not JSON: %v\n%s", err, rest)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

// eventNames is the ordered list of event names in an Anthropic stream.
func eventNames(events []streamEvent) []string {
	names := []string{}
	for _, event := range events {
		names = append(names, event.name)
	}
	return names
}

// scriptWithTwoToolCalls is the script the wire tests run against: one reply
// with text, two tool calls, and a cache write to report.
func scriptWithTwoToolCalls() testkit.Script {
	return testkit.Script{
		Name:          "provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Text: "Reading the notes and the brand file.",
			ToolCalls: []contract.ToolCall{
				{ID: "call_1", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"memory/product.md"}`)},
				{ID: "call_2", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"memory/brand.md"}`)},
			},
			Finish:              contract.FinishToolCalls,
			Usage:               contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400},
			CacheCreationTokens: 700,
		}},
	}
}

func TestTheOpenAIStreamSendsEachToolCallsArgumentsInAtLeastTwoPieces(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()

	_, _, stream := postJSON(t, server.OpenAIAddress(), `{"messages":[]}`)

	pieces := map[float64][]string{}
	names := map[float64]string{}
	identifiers := map[float64]string{}
	for _, chunk := range readOpenAIChunks(t, stream) {
		for _, call := range toolCallsIn(t, chunk) {
			index, _ := call["index"].(float64)
			function, _ := call["function"].(map[string]any)
			if arguments, isText := function["arguments"].(string); isText && arguments != "" {
				pieces[index] = append(pieces[index], arguments)
			}
			if name, isText := function["name"].(string); isText && name != "" {
				names[index] = name
			}
			if identifier, isText := call["id"].(string); isText && identifier != "" {
				identifiers[index] = identifier
			}
		}
	}

	if len(pieces) != 2 {
		t.Fatalf("the stream carried %d tool calls, want the 2 the step asks for:\n%s", len(pieces), stream)
	}
	for index, sent := range pieces {
		if len(sent) < 2 {
			t.Errorf("tool call %v sent its arguments in %d piece, and wave 1 must accumulate at least 2:\n%s",
				index, len(sent), stream)
		}
	}
	if strings.Join(pieces[0], "") != `{"path":"memory/product.md"}` {
		t.Errorf("the pieces of the first tool call join to %q, want the whole arguments", strings.Join(pieces[0], ""))
	}
	if names[0] != contract.ToolRead || identifiers[0] != "call_1" {
		t.Errorf("the first tool call arrived as name %q and id %q, want the name and the id on the first delta",
			names[0], identifiers[0])
	}
}

func TestTheOpenAIStreamSendsUsageInAFinalChunkWithNoChoices(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()

	_, _, stream := postJSON(t, server.OpenAIAddress(), `{"messages":[]}`)

	chunks := readOpenAIChunks(t, stream)
	if len(chunks) == 0 {
		t.Fatalf("the stream carried no data lines at all:\n%s", stream)
	}
	for at, chunk := range chunks[:len(chunks)-1] {
		if _, carried := chunk["usage"]; carried {
			t.Errorf("chunk %d carries the usage, and it belongs on a final chunk of its own:\n%s", at, stream)
		}
	}

	last := chunks[len(chunks)-1]
	usage, carried := last["usage"].(map[string]any)
	if !carried {
		t.Fatalf("the last chunk carries no usage:\n%s", stream)
	}
	if choices, _ := last["choices"].([]any); len(choices) != 0 {
		t.Errorf("the usage chunk carries %d choices, and wave 1 reads usage off a chunk whose choices are empty:\n%s",
			len(choices), stream)
	}
	if usage["prompt_tokens"] != float64(6100) || usage["completion_tokens"] != float64(400) {
		t.Errorf("the usage chunk says %v, want the step's counts", usage)
	}
	if !strings.HasSuffix(strings.TrimSpace(stream), "data: [DONE]") {
		t.Errorf("the stream does not end with the done marker:\n%s", stream)
	}
}

func TestTheAnthropicStreamSendsPingEventsBetweenBlocks(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()

	_, _, stream := postJSON(t, server.AnthropicAddress(), `{"messages":[]}`)

	names := eventNames(readAnthropicStream(t, stream))
	gaps := 0
	for at, name := range names {
		if name != "content_block_stop" {
			continue
		}
		rest := names[at+1:]
		next := indexOf(rest, "content_block_start")
		if next < 0 {
			continue
		}
		gaps++
		if indexOf(rest[:next], "ping") < 0 {
			t.Errorf("there is no ping between the block that stops at event %d and the next one:\n%v", at, names)
		}
	}
	if gaps == 0 {
		t.Fatalf("the stream has no two blocks to ping between, so the script is wrong:\n%v", names)
	}
}

func TestTheAnthropicStreamReportsTheCacheCreationTokensTheStepAsksFor(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()

	_, _, stream := postJSON(t, server.AnthropicAddress(), `{"messages":[]}`)

	for _, event := range readAnthropicStream(t, stream) {
		var usage map[string]any
		switch event.name {
		case "message_start":
			message, _ := event.data["message"].(map[string]any)
			usage, _ = message["usage"].(map[string]any)
		case "message_delta":
			usage, _ = event.data["usage"].(map[string]any)
		default:
			continue
		}
		written, carried := usage["cache_creation_input_tokens"]
		if !carried {
			t.Errorf("the %s event has no cache_creation_input_tokens, and wave 1 adds it to the input count: %v",
				event.name, usage)
			continue
		}
		if written != float64(700) {
			t.Errorf("the %s event says %v cache creation tokens, want the 700 the step asks for", event.name, written)
		}
	}
}

func TestTheAnthropicStreamCountsTheInputThreeWaysThatAddBackUp(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptWithTwoToolCalls())
	defer server.Close()

	_, _, stream := postJSON(t, server.AnthropicAddress(), `{"messages":[]}`)

	for _, event := range readAnthropicStream(t, stream) {
		usage := usageIn(event)
		if usage == nil {
			continue
		}
		// On the real Messages API input_tokens is what was read fresh: the cache
		// read and the cache write are counted separately, and the three add up to
		// everything the model read. A fake that put the whole count in the plain
		// field would make one script report a different total on each wire.
		plain, _ := usage["input_tokens"].(float64)
		read, _ := usage["cache_read_input_tokens"].(float64)
		written, _ := usage["cache_creation_input_tokens"].(float64)

		if plain != float64(6100-5200-700) {
			t.Errorf("the %s event says %v input tokens, want what was read fresh rather than the whole count",
				event.name, plain)
		}
		if plain+read+written != float64(6100) {
			t.Errorf("the %s event's three counts add up to %v, want the script's whole input count of 6100",
				event.name, plain+read+written)
		}
	}
}

func TestTheAnthropicStreamNeverReportsANegativeInputCount(t *testing.T) {
	script := scriptWithTwoToolCalls()
	// A script whose cached count is bigger than its whole input count is not a
	// count any real provider would send, and the remainder must not go below
	// zero.
	script.Steps[0].Usage = contract.Usage{InputTokens: 900, CachedInputTokens: 5200, OutputTokens: 64}
	server := testkit.NewFakeProviderServer(script)
	defer server.Close()

	_, _, stream := postJSON(t, server.AnthropicAddress(), `{"messages":[]}`)

	for _, event := range readAnthropicStream(t, stream) {
		usage := usageIn(event)
		if usage == nil {
			continue
		}
		if plain, _ := usage["input_tokens"].(float64); plain < 0 {
			t.Errorf("the %s event says %v input tokens, and a token count is never negative", event.name, plain)
		}
	}
}

// usageIn is the token count one event carries, or nil when it carries none.
func usageIn(event streamEvent) map[string]any {
	switch event.name {
	case "message_start":
		message, _ := event.data["message"].(map[string]any)
		usage, _ := message["usage"].(map[string]any)
		return usage
	case "message_delta":
		usage, _ := event.data["usage"].(map[string]any)
		return usage
	default:
		return nil
	}
}

func TestAStepThatMisbehavesMidStreamSendsAnErrorEventAndStops(t *testing.T) {
	script := scriptWithTwoToolCalls()
	script.Steps[0].MidStreamError = "the provider is overloaded, so try again"
	script.Steps = append(script.Steps, script.Steps[0])
	server := testkit.NewFakeProviderServer(script)
	defer server.Close()

	_, _, anthropic := postJSON(t, server.AnthropicAddress(), `{"messages":[]}`)

	names := eventNames(readAnthropicStream(t, anthropic))
	if indexOf(names, "error") < 0 {
		t.Errorf("a step that misbehaves mid-stream sent no error event:\n%v", names)
	}
	if indexOf(names, "message_stop") >= 0 {
		t.Errorf("the stream that errored mid-way still ended properly:\n%v", names)
	}
	if !strings.Contains(anthropic, "the provider is overloaded") {
		t.Errorf("the error event does not carry the step's message:\n%s", anthropic)
	}

	_, _, openAI := postJSON(t, server.OpenAIAddress(), `{"messages":[]}`)

	if !strings.Contains(openAI, `"error"`) {
		t.Errorf("the Chat Completions stream sent no error line mid-way:\n%s", openAI)
	}
	if strings.Contains(openAI, "[DONE]") {
		t.Errorf("the Chat Completions stream that errored mid-way still ended properly:\n%s", openAI)
	}
}

// toolCallsIn returns the tool-call deltas one Chat Completions chunk carries.
func toolCallsIn(t *testing.T, chunk map[string]any) []map[string]any {
	t.Helper()
	choices, _ := chunk["choices"].([]any)
	found := []map[string]any{}
	for _, choice := range choices {
		asObject, _ := choice.(map[string]any)
		delta, _ := asObject["delta"].(map[string]any)
		calls, _ := delta["tool_calls"].([]any)
		for _, call := range calls {
			if typed, isObject := call.(map[string]any); isObject {
				found = append(found, typed)
			}
		}
	}
	return found
}

// indexOf is the position of a name in a list, or minus one when it is absent.
func indexOf(names []string, wanted string) int {
	for at, name := range names {
		if name == wanted {
			return at
		}
	}
	return -1
}

// scriptWithEveryFinishReason has one step per finish reason the contract
// names, so that both wire protocols can be asked for all four words.
func scriptWithEveryFinishReason() testkit.Script {
	return testkit.Script{
		Name:          "provider",
		ContextLength: 200000,
		Steps: []testkit.Step{
			{Text: "That is done.", Finish: contract.FinishEnd},
			{
				Text:      "Reading the notes.",
				ToolCalls: []contract.ToolCall{{ID: "call_1", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"a.md"}`)}},
				Finish:    contract.FinishToolCalls,
			},
			{Text: "This reply ran out of room", Finish: contract.FinishLength},
			{Text: "I will not answer that", Finish: contract.FinishStopped},
		},
	}
}

func TestEveryFinishReasonComesOffTheWireInEachApisOwnWord(t *testing.T) {
	wanted := []struct {
		finish    contract.FinishReason
		anthropic string
		openAI    string
	}{
		{contract.FinishEnd, "end_turn", "stop"},
		{contract.FinishToolCalls, "tool_use", "tool_calls"},
		{contract.FinishLength, "max_tokens", "length"},
		{contract.FinishStopped, "refusal", "content_filter"},
	}

	anthropic := testkit.NewFakeProviderServer(scriptWithEveryFinishReason())
	defer anthropic.Close()
	openAI := testkit.NewFakeProviderServer(scriptWithEveryFinishReason())
	defer openAI.Close()

	for _, row := range wanted {
		t.Run(string(row.finish), func(t *testing.T) {
			_, _, stream := postJSON(t, anthropic.AnthropicAddress(), `{"messages":[]}`)
			if said := anthropicStopReason(t, stream); said != row.anthropic {
				t.Errorf("the Messages API said %q for the finish reason %q, want %q", said, row.finish, row.anthropic)
			}

			_, _, stream = postJSON(t, openAI.OpenAIAddress(), `{"messages":[]}`)
			if said := openAIFinishReason(t, stream); said != row.openAI {
				t.Errorf("the Chat Completions API said %q for the finish reason %q, want %q", said, row.finish, row.openAI)
			}
		})
	}
}

// anthropicStopReason reads the stop reason off the message_delta event.
func anthropicStopReason(t *testing.T, stream string) string {
	t.Helper()
	for _, event := range readAnthropicStream(t, stream) {
		if event.name != "message_delta" {
			continue
		}
		delta, _ := event.data["delta"].(map[string]any)
		reason, _ := delta["stop_reason"].(string)
		return reason
	}
	t.Fatalf("the Messages API stream carried no message_delta event:\n%s", stream)
	return ""
}

// openAIFinishReason reads the finish reason off the chunk that carries one.
func openAIFinishReason(t *testing.T, stream string) string {
	t.Helper()
	for _, chunk := range readOpenAIChunks(t, stream) {
		choices, _ := chunk["choices"].([]any)
		for _, choice := range choices {
			asObject, _ := choice.(map[string]any)
			if reason, isText := asObject["finish_reason"].(string); isText && reason != "" {
				return reason
			}
		}
	}
	t.Fatalf("the Chat Completions stream carried no finish reason:\n%s", stream)
	return ""
}
