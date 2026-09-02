// The wire shapes in this file, and the events its stream reader answers, were
// read from Prime Agent's Anthropic provider at
// ~/Code/prime-agent/packages/ai/src/providers/anthropic.ts. That file is
// TypeScript over the vendor's own software development kit; this is Go over
// net/http, so that the harness owns the parser it fuzzes.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// maxCacheMarkers is how many places in one request may carry a cache marker.
// The Messages API allows four, and design section 4 names three boundaries, so
// the fourth is headroom rather than a plan.
const maxCacheMarkers = 4

// anthropicModel is one model reached through the Anthropic Messages API.
type anthropicModel struct {
	alias   contract.ModelAlias
	options Options
}

// newAnthropicModel returns the model an Anthropic alias names.
func newAnthropicModel(alias contract.ModelAlias, options Options) *anthropicModel {
	return &anthropicModel{alias: alias, options: options}
}

// Name is the alias the user gave this model.
func (model *anthropicModel) Name() string { return model.alias.Name }

// ContextLength is how many tokens the model holds on one call.
func (model *anthropicModel) ContextLength() int { return model.alias.ContextLength }

// Send makes one call and streams the reply back.
func (model *anthropicModel) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	body, err := json.Marshal(model.buildBody(request))
	if err != nil {
		return contract.Reply{}, fmt.Errorf("the request to the model %q could not be written as JSON: %w", model.alias.Name, err)
	}
	header := http.Header{}
	header.Set("content-type", "application/json")
	header.Set("anthropic-version", anthropicVersion)
	if model.options.APIKey != "" {
		header.Set("x-api-key", model.options.APIKey)
	}

	stream, err := openStreamedCall(ctx, model.options, model.alias.Name, model.address(), header, body)
	if err != nil {
		return contract.Reply{}, err
	}
	defer stream.finish()
	return readAnthropicStream(stream, model.alias.Name, model.options, onDelta)
}

// address is where the Messages API lives for this alias, which is the public
// one unless the configuration named another.
func (model *anthropicModel) address() string {
	base := model.alias.BaseAddress
	if base == "" {
		base = anthropicPublicAddress
	}
	return strings.TrimSuffix(base, "/") + "/v1/messages"
}

// buildBody turns one harness request into the body the Messages API reads.
func (model *anthropicModel) buildBody(request contract.Request) anthropicBody {
	tools := anthropicTools(request)
	markers := 0
	return anthropicBody{
		Model:     model.alias.ModelName,
		MaxTokens: request.MaxOutputTokens,
		Stream:    true,
		System:    anthropicSystem(request, len(tools) > 0, &markers),
		Messages:  anthropicMessages(request.Messages),
		Tools:     markLastTool(tools, request, &markers),
	}
}

// anthropicSystem turns the system blocks into text blocks, putting a cache
// marker on every block that ends a boundary. Boundary B is left to the tools
// when there are any, because the Messages API reads the tools before the system
// prompt and that is where the tool list ends.
func anthropicSystem(request contract.Request, hasTools bool, markers *int) []anthropicTextBlock {
	blocks := []anthropicTextBlock{}
	for _, block := range request.SystemBlocks {
		written := anthropicTextBlock{Type: "text", Text: block.Text}
		onTools := block.Boundary == contract.CacheBoundaryB && hasTools
		if block.Boundary != contract.CacheBoundaryNone && !onTools && *markers < maxCacheMarkers {
			written.Cache = ephemeralCache()
			*markers++
		}
		blocks = append(blocks, written)
	}
	return blocks
}

// markLastTool puts boundary B's cache marker on the last tool, which is where
// the tool list ends on the wire.
func markLastTool(tools []anthropicTool, request contract.Request, markers *int) []anthropicTool {
	if len(tools) == 0 || *markers >= maxCacheMarkers {
		return tools
	}
	for _, block := range request.SystemBlocks {
		if block.Boundary == contract.CacheBoundaryB {
			tools[len(tools)-1].Cache = ephemeralCache()
			*markers++
			break
		}
	}
	return tools
}

// anthropicTools turns the tool specifications into the shape the API reads, and
// returns nothing at all when the harness has switched the tools off.
func anthropicTools(request contract.Request) []anthropicTool {
	if request.ToolsOff {
		return nil
	}
	tools := []anthropicTool{}
	for _, spec := range request.Tools {
		tools = append(tools, anthropicTool{
			Name:        spec.Name,
			Description: spec.Description,
			InputSchema: schemaForFields(spec.Fields),
		})
	}
	return tools
}

// anthropicMessages turns the conversation into the shape the API reads. A
// message with nothing in it is left out, because the API refuses empty content.
func anthropicMessages(messages []contract.Message) []anthropicMessage {
	written := []anthropicMessage{}
	for _, message := range messages {
		blocks := anthropicBlocksFor(message)
		if len(blocks) == 0 {
			continue
		}
		written = append(written, anthropicMessage{Role: string(message.Role), Content: blocks})
	}
	return written
}

// anthropicBlocksFor turns one message into its blocks, with the tool results
// first, which is the order the API insists on.
func anthropicBlocksFor(message contract.Message) []anthropicBlock {
	blocks := []anthropicBlock{}
	for _, result := range message.ToolResults {
		blocks = append(blocks, anthropicBlock{
			Type:      "tool_result",
			ToolUseID: result.CallID,
			Content:   result.Text,
			IsError:   result.Failed,
		})
	}
	if message.Text != "" {
		blocks = append(blocks, anthropicBlock{Type: "text", Text: message.Text})
	}
	for _, call := range message.ToolCalls {
		input := call.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: input})
	}
	return blocks
}
