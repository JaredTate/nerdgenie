// The shapes on this wire are the Responses API's, as the two working
// implementations send them to the ChatGPT backend: the input items, the flat
// function tools with strict off, store always false, and the output cap left
// out, were read from Hermes at
// ~/.hermes/hermes-agent/agent/codex_responses_adapter.py
// (_chat_messages_to_responses_input and the tool conversion beside it) and
// ~/.hermes/hermes-agent/agent/transports/codex.py (build_kwargs), which is
// the installed Hermes; the reference clone at ~/Code/hermes-agent is older
// than that transport and has no copy of it.

package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// codexTool is one tool the model may ask for. Unlike the Chat Completions
// wire, the Responses API writes the function's fields flat on the tool.
type codexTool struct {
	// Type is always "function".
	Type string `json:"type"`
	// Name is what the model calls.
	Name string `json:"name"`
	// Description says when to use the tool and when not to.
	Description string `json:"description"`
	// Parameters describes the tool's inputs.
	Parameters jsonSchema `json:"parameters"`
	// Strict is always false, because the harness's schemas mark only the
	// fields that are required, and strict mode wants every field required.
	Strict bool `json:"strict"`
}

// codexInputItem is one item of the conversation on the wire. A message
// carries a role and content; a function_call carries the call's identifier,
// name, and arguments; a function_call_output answers one by its identifier.
type codexInputItem struct {
	// Type is "function_call" or "function_call_output", and is left out on a
	// message, whose type the API reads from its role.
	Type string `json:"type,omitempty"`
	// Role is "user" or "assistant", on a message.
	Role string `json:"role,omitempty"`
	// Content is the message's text.
	Content string `json:"content,omitempty"`
	// CallID is the call's identifier, on a call and on its output.
	CallID string `json:"call_id,omitempty"`
	// Name is the tool the model asked for, on a call.
	Name string `json:"name,omitempty"`
	// Arguments is the JSON object the model wrote, as a string, on a call.
	Arguments string `json:"arguments,omitempty"`
	// Output is the tool's result, on an output. It is a pointer so that an
	// empty result still goes over as an empty string rather than nothing.
	Output *string `json:"output,omitempty"`
}

// codexReasoning is how hard the model is asked to think.
type codexReasoning struct {
	// Effort is the level's wire word.
	Effort string `json:"effort"`
}

// codexBody is the whole request. It carries no sampling fields, because the
// design leaves the sampling to the server, and no output cap, because the
// ChatGPT backend refuses one; the reply's text is capped in the reader.
type codexBody struct {
	// Model is what the backend calls the model.
	Model string `json:"model"`
	// Instructions is the system prompt, which this API takes as a field of
	// its own rather than as the first message.
	Instructions string `json:"instructions,omitempty"`
	// Input is the conversation as items.
	Input []codexInputItem `json:"input"`
	// Tools is what the model may ask for, and is left out when the harness
	// has switched the tools off.
	Tools []codexTool `json:"tools,omitempty"`
	// ToolChoice is "auto" when there are tools, so that the model decides.
	ToolChoice string `json:"tool_choice,omitempty"`
	// ParallelToolCalls lets the model ask for more than one tool in a reply,
	// and is left out when there are no tools.
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`
	// Store is always false, because the backend keeps nothing for a client
	// that is not the codex program, and refuses to be asked.
	Store bool `json:"store"`
	// Stream is always true, because this provider always streams.
	Stream bool `json:"stream"`
	// Reasoning is how hard to think, and is left out when nobody asked, so
	// that the backend's own default stands.
	Reasoning *codexReasoning `json:"reasoning,omitempty"`
}

// codexRequestBody turns one harness request into the JSON the backend
// reads, for the model the backend knows by the given name. The think level
// is the one on the request, which the caller has already settled against the
// alias and checked.
func codexRequestBody(request contract.Request, modelName string) ([]byte, error) {
	body := codexBody{
		Model:        modelName,
		Instructions: joinSystemBlocks(request.SystemBlocks),
		Input:        codexInputItems(request),
		Tools:        codexTools(request),
		Store:        false,
		Stream:       true,
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
		parallel := true
		body.ParallelToolCalls = &parallel
	}
	if asksForThinking(request.Think) {
		body.Reasoning = &codexReasoning{Effort: codexEffort(request.Think)}
	}
	written, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("the request to the model %q could not be written as JSON: %w", modelName, err)
	}
	return written, nil
}

// codexInputItems walks the conversation and writes it as the items this API
// reads: a message for each turn with text, a function_call for each call the
// assistant made, and a function_call_output for each result.
func codexInputItems(request contract.Request) []codexInputItem {
	items := []codexInputItem{}
	for _, message := range request.Messages {
		items = append(items, codexInputItemsFor(message)...)
	}
	return items
}

// codexInputItemsFor turns one harness message into the items it stands for.
// Results come first, because they answer calls from the turn before.
func codexInputItemsFor(message contract.Message) []codexInputItem {
	written := []codexInputItem{}
	for _, result := range message.ToolResults {
		text := result.Text
		if result.Failed {
			text = failedToolResultPrefix + text
		}
		written = append(written, codexInputItem{Type: "function_call_output", CallID: result.CallID, Output: &text})
	}
	if message.Text != "" {
		written = append(written, codexInputItem{Role: string(message.Role), Content: message.Text})
	}
	for _, call := range message.ToolCalls {
		arguments := string(call.Input)
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		written = append(written, codexInputItem{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: arguments})
	}
	return written
}

// codexTools turns the tool specifications into the flat function tools this
// API reads, and returns nothing at all when the harness has switched the
// tools off.
func codexTools(request contract.Request) []codexTool {
	if request.ToolsOff {
		return nil
	}
	tools := []codexTool{}
	for _, spec := range request.Tools {
		tools = append(tools, codexTool{
			Type:        "function",
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  schemaForFields(spec.Fields),
			Strict:      false,
		})
	}
	return tools
}
