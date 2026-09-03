package provider

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// The seeds below are one good answer and several broken ones for each thing
// this package parses. What is asserted of every input, good or not, is the
// same: the parser comes back with a reply or an error, it never panics, and it
// never takes longer than the stall timeout, because a parser that can spin is a
// parser that can hang the whole agent.

// checkPromptly runs a parser and fails when it took longer than the stall
// timeout, which is the longest the harness ever waits on a stream.
func checkPromptly(t *testing.T, name string, parse func()) {
	t.Helper()
	started := time.Now()
	parse()
	if taken := time.Since(started); taken > stallTimeout {
		t.Fatalf("%s took %s on one input, and nothing may take longer than the stall timeout of %s", name, taken, stallTimeout)
	}
}

func FuzzAnthropicStreamParser(f *testing.F) {
	f.Add(strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n")
	f.Add("data: {\"type\":\"ping\"}\n\n")
	f.Add("data: {\"type\":\"error\",\"error\":{\"message\":\"broken\"}}\n\n")
	f.Add("data: {\"type\":\"content_block_delta\",\"index\":9,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\"}}\n\n")
	f.Add("event: message_stop\n\n")
	f.Add("")
	f.Add("data:")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, stream string) {
		checkPromptly(t, "the Anthropic stream parser", func() {
			written := strings.Builder{}
			reply, err := parseAnthropicStream(strings.NewReader(stream), "a model", Options{}, func(delta string) {
				written.WriteString(delta)
			})
			if err == nil && reply.Text != written.String() {
				t.Fatalf("the deltas joined to %q and the reply is %q, and the two must always be the same",
					written.String(), reply.Text)
			}
		})
	})
}

func FuzzOpenAIStreamParser(f *testing.F) {
	f.Add(strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":1}}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n")
	f.Add("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"arguments\":\"{\"}}]}}]}\n\n")
	f.Add("data: [DONE]\n\n")
	f.Add(": a comment\n\n")
	f.Add("")
	f.Add("data: {\"choices\":[{\"delta\":{\"content\":null},\"finish_reason\":\"made up\"}]}\n\n")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, stream string) {
		checkPromptly(t, "the OpenAI stream parser", func() {
			written := strings.Builder{}
			reply, err := parseOpenAIStream(strings.NewReader(stream), "a model", Options{}, func(delta string) {
				written.WriteString(delta)
			})
			if err == nil && reply.Text != written.String() {
				t.Fatalf("the deltas joined to %q and the reply is %q, and the two must always be the same",
					written.String(), reply.Text)
			}
		})
	})
}

func FuzzCodexStreamParser(f *testing.F) {
	f.Add(strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"read","arguments":""}}`,
		``,
		`event: response.function_call_arguments.delta`,
		`data: {"type":"response.function_call_arguments.delta","delta":"{\"path\":\"a\"}","item_id":"fc_1"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"hi","item_id":"msg_1"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":3,"input_tokens_details":{"cached_tokens":1},"output_tokens":1}}}`,
		``,
	}, "\n") + "\n")
	f.Add("data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"fc_9\",\"type\":\"function_call\",\"arguments\":\"{\"}}\n\n")
	f.Add("data: {\"type\":\"response.incomplete\",\"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n")
	f.Add("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"broken\"}}}\n\n")
	f.Add("data: {\"type\":\"error\",\"message\":\"broken\"}\n\n")
	f.Add("data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{\",\"item_id\":\"\"}\n\n")
	f.Add("event: response.completed\n\n")
	f.Add("")
	f.Add("data:")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, stream string) {
		checkPromptly(t, "the Codex stream parser", func() {
			written := strings.Builder{}
			reply, err := parseCodexStream(strings.NewReader(stream), "a model", Options{}, func(delta string) {
				written.WriteString(delta)
			})
			if err == nil && reply.Text != written.String() {
				t.Fatalf("the deltas joined to %q and the reply is %q, and the two must always be the same",
					written.String(), reply.Text)
			}
			if err == nil && reply.Usage.CachedInputTokens > reply.Usage.InputTokens {
				t.Fatalf("the reply says %d of %d input tokens were cached, and the cached count is a part of the input count",
					reply.Usage.CachedInputTokens, reply.Usage.InputTokens)
			}
		})
	})
}

func FuzzClaudeOutputParser(f *testing.F) {
	f.Add(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}}
{"type":"result","subtype":"success","is_error":false,"result":"hi","total_cost_usd":0.001,"usage":{"input_tokens":10,"output_tokens":1}}
`)
	f.Add(`{"type":"result","is_error":true,"result":"usage limit reached"}` + "\n")
	f.Add("not json at all\n")
	f.Add("")
	f.Add("{\n")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, printed string) {
		checkPromptly(t, "the claude output parser", func() {
			written := strings.Builder{}
			found := parseClaudeOutput(strings.NewReader(printed), func(delta string) { written.WriteString(delta) })
			if found.text != written.String() {
				t.Fatalf("the deltas joined to %q and the text is %q, and the two must always be the same",
					written.String(), found.text)
			}
		})
	})
}

func FuzzCodexOutputParser(f *testing.F) {
	f.Add(`{"type":"thread.started","thread_id":"x"}
{"type":"item.completed","item":{"type":"agent_message","text":"hi"}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":1}}
`)
	f.Add(`{"type":"error","message":"the prompt is too long"}` + "\n")
	f.Add("Reading prompt from stdin...\n")
	f.Add("")
	f.Add("{\"type\":\"item.completed\",\"item\":{\"type\":\"reasoning\"}}\n")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, printed string) {
		checkPromptly(t, "the codex output parser", func() {
			written := strings.Builder{}
			found := parseCodexOutput(strings.NewReader(printed), func(delta string) { written.WriteString(delta) })
			if found.text != written.String() {
				t.Fatalf("the deltas joined to %q and the text is %q, and the two must always be the same",
					written.String(), found.text)
			}
		})
	})
}

func FuzzLocalServerProps(f *testing.F) {
	f.Add(`{"default_generation_settings":{"n_ctx":262144},"model_alias":"local-coder"}`)
	f.Add(`{"default_generation_settings":null}`)
	f.Add(`{"default_generation_settings":{"n_ctx":-5}}`)
	f.Add(`{"models":[]}`)
	f.Add("")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, body string) {
		checkPromptly(t, "the props reader", func() {
			found, answered := readProps([]byte(body))
			if !answered && found.contextLength != 0 {
				t.Fatalf("a server that did not answer reported a window of %d", found.contextLength)
			}
		})
	})
}

func FuzzRefusalBody(f *testing.F) {
	f.Add(`{"error":{"message":"prompt is too long: 300000 tokens > 200000 maximum","type":"invalid_request_error"}}`)
	f.Add(`{"message":"too many requests"}`)
	f.Add(`{"error":{}}`)
	f.Add("plain words")
	f.Add("")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, body string) {
		checkPromptly(t, "the refusal reader", func() {
			said := messageFromBody([]byte(body))
			if said == "" {
				t.Fatal("a refusal was read as an empty message, and every error must say something")
			}
			if len(said) > maxErrorBodyBytes+len("  ") {
				t.Fatalf("a refusal was read as %d bytes, and the cap is %d", len(said), maxErrorBodyBytes)
			}
			_ = looksLikeOverflow(said)
			_ = bytes.Contains([]byte(body), []byte("x"))
		})
	})
}
