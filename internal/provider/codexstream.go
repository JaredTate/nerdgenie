// The events this reader answers, and the way a function call is put back
// together from the item that opens it, the argument pieces keyed by that
// item's identifier, and the finished item that closes it, were read from the
// Responses stream as Hermes handles it at
// ~/.hermes/hermes-agent/agent/codex_responses_adapter.py, which is the
// installed Hermes; the reference clone at ~/Code/hermes-agent is older than
// that transport and has no copy of it. Hermes reads the stream through the
// vendor's software development kit; this is Go over net/http, so that the
// harness owns the parser it fuzzes.

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

// codexResult is what one whole stream from the backend came to.
type codexResult struct {
	// Text is the model's plain text, already handed out in deltas.
	Text string
	// ToolCalls are the tools it asked for, in the order it asked.
	ToolCalls []contract.ToolCall
	// Usage is the token count the backend reported at the end.
	Usage contract.Usage
	// Finish says why the model stopped.
	Finish contract.FinishReason
	// Model is what the backend said answered, when it said.
	Model string
}

// codexCount is the token count the backend reports on the event that ends
// the response.
type codexCount struct {
	// InputTokens is the whole prompt, including whatever came from the cache.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is what the model wrote.
	OutputTokens int `json:"output_tokens"`
	// InputTokensDetails holds the part of the prompt that came from the cache.
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

// codexItem is one output item as the stream describes it when it opens and
// when it is done. Only a function call's fields are read.
type codexItem struct {
	// ID is the item's own identifier, which the argument pieces are keyed by.
	ID string `json:"id"`
	// Type is "message", "function_call", or "reasoning".
	Type string `json:"type"`
	// CallID is the call's identifier, which the result goes back under.
	CallID string `json:"call_id"`
	// Name is the tool the model asked for.
	Name string `json:"name"`
	// Arguments is the whole JSON object, on the finished item.
	Arguments string `json:"arguments"`
}

// codexEvent is one event of the Responses stream, wide enough to hold every
// event the reader answers.
type codexEvent struct {
	// Type is the event's name, which the stream repeats inside the payload.
	Type string `json:"type"`
	// Delta is one piece of text or one piece of a call's arguments.
	Delta string `json:"delta"`
	// ItemID says which item a piece belongs to.
	ItemID string `json:"item_id"`
	// Item is the output item an item event describes.
	Item codexItem `json:"item"`
	// Response carries the model, the usage, and the reason on the event that
	// ends it.
	Response struct {
		Model             string      `json:"model"`
		Usage             *codexCount `json:"usage"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	// Message is what an error event says at its top level.
	Message string `json:"message"`
	// Error is what an error event says when it wraps the message instead.
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// errCodexStreamIncomplete means the stream ended before the backend said the
// response was complete, which a later attempt could get past.
var errCodexStreamIncomplete = errors.New("the stream ended before the backend said the response was complete, so the reply is incomplete")

// codexReply is the reply as it is being built from the stream.
type codexReply struct {
	onDelta func(delta string)
	text    bytes.Buffer
	calls   map[string]*toolCallBuild
	order   []string
	usage   contract.Usage
	model   string
	// incompleteReason is why the backend stopped early, or empty when it
	// finished.
	incompleteReason string
	sawEnd           bool
}

// readCodexStream turns the bytes of a Responses stream into one result,
// handing every piece of text to the delta function as it arrives.
func readCodexStream(reader io.Reader, onDelta func(delta string)) (codexResult, error) {
	building := &codexReply{onDelta: onDelta, calls: map[string]*toolCallBuild{}}
	err := forEachDataLine(reader, func(payload []byte) (bool, error) {
		event := codexEvent{}
		if err := json.Unmarshal(payload, &event); err != nil {
			return false, nil
		}
		return building.take(event)
	})
	if err != nil {
		return codexResult{}, err
	}
	if !building.sawEnd {
		return codexResult{}, errCodexStreamIncomplete
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
	case "response.output_item.added":
		return false, building.openItem(event.Item)
	case "response.function_call_arguments.delta":
		return false, building.addArguments(event.ItemID, event.Delta)
	case "response.output_item.done":
		return false, building.closeItem(event.Item)
	case "response.completed", "response.incomplete":
		building.countTokens(event.Response.Usage)
		building.model = event.Response.Model
		building.incompleteReason = event.Response.IncompleteDetails.Reason
		building.sawEnd = true
		return true, nil
	case "response.failed":
		return true, fmt.Errorf("the backend stopped with an error: %s", event.Response.Error.Message)
	case "error":
		said := event.Message
		if said == "" {
			said = event.Error.Message
		}
		return true, fmt.Errorf("the backend stopped with an error: %s", said)
	}
	return false, nil
}

// countTokens keeps the usage from the event that ended the response. The
// cached count is kept inside the input count, because the contract says it is
// a part of it, and a backend that said otherwise would put nonsense on the
// cost line.
func (building *codexReply) countTokens(count *codexCount) {
	if count == nil {
		return
	}
	building.usage = contract.Usage{
		InputTokens:       count.InputTokens,
		CachedInputTokens: min(count.InputTokensDetails.CachedTokens, count.InputTokens),
		OutputTokens:      count.OutputTokens,
	}
}

// openItem starts a tool call when the item that opened is one, and ignores
// every other kind of item.
func (building *codexReply) openItem(item codexItem) error {
	if item.Type != "function_call" {
		return nil
	}
	block, err := building.callFor(item.ID)
	if err != nil {
		return err
	}
	block.id = item.CallID
	block.name = item.Name
	return building.addArguments(item.ID, item.Arguments)
}

// addArguments adds one piece of a call's arguments.
func (building *codexReply) addArguments(itemID, piece string) error {
	block, err := building.callFor(itemID)
	if err != nil {
		return err
	}
	if block.arguments.Len()+len(piece) > maxToolCallJSONBytes {
		return fmt.Errorf("the backend sent more than %d bytes of arguments for one tool call, so the call was given up on",
			maxToolCallJSONBytes)
	}
	block.arguments.WriteString(piece)
	return nil
}

// closeItem takes the finished item's own fields as the truth about a call,
// because the backend writes the whole arguments there whether or not it sent
// them in pieces first.
func (building *codexReply) closeItem(item codexItem) error {
	if item.Type != "function_call" {
		return nil
	}
	block, err := building.callFor(item.ID)
	if err != nil {
		return err
	}
	if item.CallID != "" {
		block.id = item.CallID
	}
	if item.Name != "" {
		block.name = item.Name
	}
	if item.Arguments == "" {
		return nil
	}
	block.arguments.Reset()
	return building.addArguments(item.ID, item.Arguments)
}

// callFor finds the call an item identifier belongs to, starting one when it
// is new and refusing to start more than the cap.
func (building *codexReply) callFor(itemID string) (*toolCallBuild, error) {
	block, known := building.calls[itemID]
	if known {
		return block, nil
	}
	if len(building.order) >= maxToolCallsPerReply {
		return nil, fmt.Errorf("the backend asked for more than %d tools in one reply, so the reply was given up on",
			maxToolCallsPerReply)
	}
	block = &toolCallBuild{}
	building.calls[itemID] = block
	building.order = append(building.order, itemID)
	return block, nil
}

// finished turns the built-up state into the result the provider reads.
func (building *codexReply) finished() codexResult {
	calls := []contract.ToolCall{}
	for _, itemID := range building.order {
		block := building.calls[itemID]
		arguments := block.arguments.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		calls = append(calls, contract.ToolCall{ID: block.id, Name: block.name, Input: json.RawMessage(arguments)})
	}
	return codexResult{
		Text:      building.text.String(),
		ToolCalls: calls,
		Usage:     building.usage,
		Finish:    codexFinish(building.incompleteReason, len(calls) > 0),
		Model:     building.model,
	}
}

// codexFinish maps the way the response ended onto the four reasons the
// contract names. A completed response has no reason at all, a response cut
// at the output cap says so, and anything else the backend stopped for is
// read as stopped.
func codexFinish(incompleteReason string, hasCalls bool) contract.FinishReason {
	switch incompleteReason {
	case "":
		if hasCalls {
			return contract.FinishToolCalls
		}
		return contract.FinishEnd
	case "max_output_tokens":
		return contract.FinishLength
	default:
		return contract.FinishStopped
	}
}
