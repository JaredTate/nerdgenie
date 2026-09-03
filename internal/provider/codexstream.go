// The event names, the fields read from each, and the rule that a tool call is
// put together from the item events rather than from the closing event's output
// list were read from Hermes Agent's Codex stream reader at
// ~/Code/hermes-agent/agent/codex_runtime.py, which is the path that
// ~/Code/hermes-agent/agent/transports/codex.py and
// ~/Code/hermes-agent/agent/codex_responses_adapter.py run their streams
// through. The two places an error frame keeps its message, flat or inside an
// "error" envelope, come from the same file, and the usage field names from
// ~/Code/hermes-agent/agent/usage_pricing.py.

package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// codexResult is what one streamed call to the Codex backend came to.
type codexResult struct {
	// Text is the model's plain text, already handed to the caller in deltas.
	Text string
	// ToolCalls are the tools it asked for, in the order they appeared.
	ToolCalls []contract.ToolCall
	// Usage is the token count the closing event reported.
	Usage contract.Usage
	// Finish says why the model stopped.
	Finish contract.FinishReason
	// Model is the name of the model that answered.
	Model string
}

// codexItem is one output item of a Responses API reply, as the added and done
// events carry it. Only a function call is read from it; a message's text has
// already arrived in deltas.
type codexItem struct {
	// Type says what the item is, and "function_call" is the one kind read.
	Type string `json:"type"`
	// ID is the item's own identifier, which the argument events point at.
	ID string `json:"id"`
	// CallID is the identifier the harness echoes back on the tool result.
	CallID string `json:"call_id"`
	// Name is the tool's name.
	Name string `json:"name"`
	// Arguments is the whole of the arguments as one JSON string, on the done
	// event and sometimes already on the added event.
	Arguments string `json:"arguments"`
}

// codexError is the message a failing stream carries, in either of its shapes.
type codexError struct {
	// Message says what went wrong, in the backend's words.
	Message string `json:"message"`
}

// codexEvent is one data line of a Responses API stream, wide enough to hold
// every event the reader answers.
type codexEvent struct {
	// Type is the event's name.
	Type string `json:"type"`
	// OutputIndex says which output item the event belongs to, when it has one.
	OutputIndex *int `json:"output_index"`
	// ItemID names the item an argument event belongs to.
	ItemID string `json:"item_id"`
	// Item is the output item an added or done event carries.
	Item codexItem `json:"item"`
	// Delta is one piece of text, or one piece of a tool call's arguments.
	Delta string `json:"delta"`
	// Arguments is the whole of a call's arguments on the argument done event.
	// It is a pointer so that a missing field leaves the streamed pieces alone
	// while an empty string counts as the backend's final word.
	Arguments *string `json:"arguments"`
	// Response is the reply's closing summary, on the three ending events.
	Response struct {
		// Model is the name of the model that answered.
		Model string `json:"model"`
		// Usage is the token count for the call.
		Usage struct {
			// InputTokens is everything the model read, including the cached part.
			InputTokens int `json:"input_tokens"`
			// InputTokensDetails holds the part of the input that came from the cache.
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			// OutputTokens is everything the model wrote. It already counts the
			// reasoning tokens that output_tokens_details.reasoning_tokens sets
			// apart, so that detail is not read: adding it would count them twice.
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		// IncompleteDetails says why an incomplete reply stopped short.
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		// Error is what a failed reply says went wrong.
		Error codexError `json:"error"`
	} `json:"response"`
	// Message is an error frame's message when the frame keeps it flat.
	Message string `json:"message"`
	// Error is an error frame's message when the frame wraps it in an envelope.
	Error codexError `json:"error"`
}

// codexReply is the reply as it is being built from the stream.
type codexReply struct {
	onDelta func(delta string)
	text    bytes.Buffer
	calls   map[string]*toolCallBuild
	order   []string
	usage   contract.Usage
	model   string
	finish  contract.FinishReason
	// sawEnd says the stream reached a completed or an incomplete event, which
	// are the two endings that leave a reply behind.
	sawEnd bool
}

// readCodexStream reads one streamed reply, handing each piece of text to
// onDelta as it arrives.
func readCodexStream(reader io.Reader, onDelta func(delta string)) (codexResult, error) {
	building := &codexReply{onDelta: onDelta, calls: map[string]*toolCallBuild{}}
	err := forEachDataLine(reader, func(payload []byte) (bool, error) {
		event := codexEvent{}
		if err := json.Unmarshal(payload, &event); err != nil {
			return true, fmt.Errorf("the codex stream sent a data line that is not JSON, so the reply was given up on: %w", err)
		}
		return building.take(event)
	})
	if err != nil {
		return codexResult{}, err
	}
	if !building.sawEnd {
		return codexResult{}, errors.New("the codex stream ended before the response.completed event, so the reply is incomplete and the call should be made again")
	}
	return building.finished(), nil
}

// take answers one event, and says whether the stream is over.
func (building *codexReply) take(event codexEvent) (bool, error) {
	switch event.Type {
	case "response.output_text.delta":
		if kept := addText(&building.text, event.Delta); kept != "" && building.onDelta != nil {
			building.onDelta(kept)
		}
	case "response.output_item.added", "response.output_item.done":
		if event.Item.Type == "function_call" {
			return false, building.takeItem(event)
		}
	case "response.function_call_arguments.delta":
		if block, known := building.calls[building.keyFor(event.ItemID, event.OutputIndex)]; known {
			return false, addArguments(block, event.Delta)
		}
	case "response.function_call_arguments.done":
		if block, known := building.calls[building.keyFor(event.ItemID, event.OutputIndex)]; known && event.Arguments != nil {
			block.arguments.Reset()
			return false, addArguments(block, *event.Arguments)
		}
	case "response.completed":
		building.end(event, contract.FinishEnd)
		return true, nil
	case "response.incomplete":
		building.end(event, incompleteFinish(event.Response.IncompleteDetails.Reason))
		return true, nil
	case "response.failed":
		return true, fmt.Errorf("the codex backend failed the call and said: %s", orNoReason(event.Response.Error.Message))
	case "error":
		return true, fmt.Errorf("the codex backend stopped the call with an error: %s", orNoReason(firstOf(event.Message, event.Error.Message)))
	}
	return false, nil
}

// takeItem opens a function call on its added event, or fills it in on its
// done event, whichever arrives first. The done event's fields win where they
// are set, and the streamed pieces stay where it leaves a field empty.
func (building *codexReply) takeItem(event codexEvent) error {
	key := building.keyFor(event.Item.ID, event.OutputIndex)
	if key == "" {
		key = fmt.Sprintf("unnamed call %d", len(building.order))
	}
	block, known := building.calls[key]
	if !known {
		if len(building.order) >= maxToolCallsPerReply {
			return fmt.Errorf("the model asked for more than %d tools in one reply, so the reply was given up on", maxToolCallsPerReply)
		}
		block = &toolCallBuild{}
		building.calls[key] = block
		building.order = append(building.order, key)
	}
	block.id = firstOf(event.Item.CallID, block.id)
	block.name = firstOf(event.Item.Name, block.name)
	if event.Item.Arguments == "" {
		return nil
	}
	block.arguments.Reset()
	return addArguments(block, event.Item.Arguments)
}

// keyFor names the call an event belongs to: by the item's identifier when the
// backend sends one, and by its place in the output when it does not. An event
// with neither has no call, and the caller decides what that means.
func (building *codexReply) keyFor(itemID string, outputIndex *int) string {
	if itemID != "" {
		return "item " + itemID
	}
	if outputIndex != nil {
		return fmt.Sprintf("index %d", *outputIndex)
	}
	return ""
}

// end takes what the closing event reports and marks the stream as ended.
func (building *codexReply) end(event codexEvent, finish contract.FinishReason) {
	building.usage = contract.Usage{
		InputTokens:       event.Response.Usage.InputTokens,
		CachedInputTokens: event.Response.Usage.InputTokensDetails.CachedTokens,
		OutputTokens:      event.Response.Usage.OutputTokens,
	}
	building.model = event.Response.Model
	building.finish = finish
	building.sawEnd = true
}

// finished turns the built-up state into the result the caller reads.
func (building *codexReply) finished() codexResult {
	calls := []contract.ToolCall{}
	for _, key := range building.order {
		block := building.calls[key]
		arguments := block.arguments.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		calls = append(calls, contract.ToolCall{ID: block.id, Name: block.name, Input: json.RawMessage(arguments)})
	}
	finish := building.finish
	if finish == contract.FinishEnd && len(calls) > 0 {
		finish = contract.FinishToolCalls
	}
	return codexResult{
		Text:      building.text.String(),
		ToolCalls: calls,
		Usage:     building.usage,
		Finish:    finish,
		Model:     building.model,
	}
}

// addArguments appends a piece of a tool call's arguments, keeping the whole
// inside the cap.
func addArguments(block *toolCallBuild, piece string) error {
	if block.arguments.Len()+len(piece) > maxToolCallJSONBytes {
		return fmt.Errorf("the model sent more than %d bytes of arguments for one tool call, so the call was given up on", maxToolCallJSONBytes)
	}
	block.arguments.WriteString(piece)
	return nil
}

// incompleteFinish maps the reason an incomplete reply gives onto the contract:
// the output cap is a length finish, and anything else is a reply the backend
// cut short.
func incompleteFinish(reason string) contract.FinishReason {
	if reason == "max_output_tokens" {
		return contract.FinishLength
	}
	return contract.FinishStopped
}

// firstOf returns the first of the two strings that is not empty.
func firstOf(first string, second string) string {
	if first != "" {
		return first
	}
	return second
}

// orNoReason fills in for a backend that failed without saying why.
func orNoReason(said string) string {
	return firstOf(said, "no reason was given")
}
