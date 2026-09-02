package provider

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// collect reads a stream written by hand and returns the reply, the deltas
// joined, and the error.
func collect(t *testing.T, reader string) (contract.Reply, string, error) {
	t.Helper()
	streamed := strings.Builder{}
	reply, err := parseOpenAIStream(strings.NewReader(reader), "local", Options{}, func(delta string) {
		streamed.WriteString(delta)
	})
	return reply, streamed.String(), err
}

func TestTheOpenAIReaderJoinsToolArgumentsThatArrivedInPieces(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_9","type":"function","function":{"name":"read","arguments":"{"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"path\":\""}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"notes.md\"}"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":300,"completion_tokens":25,"prompt_tokens_details":{"cached_tokens":128}}}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n"

	reply, streamed, err := collect(t, stream)

	if err != nil {
		t.Fatalf("reading a stream the local daemon could send failed: %v", err)
	}
	if len(reply.ToolCalls) != 1 || string(reply.ToolCalls[0].Input) != `{"path":"notes.md"}` {
		t.Fatalf("the pieces did not join into one call: %+v", reply.ToolCalls)
	}
	if reply.ToolCalls[0].ID != "call_9" || reply.ToolCalls[0].Name != contract.ToolRead {
		t.Errorf("the call is %+v, want call_9 asking for read", reply.ToolCalls[0])
	}
	if streamed != "" {
		t.Errorf("the arguments were streamed as text deltas, and only text belongs there: %q", streamed)
	}
	want := contract.Usage{InputTokens: 300, CachedInputTokens: 128, OutputTokens: 25}
	if reply.Usage != want {
		t.Errorf("the usage came back as %+v, want %+v from the chunk with no choices at all", reply.Usage, want)
	}
}

func TestTheOpenAIReaderSaysWhenTheStreamStoppedWithoutAFinishReason(t *testing.T) {
	stream := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"half a th\"},\"finish_reason\":null}]}\n\n"

	_, _, err := collect(t, stream)

	if err == nil {
		t.Fatal("a stream that stopped part way through came back as a good reply")
	}
	if !strings.Contains(err.Error(), "local") {
		t.Errorf("the error does not name the model: %v", err)
	}
}

func TestTheOpenAIReaderSkipsLinesThatAreNotJSON(t *testing.T) {
	stream := ": keep alive\n\ndata: not json at all\n\nevent: ping\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fine\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

	reply, streamed, err := collect(t, stream)

	if err != nil {
		t.Fatalf("a stream with junk in it failed: %v", err)
	}
	if reply.Text != "fine" || streamed != "fine" {
		t.Errorf("the reply is %q and the deltas are %q, want the one good chunk's text", reply.Text, streamed)
	}
}

func TestTheOpenAIReaderReportsAnErrorTheServerSentInTheStream(t *testing.T) {
	stream := "data: {\"error\":{\"message\":\"the slot ran out of room\"}}\n\n"

	_, _, err := collect(t, stream)

	if err == nil || !strings.Contains(err.Error(), "the slot ran out of room") {
		t.Fatalf("an error sent inside the stream came back as %v", err)
	}
}

func TestTheAnthropicReaderKeepsTheCountsThatArriveInTwoPlaces(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":900,"cache_read_input_tokens":5200,"cache_creation_input_tokens":300,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":64}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"

	reply, err := parseAnthropicStream(strings.NewReader(stream), "opus", Options{}, nil)

	if err != nil {
		t.Fatalf("reading a Messages API stream failed: %v", err)
	}
	want := contract.Usage{InputTokens: 1200, CachedInputTokens: 5200, OutputTokens: 64}
	if reply.Usage != want {
		t.Errorf("the usage came back as %+v, want %+v with the cache-creation tokens added to the input", reply.Usage, want)
	}
	if reply.Finish != contract.FinishLength {
		t.Errorf("the reply finished with %q, want %q", reply.Finish, contract.FinishLength)
	}
}

func TestTheAnthropicReaderReadsARefusalAsStopped(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"refusal"},"usage":{"output_tokens":2}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"
	said := []string{}

	reply, err := parseAnthropicStream(strings.NewReader(stream), "opus",
		Options{Log: func(line string) { said = append(said, line) }}, nil)

	if err != nil {
		t.Fatalf("reading a Messages API stream failed: %v", err)
	}
	if reply.Finish != contract.FinishStopped {
		t.Errorf("a refusal finished with %q, want %q", reply.Finish, contract.FinishStopped)
	}
	if len(said) != 1 || !strings.Contains(said[0], "refusal") {
		t.Errorf("the reason the model gave was not written down: %v", said)
	}
}

func TestTheAnthropicReaderRefusesALineLongerThanTheLimit(t *testing.T) {
	stream := "data: {\"type\":\"message_start\",\"message\":{\"note\":\"" + strings.Repeat("x", maxEventLineBytes) + "\"}}\n\n"

	_, err := parseAnthropicStream(strings.NewReader(stream), "opus", Options{}, nil)

	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("a line past the cap came back as %v, want an error naming the limit", err)
	}
}
