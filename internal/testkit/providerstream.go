package testkit

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// writeStream sends one step of the script in the shape the path asks for. When
// dropEarly is true it stops part way through, without ending the stream, which
// is what a broken connection looks like to the caller.
func (provider *FakeProviderServer) writeStream(writer http.ResponseWriter, path string, step Step, dropEarly bool) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.WriteHeader(http.StatusOK)

	flush, canFlush := writer.(http.Flusher)
	send := func(line string) {
		fmt.Fprint(writer, line)
		if canFlush {
			flush.Flush()
		}
	}

	if path == AnthropicPath {
		writeAnthropicStream(send, step, dropEarly)
		return
	}
	writeOpenAIStream(send, step, dropEarly)
}

// writeAnthropicStream writes the Messages API's event stream: a start with the
// token counts on it, a ping between every pair of blocks, one block per piece
// of the reply, and a stop. A step that misbehaves mid-stream sends an error
// event after the text block and stops there instead.
func writeAnthropicStream(send func(line string), step Step, dropEarly bool) {
	send(anthropicEvent("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_fake", "type": "message", "role": "assistant", "content": []any{},
			"usage": anthropicUsage(step, 0),
		},
	}))
	if dropEarly {
		return
	}
	send(anthropicPing())

	writeAnthropicText(send, step)
	if step.MidStreamError != "" {
		send(anthropicEvent("error", map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "overloaded_error", "message": step.MidStreamError},
		}))
		return
	}
	writeAnthropicToolCalls(send, step)

	send(anthropicEvent("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": finishReasonFor(step, true)},
		"usage": anthropicUsage(step, step.Usage.OutputTokens),
	}))
	send(anthropicEvent("message_stop", map[string]any{"type": "message_stop"}))
}

// anthropicUsage is the token count the Messages API reports, with the cache
// creation count the step asks for. Wave 1's brief adds that count to the input
// tokens, so a fake that always says zero would let a harness that ignores it
// pass.
func anthropicUsage(step Step, outputTokens int) map[string]any {
	return map[string]any{
		"input_tokens":                step.Usage.InputTokens,
		"cache_read_input_tokens":     step.Usage.CachedInputTokens,
		"cache_creation_input_tokens": step.CacheCreationTokens,
		"output_tokens":               outputTokens,
	}
}

// anthropicPing is the event the Messages API sends to keep the connection
// warm. It carries nothing, and a parser that chokes on it is broken.
func anthropicPing() string {
	return anthropicEvent("ping", map[string]any{"type": "ping"})
}

// writeAnthropicText writes the text block of a reply, in deltas.
func writeAnthropicText(send func(line string), step Step) {
	send(anthropicEvent("content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	}))
	for _, delta := range splitIntoDeltas(step.Text, deltasPerReply) {
		send(anthropicEvent("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": delta},
		}))
	}
	send(anthropicEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}))
}

// writeAnthropicToolCalls writes one tool-use block per tool call, with a ping
// before each one and its arguments arriving as input-JSON deltas the way the
// real API sends them.
func writeAnthropicToolCalls(send func(line string), step Step) {
	for at, call := range step.ToolCalls {
		index := at + 1
		send(anthropicPing())
		send(anthropicEvent("content_block_start", map[string]any{
			"type": "content_block_start", "index": index,
			"content_block": map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": map[string]any{}},
		}))
		for _, delta := range argumentPieces(call.Input) {
			send(anthropicEvent("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": index,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": delta},
			}))
		}
		send(anthropicEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": index}))
	}
}

// writeOpenAIStream writes the Chat Completions stream: data lines carrying the
// text deltas, then the tool calls in pieces, then the finish reason, then a
// final chunk carrying only the usage, then the done marker. A step that
// misbehaves mid-stream sends an error line after the text and stops there.
func writeOpenAIStream(send func(line string), step Step, dropEarly bool) {
	for _, delta := range splitIntoDeltas(step.Text, deltasPerReply) {
		send(openAIChunk(map[string]any{
			"index":         0,
			"delta":         map[string]any{"role": "assistant", "content": delta},
			"finish_reason": nil,
		}))
	}
	if dropEarly {
		return
	}
	if step.MidStreamError != "" {
		send(openAIDataLine(map[string]any{
			"error": map[string]any{"message": step.MidStreamError, "type": "api_error"},
		}))
		return
	}

	writeOpenAIToolCalls(send, step)
	send(openAIChunk(map[string]any{
		"index": 0, "delta": map[string]any{}, "finish_reason": finishReasonFor(step, false),
	}))
	send(openAIUsageChunk(step))
	send("data: [DONE]\n\n")
}

// writeOpenAIToolCalls writes each tool call's arguments in more than one piece,
// the way a real stream sends them: the first delta carries the identifier and
// the name, and every delta carries the next piece of the arguments string. Wave
// 1's brief requires the provider to accumulate these by index, and a fake that
// sent the whole string at once would let a provider that never accumulates
// pass.
func writeOpenAIToolCalls(send func(line string), step Step) {
	for at, call := range step.ToolCalls {
		for piece, arguments := range argumentPieces(call.Input) {
			function := map[string]any{"arguments": arguments}
			delta := map[string]any{"index": at, "function": function}
			if piece == 0 {
				delta["id"] = call.ID
				delta["type"] = "function"
				function["name"] = call.Name
			}
			send(openAIChunk(map[string]any{
				"index":         0,
				"delta":         map[string]any{"tool_calls": []any{delta}},
				"finish_reason": nil,
			}))
		}
	}
}

// argumentPieces cuts one tool call's arguments into the two pieces a real
// stream sends them in. A call with no arguments still sends one piece, so that
// its identifier and its name reach the caller.
func argumentPieces(input json.RawMessage) []string {
	pieces := splitIntoDeltas(string(input), 2)
	if len(pieces) == 0 {
		return []string{""}
	}
	return pieces
}

// anthropicEvent formats one server-sent event the way the Messages API does,
// with the event name on its own line above the data.
func anthropicEvent(name string, body map[string]any) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, mustJSON(body))
}

// openAIChunk formats one data line of a Chat Completions stream carrying one
// choice.
func openAIChunk(choice map[string]any) string {
	return openAIDataLine(map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion.chunk",
		"choices": []any{choice},
	})
}

// openAIUsageChunk formats the final data line, which carries the token counts
// and an empty list of choices. That is what OpenAI and llama-server send when
// stream_options asks for the usage, and it is the shape wave 1's brief tells
// the provider to read.
func openAIUsageChunk(step Step) string {
	return openAIDataLine(map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion.chunk",
		"choices": []any{},
		"usage": map[string]any{
			"prompt_tokens":         step.Usage.InputTokens,
			"completion_tokens":     step.Usage.OutputTokens,
			"total_tokens":          step.Usage.InputTokens + step.Usage.OutputTokens,
			"prompt_tokens_details": map[string]any{"cached_tokens": step.Usage.CachedInputTokens},
		},
	})
}

// openAIDataLine formats one data line of a Chat Completions stream.
func openAIDataLine(chunk map[string]any) string {
	return fmt.Sprintf("data: %s\n\n", mustJSON(chunk))
}

// mustJSON turns a value into JSON, and returns an error object rather than
// failing, because a fake server that panics tells a test nothing useful.
func mustJSON(value any) string {
	written, err := json.Marshal(value)
	if err != nil {
		return `{"type":"error","error":{"message":"the fake provider could not write this event as JSON"}}`
	}
	return string(written)
}
