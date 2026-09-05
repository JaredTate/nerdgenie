// The way one tool call is put back together from deltas keyed by their index,
// and the usage fields this reader takes, were read from Prime Agent's
// chat-completions provider at
// ~/Code/prime-agent/packages/ai/src/providers/openai-completions.ts.

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// openAICount is the token count the server reports in the last chunk of the
// stream, which it only sends because the request asked for it.
type openAICount struct {
	// PromptTokens is the whole prompt, including whatever came from the cache.
	PromptTokens int `json:"prompt_tokens"`
	// CompletionTokens is what the model wrote.
	CompletionTokens int `json:"completion_tokens"`
	// PromptTokensDetails holds the part of the prompt that came from the cache.
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// openAITimings is llama-server's own account of the last call, on the final
// chunk of the stream.
type openAITimings struct {
	// CacheN is how many prompt tokens the daemon read from its cache.
	CacheN int `json:"cache_n"`
	// PromptN is how many prompt tokens the daemon had to process.
	PromptN int `json:"prompt_n"`
}

// openAIDeltaCall is one piece of one tool call. The identifier and the name
// arrive on the first piece and the arguments in pieces after it.
type openAIDeltaCall struct {
	// Index says which call of the reply this piece belongs to.
	Index *int `json:"index"`
	// ID is the identifier the server gave the call.
	ID string `json:"id"`
	// Function carries the name and the piece of the arguments.
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAIChunk is one data line of a Chat Completions stream.
type openAIChunk struct {
	// Choices is the reply's pieces. The last chunk of a stream carries the
	// usage and no choices at all, which is what the local daemon sends.
	Choices []struct {
		Delta struct {
			Content   string            `json:"content"`
			ToolCalls []openAIDeltaCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Usage is the token count, on the last chunk only.
	Usage *openAICount `json:"usage"`
	// Timings is what llama-server adds to the last chunk: its own count of
	// the prompt tokens it read from its cache and the ones it had to process.
	// A live run showed the usage field's cached count standing still at 5.8k
	// for twelve rounds while the daemon's log showed it had reused ninety
	// thousand, so when the timings are there they say what was reused.
	Timings *openAITimings `json:"timings"`
	// Error is what a server says when it gives up part way through the stream.
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// openAIReply is the reply as it is being built from the stream.
type openAIReply struct {
	modelName string
	onDelta   func(delta string)
	text      bytes.Buffer
	calls     map[int]*toolCallBuild
	order     []int
	usage     contract.Usage
	finish    string
	sawFinish bool
}

// readOpenAIStream reads one whole answer, turning a call the stall watch cut
// short into the stalled-stream sentinel.
func readOpenAIStream(stream *openStream, modelName string, options Options,
	onDelta func(delta string)) (contract.Reply, error) {
	reply, err := parseOpenAIStream(stream.reader, modelName, options, onDelta)
	if err != nil && stream.watch.stalled.Load() {
		return contract.Reply{}, stream.watch.explain(modelName, err)
	}
	return reply, err
}

// parseOpenAIStream turns the bytes of a Chat Completions stream into one reply,
// handing every piece of text to the delta function as it arrives.
func parseOpenAIStream(reader io.Reader, modelName string, options Options,
	onDelta func(delta string)) (contract.Reply, error) {
	building := &openAIReply{modelName: modelName, onDelta: onDelta, calls: map[int]*toolCallBuild{}}
	err := forEachDataLine(reader, func(payload []byte) (bool, error) {
		chunk := openAIChunk{}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			return false, nil
		}
		return building.take(chunk)
	})
	if err != nil {
		return contract.Reply{}, err
	}
	if !building.sawFinish {
		return contract.Reply{}, providerError{
			modelName: modelName,
			message:   "the stream ended before the model said why it stopped, so the reply is incomplete",
			retryable: true,
		}
	}
	return building.finished(options), nil
}

// take answers one chunk of the stream.
func (building *openAIReply) take(chunk openAIChunk) (bool, error) {
	if chunk.Error.Message != "" {
		return true, fmt.Errorf("the model %q stopped with an error: %s", building.modelName, chunk.Error.Message)
	}
	if chunk.Usage != nil {
		building.usage = contract.Usage{
			InputTokens:       chunk.Usage.PromptTokens,
			CachedInputTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:      chunk.Usage.CompletionTokens,
		}
		if chunk.Timings != nil && chunk.Timings.CacheN+chunk.Timings.PromptN > 0 {
			building.usage.CachedInputTokens = chunk.Timings.CacheN
		}
	}
	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			building.finish = choice.FinishReason
			building.sawFinish = true
		}
		if kept := addText(&building.text, choice.Delta.Content); kept != "" && building.onDelta != nil {
			building.onDelta(kept)
		}
		if err := building.addCalls(choice.Delta.ToolCalls); err != nil {
			return true, err
		}
	}
	return false, nil
}

// addCalls adds the pieces of the tool calls one chunk carried.
func (building *openAIReply) addCalls(pieces []openAIDeltaCall) error {
	for _, piece := range pieces {
		index := len(building.order)
		if piece.Index != nil {
			index = *piece.Index
		}
		block, known := building.calls[index]
		if !known {
			if len(building.order) >= maxToolCallsPerReply {
				return fmt.Errorf("the model %q asked for more than %d tools in one reply, so the reply was given up on",
					building.modelName, maxToolCallsPerReply)
			}
			block = &toolCallBuild{}
			building.calls[index] = block
			building.order = append(building.order, index)
		}
		if block.id == "" {
			block.id = piece.ID
		}
		if block.name == "" {
			block.name = piece.Function.Name
		}
		if block.arguments.Len()+len(piece.Function.Arguments) > maxToolCallJSONBytes {
			return fmt.Errorf("the model %q sent more than %d bytes of arguments for one tool call, so the call was given up on",
				building.modelName, maxToolCallJSONBytes)
		}
		block.arguments.WriteString(piece.Function.Arguments)
	}
	return nil
}

// finished turns the built-up state into the reply the harness reads.
func (building *openAIReply) finished(options Options) contract.Reply {
	calls := []contract.ToolCall{}
	for _, index := range building.order {
		block := building.calls[index]
		arguments := block.arguments.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		calls = append(calls, contract.ToolCall{ID: block.id, Name: block.name, Input: json.RawMessage(arguments)})
	}
	return contract.Reply{
		Text:      building.text.String(),
		ToolCalls: calls,
		Finish:    openAIFinish(building.finish, len(calls) > 0, building.modelName, options),
		Usage:     building.usage,
		Model:     building.modelName,
	}
}

// openAIFinish maps the API's finish reason onto the four the contract names.
func openAIFinish(reason string, hasCalls bool, modelName string, options Options) contract.FinishReason {
	switch reason {
	case "stop":
		return contract.FinishEnd
	case "tool_calls", "function_call":
		return contract.FinishToolCalls
	case "length":
		return contract.FinishLength
	case "content_filter":
		return contract.FinishStopped
	case "":
		if hasCalls {
			return contract.FinishToolCalls
		}
		return contract.FinishEnd
	default:
		options.note("the model %q stopped for the reason %q, which the harness reads as stopped", modelName, reason)
		return contract.FinishStopped
	}
}
