// The wire shapes in this file, and the way the streamed tool calls are put back
// together, were read from Prime Agent's chat-completions provider at
// ~/Code/prime-agent/packages/ai/src/providers/openai-completions.ts. That file
// reads the stream through the vendor's software development kit; this is Go
// over net/http, so that the harness owns the parser it fuzzes.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// failedToolResultPrefix is put in front of a tool result that failed. This API
// has no field for it, unlike the Messages API, and a model that cannot tell a
// failure from a result will act on the failure as though it worked.
const failedToolResultPrefix = "This tool call failed.\n"

// openAIModel is one model reached through the OpenAI-compatible Chat
// Completions API at a base address, which covers OpenAI itself, the
// llama-server daemon, LM Studio, Ollama, and every cloud gateway.
type openAIModel struct {
	alias   contract.ModelAlias
	options Options
	// contextLength is the window the model really has, which is the configured
	// one unless a local server reported a smaller one at construction.
	contextLength int
	// thinkingOff says to send the thinking-off hint, which only a local server
	// that was found to understand it is given.
	thinkingOff bool
}

// newOpenAIModel returns the model an OpenAI-compatible alias names, probing a
// local server once for its real window and its thinking-off hint.
func newOpenAIModel(alias contract.ModelAlias, options Options) (*openAIModel, error) {
	if alias.BaseAddress == "" {
		return nil, fmt.Errorf("the model alias %q names no base address, so add the address of the server, such as %q",
			alias.Name, "http://127.0.0.1:19091/v1")
	}
	model := &openAIModel{alias: alias, options: options, contextLength: alias.ContextLength}
	found, answered := probeLocalServer(alias.BaseAddress)
	if !answered {
		return model, nil
	}
	model.thinkingOff = true
	if found.contextLength > 0 && found.contextLength < alias.ContextLength {
		options.note("the server at %s holds %d tokens, not the %d in config.toml, so the smaller window is used",
			alias.BaseAddress, found.contextLength, alias.ContextLength)
		model.contextLength = found.contextLength
	}
	return model, nil
}

// Name is the alias the user gave this model.
func (model *openAIModel) Name() string { return model.alias.Name }

// ContextLength is how many tokens the model holds on one call.
func (model *openAIModel) ContextLength() int { return model.contextLength }

// Send makes one call and streams the reply back.
func (model *openAIModel) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	shaped, err := model.buildBody(request)
	if err != nil {
		return contract.Reply{}, err
	}
	body, err := json.Marshal(shaped)
	if err != nil {
		return contract.Reply{}, fmt.Errorf("the request to the model %q could not be written as JSON: %w", model.alias.Name, err)
	}
	header := http.Header{}
	header.Set("content-type", "application/json")
	if model.options.APIKey != "" {
		header.Set("Authorization", "Bearer "+model.options.APIKey)
	}

	stream, err := openStreamedCall(ctx, model.options, model.alias.Name, model.address(), header, body)
	if err != nil {
		return contract.Reply{}, err
	}
	defer stream.finish()
	return readOpenAIStream(stream, model.alias.Name, model.options, onDelta)
}

// address is where the Chat Completions API lives for this alias. The base
// address already ends in the version folder, so only the method is added.
func (model *openAIModel) address() string {
	return strings.TrimSuffix(model.alias.BaseAddress, "/") + "/chat/completions"
}

// buildBody turns one harness request into the body the API reads. There are no
// cache markers here: OpenAI and llama-server both reuse a prompt's prefix on
// their own, so a boundary has no form to take on this wire.
func (model *openAIModel) buildBody(request contract.Request) (openAIBody, error) {
	outputTokens, err := outputTokensFor(request, model.alias.Name)
	if err != nil {
		return openAIBody{}, err
	}
	body := openAIBody{
		Model:               model.alias.ModelName,
		Messages:            openAIMessages(request),
		Stream:              true,
		StreamOptions:       openAIStreamOptions{IncludeUsage: true},
		Tools:               openAITools(request),
		MaxCompletionTokens: outputTokens,
	}
	if model.thinkingOff {
		body.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	}
	return body, nil
}

// openAIMessages puts the joined system prompt at the top and then walks the
// conversation, because this API takes the system prompt as the first message
// rather than as a field of its own.
func openAIMessages(request contract.Request) []openAIMessage {
	messages := []openAIMessage{}
	if joined := joinSystemBlocks(request.SystemBlocks); joined != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: joined})
	}
	for _, message := range request.Messages {
		messages = append(messages, openAIMessagesFor(message)...)
	}
	return messages
}

// joinSystemBlocks runs the system blocks together with a blank line between
// them, which is the whole system prompt for a provider with no block shape.
func joinSystemBlocks(blocks []contract.SystemBlock) string {
	pieces := []string{}
	for _, block := range blocks {
		if block.Text != "" {
			pieces = append(pieces, block.Text)
		}
	}
	return strings.Join(pieces, "\n\n")
}

// openAIMessagesFor turns one harness message into the one or more messages this
// API needs, because a result goes back as a message of its own rather than as a
// block inside the user's turn.
func openAIMessagesFor(message contract.Message) []openAIMessage {
	written := []openAIMessage{}
	for _, result := range message.ToolResults {
		text := result.Text
		if result.Failed {
			text = failedToolResultPrefix + text
		}
		written = append(written, openAIMessage{Role: "tool", Content: text, ToolCallID: result.CallID})
	}
	if message.Text == "" && len(message.ToolCalls) == 0 {
		return written
	}
	turn := openAIMessage{Role: string(message.Role), Content: message.Text}
	for _, call := range message.ToolCalls {
		arguments := string(call.Input)
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		turn.ToolCalls = append(turn.ToolCalls, openAIToolCall{
			ID: call.ID, Type: "function",
			Function: openAICallFunction{Name: call.Name, Arguments: arguments},
		})
	}
	return append(written, turn)
}

// openAITools turns the tool specifications into the shape the API reads, and
// returns nothing at all when the harness has switched the tools off.
func openAITools(request contract.Request) []openAITool {
	if request.ToolsOff {
		return nil
	}
	tools := []openAITool{}
	for _, spec := range request.Tools {
		tools = append(tools, openAITool{Type: "function", Function: openAIFunction{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  schemaForFields(spec.Fields),
		}})
	}
	return tools
}
