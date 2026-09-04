// The shape of one Responses API call to OpenAI's Codex backend was read from
// Hermes: the message, function_call, and function_call_output input items and
// the flat function tool with strict off come from
// ~/Code/hermes-agent/agent/codex_responses_adapter.py
// (_chat_messages_to_responses_input and _responses_tools), and the request
// fields it posts, store, tool_choice, parallel_tool_calls, reasoning, and
// include, come from ~/Code/hermes-agent/agent/transports/codex.py. The copy
// under ~/.hermes/hermes-agent that the brief named builds the same shapes.

package provider

import (
	"encoding/json"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// codexTextPart is one piece of text inside a message item. Its type is
// "input_text" on a user message and "output_text" on an assistant one, and the
// API rejects the wrong one.
type codexTextPart struct {
	// Type says which side wrote the text.
	Type string `json:"type"`
	// Text is the words.
	Text string `json:"text"`
}

// codexMessageItem is one turn of plain text in the input.
type codexMessageItem struct {
	// Type is always "message".
	Type string `json:"type"`
	// Role is "user" or "assistant".
	Role string `json:"role"`
	// Content is the text in its one part.
	Content []codexTextPart `json:"content"`
}

// codexCallItem is one tool call the model made on an earlier turn, sent back
// so that the model can see its own call beside the result.
type codexCallItem struct {
	// Type is always "function_call".
	Type string `json:"type"`
	// CallID is the identifier the result answers.
	CallID string `json:"call_id"`
	// Name is the tool the model asked for.
	Name string `json:"name"`
	// Arguments is the JSON object the model wrote, as a string.
	Arguments string `json:"arguments"`
}

// codexOutputItem is the result of one tool call.
type codexOutputItem struct {
	// Type is always "function_call_output".
	Type string `json:"type"`
	// CallID says which call this answers.
	CallID string `json:"call_id"`
	// Output is the result's text, which may be empty.
	Output string `json:"output"`
}

// codexTool is one tool the model may ask for. Unlike the chat-completions
// shape, the name and description sit beside the type rather than under a
// function key.
type codexTool struct {
	// Type is always "function".
	Type string `json:"type"`
	// Name is what the model calls.
	Name string `json:"name"`
	// Description says when to use the tool and when not to.
	Description string `json:"description"`
	// Parameters describes the tool's inputs.
	Parameters jsonSchema `json:"parameters"`
	// Strict is always false, because strict mode demands a schema that names
	// every field required and forbids the optional ones these tools have.
	Strict bool `json:"strict"`
}

// codexReasoning is how hard the model is asked to think.
type codexReasoning struct {
	// Effort is the level's own name.
	Effort string `json:"effort"`
}

// codexBody is the whole request.
type codexBody struct {
	// Model is what the backend calls the model.
	Model string `json:"model"`
	// Instructions is the system prompt, which this API takes as a field of its
	// own rather than as the first message.
	Instructions string `json:"instructions"`
	// Input is the conversation as a flat list of items.
	Input []any `json:"input"`
	// Tools is what the model may ask for, and is left out when the harness has
	// switched the tools off or there are none.
	Tools []codexTool `json:"tools,omitempty"`
	// ToolChoice is "auto" whenever tools are sent, and left out otherwise.
	ToolChoice string `json:"tool_choice,omitempty"`
	// ParallelToolCalls lets the model make several calls in one turn, and is
	// sent only beside the tools.
	ParallelToolCalls bool `json:"parallel_tool_calls,omitempty"`
	// Stream is always true, because this provider always streams.
	Stream bool `json:"stream"`
	// Store is always false, because the backend keeps nothing between calls
	// and the harness sends the whole conversation each time.
	Store bool `json:"store"`
	// Reasoning is the effort level, and is left out when nobody asked or
	// thinking is off, so that the backend's own default stands.
	Reasoning *codexReasoning `json:"reasoning,omitempty"`
	// Include asks the backend to send its encrypted reasoning back beside the
	// reply, and goes with the reasoning field.
	Include []string `json:"include,omitempty"`
}

// encryptedReasoningInclude is the one thing the reference asks the backend to
// include.
const encryptedReasoningInclude = "reasoning.encrypted_content"

// codexRequestBody is the JSON body of one call to the Codex backend for the
// request.
func codexRequestBody(request contract.Request, modelName string) ([]byte, error) {
	body := codexBody{
		Model:        modelName,
		Instructions: joinSystemBlocks(request.SystemBlocks),
		Input:        codexInput(request.Messages),
		Tools:        codexTools(request),
		Stream:       true,
		Store:        false,
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
		body.ParallelToolCalls = true
	}
	if asksForThinking(request.Think) {
		body.Reasoning = &codexReasoning{Effort: string(request.Think)}
		body.Include = []string{encryptedReasoningInclude}
	}
	return json.Marshal(body)
}

// codexInput flattens the conversation into the items this API reads, in order.
func codexInput(messages []contract.Message) []any {
	items := []any{}
	for _, message := range messages {
		items = append(items, codexItemsFor(message)...)
	}
	return items
}

// codexItemsFor turns one harness message into its items: the results first,
// because they answer the calls that came before, then the text, then the
// calls. A message that carries nothing at all adds nothing.
func codexItemsFor(message contract.Message) []any {
	items := []any{}
	for _, result := range message.ToolResults {
		text := result.Text
		if result.Failed {
			text = failedToolResultPrefix + text
		}
		items = append(items, codexOutputItem{Type: "function_call_output", CallID: result.CallID, Output: text})
	}
	if message.Text != "" {
		items = append(items, codexMessageFor(message.Role, message.Text))
	}
	for _, call := range message.ToolCalls {
		arguments := strings.TrimSpace(string(call.Input))
		if arguments == "" {
			arguments = "{}"
		}
		items = append(items, codexCallItem{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: arguments})
	}
	return items
}

// codexMessageFor wraps text in the message item for its side, with the part
// type that side is allowed.
func codexMessageFor(role contract.Role, text string) codexMessageItem {
	partType := "input_text"
	if role == contract.RoleAssistant {
		partType = "output_text"
	}
	return codexMessageItem{
		Type:    "message",
		Role:    string(role),
		Content: []codexTextPart{{Type: partType, Text: text}},
	}
}

// codexTools turns the tool specifications into the flat shape this API reads,
// and returns nothing at all when the harness has switched the tools off.
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
