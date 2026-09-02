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
// token counts on it, one block per piece of the reply, and a stop.
func writeAnthropicStream(send func(line string), step Step, dropEarly bool) {
	send(anthropicEvent("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_fake", "type": "message", "role": "assistant", "content": []any{},
			"usage": map[string]any{
				"input_tokens":                step.Usage.InputTokens,
				"cache_read_input_tokens":     step.Usage.CachedInputTokens,
				"cache_creation_input_tokens": 0,
				"output_tokens":               0,
			},
		},
	}))
	if dropEarly {
		return
	}

	writeAnthropicText(send, step)
	writeAnthropicToolCalls(send, step)

	send(anthropicEvent("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": finishReasonFor(step, true)},
		"usage": map[string]any{
			"input_tokens":            step.Usage.InputTokens,
			"cache_read_input_tokens": step.Usage.CachedInputTokens,
			"output_tokens":           step.Usage.OutputTokens,
		},
	}))
	send(anthropicEvent("message_stop", map[string]any{"type": "message_stop"}))
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

// writeAnthropicToolCalls writes one tool-use block per tool call, with its
// arguments arriving as input-JSON deltas the way the real API sends them.
func writeAnthropicToolCalls(send func(line string), step Step) {
	for at, call := range step.ToolCalls {
		index := at + 1
		send(anthropicEvent("content_block_start", map[string]any{
			"type": "content_block_start", "index": index,
			"content_block": map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": map[string]any{}},
		}))
		for _, delta := range splitIntoDeltas(string(call.Input), 2) {
			send(anthropicEvent("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": index,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": delta},
			}))
		}
		send(anthropicEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": index}))
	}
}

// writeOpenAIStream writes the Chat Completions stream: data lines carrying the
// deltas, then the finish reason with the usage, then the done marker.
func writeOpenAIStream(send func(line string), step Step, dropEarly bool) {
	for _, delta := range splitIntoDeltas(step.Text, deltasPerReply) {
		send(openAIChunk(map[string]any{
			"index":         0,
			"delta":         map[string]any{"role": "assistant", "content": delta},
			"finish_reason": nil,
		}, nil))
	}
	if dropEarly {
		return
	}

	for at, call := range step.ToolCalls {
		send(openAIChunk(map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": at, "id": call.ID, "type": "function",
				"function": map[string]any{"name": call.Name, "arguments": string(call.Input)},
			}}},
			"finish_reason": nil,
		}, nil))
	}

	send(openAIChunk(
		map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finishReasonFor(step, false)},
		map[string]any{
			"prompt_tokens":         step.Usage.InputTokens,
			"completion_tokens":     step.Usage.OutputTokens,
			"total_tokens":          step.Usage.InputTokens + step.Usage.OutputTokens,
			"prompt_tokens_details": map[string]any{"cached_tokens": step.Usage.CachedInputTokens},
		},
	))
	send("data: [DONE]\n\n")
}

// anthropicEvent formats one server-sent event the way the Messages API does,
// with the event name on its own line above the data.
func anthropicEvent(name string, body map[string]any) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, mustJSON(body))
}

// openAIChunk formats one data line of a Chat Completions stream.
func openAIChunk(choice map[string]any, usage map[string]any) string {
	chunk := map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion.chunk",
		"choices": []any{choice},
	}
	if usage != nil {
		chunk["usage"] = usage
	}
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
