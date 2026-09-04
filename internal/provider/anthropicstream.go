// The event names, the block indexes, and the two usage moments this reader
// answers were read from Prime Agent's Anthropic provider at
// ~/Code/prime-agent/packages/ai/src/providers/anthropic.ts, which reads the
// same stream through the vendor's software development kit.

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// anthropicCount is one token count in the stream. The fields are pointers
// because a proxy may leave one out, and a missing count must not overwrite the
// one the stream already reported.
type anthropicCount struct {
	// InputTokens is what the model read, not counting anything it reused from
	// the cache or wrote into it.
	InputTokens *int `json:"input_tokens"`
	// OutputTokens is what the model wrote.
	OutputTokens *int `json:"output_tokens"`
	// CacheRead is how much of the prompt came back from the cache.
	CacheRead *int `json:"cache_read_input_tokens"`
	// CacheCreation is how much of the prompt was written into the cache.
	CacheCreation *int `json:"cache_creation_input_tokens"`
}

// anthropicEvent is one event of the Messages API stream, wide enough to hold
// every event the reader answers.
type anthropicEvent struct {
	// Type is the event's name, which the stream repeats inside the payload.
	Type string `json:"type"`
	// Index says which content block the event belongs to.
	Index int `json:"index"`
	// Message carries the first token counts, on the opening event.
	Message struct {
		Usage anthropicCount `json:"usage"`
	} `json:"message"`
	// ContentBlock describes a block that is starting.
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	// Delta is one piece of text, or one piece of a tool call's arguments, or
	// the reason the model stopped.
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	// Usage carries the final token counts, on the closing event.
	Usage anthropicCount `json:"usage"`
	// Error is what the server says when it gives up part way through.
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// toolCallBuild is one tool call being put back together from the pieces its
// arguments arrived in.
type toolCallBuild struct {
	id        string
	name      string
	arguments strings.Builder
}

// anthropicReply is the reply as it is being built from the stream.
type anthropicReply struct {
	modelName string
	onDelta   func(delta string)
	text      bytes.Buffer
	blocks    map[int]*toolCallBuild
	order     []int
	usage     contract.Usage
	// The three input counts are kept apart because the count the harness reports
	// is all three added together, and any one of them may arrive on its own.
	inputTokens   int
	cacheCreation int
	cacheRead     int
	stopReason    string
	sawStop       bool
}

// readAnthropicStream reads one whole answer, turning a call the stall watch cut
// short into the stalled-stream sentinel.
func readAnthropicStream(stream *openStream, modelName string, options Options,
	onDelta func(delta string)) (contract.Reply, error) {
	reply, err := parseAnthropicStream(stream.reader, modelName, options, onDelta)
	if err != nil && stream.watch.stalled.Load() {
		return contract.Reply{}, stream.watch.explain(modelName, err)
	}
	return reply, err
}

// parseAnthropicStream turns the bytes of a Messages API stream into one reply,
// handing every piece of text to the delta function as it arrives.
func parseAnthropicStream(reader io.Reader, modelName string, options Options,
	onDelta func(delta string)) (contract.Reply, error) {
	building := &anthropicReply{modelName: modelName, onDelta: onDelta, blocks: map[int]*toolCallBuild{}}
	err := forEachDataLine(reader, func(payload []byte) (bool, error) {
		event := anthropicEvent{}
		if err := json.Unmarshal(payload, &event); err != nil {
			return false, nil
		}
		return building.take(event)
	})
	if err != nil {
		return contract.Reply{}, err
	}
	if !building.sawStop {
		return contract.Reply{}, providerError{
			modelName: modelName,
			message:   "the stream ended before the model said it had finished, so the reply is incomplete",
			retryable: true,
		}
	}
	return building.finished(options), nil
}

// take answers one event, and says whether the stream is over.
func (building *anthropicReply) take(event anthropicEvent) (bool, error) {
	switch event.Type {
	case "message_start":
		building.countTokens(event.Message.Usage)
	case "content_block_start":
		building.startBlock(event)
	case "content_block_delta":
		return false, building.addDelta(event)
	case "message_delta":
		if event.Delta.StopReason != "" {
			building.stopReason = event.Delta.StopReason
		}
		building.countTokens(event.Usage)
	case "message_stop":
		building.sawStop = true
		return true, nil
	case "error":
		return true, fmt.Errorf("the model %q stopped with an error: %s", building.modelName, event.Error.Message)
	}
	return false, nil
}

// startBlock opens a tool-use block, which is the only kind whose start the
// reader has to remember.
func (building *anthropicReply) startBlock(event anthropicEvent) {
	if event.ContentBlock.Type != "tool_use" || len(building.order) >= maxToolCallsPerReply {
		return
	}
	if _, already := building.blocks[event.Index]; already {
		return
	}
	building.blocks[event.Index] = &toolCallBuild{id: event.ContentBlock.ID, name: event.ContentBlock.Name}
	building.order = append(building.order, event.Index)
}

// addDelta adds one piece of text or one piece of a tool call's arguments.
func (building *anthropicReply) addDelta(event anthropicEvent) error {
	switch event.Delta.Type {
	case "text_delta":
		kept := addText(&building.text, event.Delta.Text)
		if kept != "" && building.onDelta != nil {
			building.onDelta(kept)
		}
	case "input_json_delta":
		block, known := building.blocks[event.Index]
		if !known {
			return nil
		}
		if block.arguments.Len()+len(event.Delta.PartialJSON) > maxToolCallJSONBytes {
			return fmt.Errorf("the model %q sent more than %d bytes of arguments for one tool call, so the call was given up on",
				building.modelName, maxToolCallJSONBytes)
		}
		block.arguments.WriteString(event.Delta.PartialJSON)
	}
	return nil
}

// countTokens takes the counts an event reported, leaving alone every count the
// event did not mention.
func (building *anthropicReply) countTokens(counted anthropicCount) {
	if counted.InputTokens != nil {
		building.inputTokens = *counted.InputTokens
	}
	if counted.OutputTokens != nil {
		building.usage.OutputTokens = *counted.OutputTokens
	}
	if counted.CacheRead != nil {
		building.cacheRead = *counted.CacheRead
	}
	if counted.CacheCreation != nil {
		building.cacheCreation = *counted.CacheCreation
	}
	// The input count is everything the model read. This wire counts that in
	// three places, because it charges differently for each: what it read
	// plainly, what it wrote into the cache, and what it read back out of the
	// cache. The cached count the harness reports is the last of the three on its
	// own, which is what makes it a part of the input count rather than an
	// addition to it.
	building.usage.CachedInputTokens = building.cacheRead
	building.usage.InputTokens = building.inputTokens + building.cacheCreation + building.cacheRead
}

// finished turns the built-up state into the reply the harness reads.
func (building *anthropicReply) finished(options Options) contract.Reply {
	calls := []contract.ToolCall{}
	for _, index := range building.order {
		block := building.blocks[index]
		arguments := block.arguments.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		calls = append(calls, contract.ToolCall{ID: block.id, Name: block.name, Input: json.RawMessage(arguments)})
	}
	return contract.Reply{
		Text:      building.text.String(),
		ToolCalls: calls,
		Finish:    anthropicFinish(building.stopReason, len(calls) > 0, building.modelName, options),
		Usage:     building.usage,
		Model:     building.modelName,
	}
}

// anthropicFinish maps the API's stop reason onto the four the contract names.
func anthropicFinish(stopReason string, hasCalls bool, modelName string, options Options) contract.FinishReason {
	switch stopReason {
	case "end_turn", "stop_sequence", "pause_turn":
		return contract.FinishEnd
	case "tool_use":
		return contract.FinishToolCalls
	case "max_tokens":
		return contract.FinishLength
	case "":
		if hasCalls {
			return contract.FinishToolCalls
		}
		return contract.FinishEnd
	default:
		options.note("the model %q stopped for the reason %q, which the harness reads as stopped", modelName, stopReason)
		return contract.FinishStopped
	}
}
