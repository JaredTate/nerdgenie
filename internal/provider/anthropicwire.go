package provider

import "encoding/json"

// cacheControl is the marker that says everything up to here may be reused from
// the provider's cache on the next call.
type cacheControl struct {
	// Type is the only kind of cache the Messages API offers.
	Type string `json:"type"`
}

// ephemeralCache is the marker every cache boundary carries.
func ephemeralCache() *cacheControl { return &cacheControl{Type: "ephemeral"} }

// anthropicTextBlock is one piece of the system prompt on the wire.
type anthropicTextBlock struct {
	// Type is always "text" for a system block.
	Type string `json:"type"`
	// Text is the block's words.
	Text string `json:"text"`
	// Cache is the marker on a block that ends a cache boundary, and is left out
	// of the request when the block ends none.
	Cache *cacheControl `json:"cache_control,omitempty"`
}

// anthropicBlock is one piece of a message: text somebody wrote, a tool the
// model asked for, or the result the harness gave back.
type anthropicBlock struct {
	// Type is "text", "tool_use", or "tool_result".
	Type string `json:"type"`
	// Text is the words of a text block.
	Text string `json:"text,omitempty"`
	// ID is the identifier of a tool the model asked for.
	ID string `json:"id,omitempty"`
	// Name is the tool's name on a tool-use block.
	Name string `json:"name,omitempty"`
	// Input is the arguments the model wrote for a tool.
	Input json.RawMessage `json:"input,omitempty"`
	// ToolUseID says which call a result answers.
	ToolUseID string `json:"tool_use_id,omitempty"`
	// Content is the text of a tool result.
	Content string `json:"content,omitempty"`
	// IsError says the tool reported a failure rather than a result.
	IsError bool `json:"is_error,omitempty"`
}

// anthropicMessage is one turn of the conversation on the wire.
type anthropicMessage struct {
	// Role is "user" or "assistant".
	Role string `json:"role"`
	// Content is the message's blocks, in the order the API reads them.
	Content []anthropicBlock `json:"content"`
}

// anthropicTool is one tool the model may ask for.
type anthropicTool struct {
	// Name is what the model calls.
	Name string `json:"name"`
	// Description says when to use the tool and when not to.
	Description string `json:"description"`
	// InputSchema describes the tool's inputs.
	InputSchema jsonSchema `json:"input_schema"`
	// Cache is the marker on the last tool, which is where the tool list ends.
	Cache *cacheControl `json:"cache_control,omitempty"`
}

// anthropicBody is the whole request. It carries no sampling fields and no
// thinking field, because Opus 4.8 rejects the first and the second is a
// deliberate omission until a real user needs it.
type anthropicBody struct {
	// Model is what the server calls the model.
	Model string `json:"model"`
	// MaxTokens is the output cap the request asked for.
	MaxTokens int `json:"max_tokens"`
	// Stream is always true, because this provider always streams.
	Stream bool `json:"stream"`
	// System is the system prompt in order, with its cache markers.
	System []anthropicTextBlock `json:"system,omitempty"`
	// Messages is the conversation.
	Messages []anthropicMessage `json:"messages"`
	// Tools is what the model may ask for, and is left out when the harness has
	// switched the tools off.
	Tools []anthropicTool `json:"tools,omitempty"`
}
