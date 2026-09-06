package provider

// openAIFunction is the part of a tool the API calls a function.
type openAIFunction struct {
	// Name is what the model calls.
	Name string `json:"name"`
	// Description says when to use the tool and when not to.
	Description string `json:"description"`
	// Parameters describes the tool's inputs.
	Parameters jsonSchema `json:"parameters"`
}

// openAITool is one tool the model may ask for.
type openAITool struct {
	// Type is always "function", which is the only kind this API has.
	Type string `json:"type"`
	// Function is the tool itself.
	Function openAIFunction `json:"function"`
}

// openAICallFunction is the name and arguments of one call the model made. The
// arguments are a string holding JSON, not JSON, which is this API's own shape.
type openAICallFunction struct {
	// Name is the tool the model asked for.
	Name string `json:"name"`
	// Arguments is the JSON object the model wrote, as a string.
	Arguments string `json:"arguments"`
}

// openAIToolCall is one tool call on a message the harness sends back.
type openAIToolCall struct {
	// ID is the identifier the server gave the call.
	ID string `json:"id"`
	// Type is always "function".
	Type string `json:"type"`
	// Function is the call's name and arguments.
	Function openAICallFunction `json:"function"`
}

// openAIMessage is one turn of the conversation on the wire.
type openAIMessage struct {
	// Role is "system", "user", "assistant", or "tool".
	Role string `json:"role"`
	// Content is the message's text, which is empty on a message that carries
	// only tool calls, or the list of parts of a message that carries a
	// picture beside its words.
	Content any `json:"content"`
	// ToolCalls are the tools an assistant message asked for.
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
	// ToolCallID says which call a tool message answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// openAIPart is one part of a message whose content is a list: a piece of
// text, or a picture given as a data address.
type openAIPart struct {
	// Type is "text" or "image_url".
	Type string `json:"type"`
	// Text is the words of a text part.
	Text string `json:"text,omitempty"`
	// ImageURL is the picture of an image part.
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

// openAIImageURL carries a picture as a data address, which is how the local
// daemon and the API both read one.
type openAIImageURL struct {
	// URL is the data address carrying the picture as base64.
	URL string `json:"url"`
}

// openAIStreamOptions asks the server to report the token counts at the end of
// the stream, which it otherwise leaves out.
type openAIStreamOptions struct {
	// IncludeUsage is always true, because the cost line needs the counts.
	IncludeUsage bool `json:"include_usage"`
}

// openAIBody is the whole request. It carries no sampling fields, because the
// design leaves the sampling to the server.
type openAIBody struct {
	// Model is what the server calls the model.
	Model string `json:"model"`
	// Messages is the system prompt and the conversation.
	Messages []openAIMessage `json:"messages"`
	// Stream is always true, because this provider always streams.
	Stream bool `json:"stream"`
	// StreamOptions asks for the token counts.
	StreamOptions openAIStreamOptions `json:"stream_options"`
	// Tools is what the model may ask for, and is left out when the harness has
	// switched the tools off.
	Tools []openAITool `json:"tools,omitempty"`
	// MaxCompletionTokens is the output cap. It is this field rather than the
	// older max_tokens, because the reasoning models refuse the older one and
	// the local daemon honours this one.
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
	// ReasoningEffort is how hard the model is asked to think, and is left out
	// when nobody asked, so that the server's own default stands.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// ChatTemplateKwargs says whether the model thinks, and is sent only to a
	// local server that was found to understand it.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}
