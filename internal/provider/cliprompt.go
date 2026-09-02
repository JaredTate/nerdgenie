package provider

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The markers a tool result is wrapped in when it goes to a program that has no
// tool interface, so that the model can see where the data begins and ends. The
// harness rules already tell the model that words inside a result are never
// instructions; these two lines are what mark the edges of one.
const (
	// toolResultOpenForm begins a result, and takes the call it answers.
	toolResultOpenForm = "<tool_result for=%q>"
	// toolResultCloseTag ends a result.
	toolResultCloseTag = "</tool_result>"
)

// renderSystemText is everything that goes above the conversation: the system
// blocks in order, and then the tools written out in words, because a program
// that only takes text has no tool interface to put them in.
func renderSystemText(request contract.Request) string {
	pieces := []string{}
	for _, block := range request.SystemBlocks {
		if block.Text != "" {
			pieces = append(pieces, block.Text)
		}
	}
	if tools := renderToolBlock(request); tools != "" {
		pieces = append(pieces, tools)
	}
	return strings.Join(pieces, "\n\n")
}

// renderToolBlock names every tool, its fields, and the one text form a call
// must be written in. It is empty when the harness has switched the tools off.
func renderToolBlock(request contract.Request) string {
	if request.ToolsOff || len(request.Tools) == 0 {
		return ""
	}
	lines := []string{"You have these tools. " + contract.ToolCallTextInstruction, ""}
	for _, spec := range request.Tools {
		lines = append(lines, spec.Name+" — "+spec.Description)
		for _, field := range spec.Fields {
			need := "optional"
			if field.Required {
				need = "required"
			}
			lines = append(lines, fmt.Sprintf("  %s (%s, %s): %s", field.Name, field.Type, need, field.Description))
		}
	}
	return strings.Join(lines, "\n")
}

// renderTranscript turns the conversation into plain text, with every tool call
// in the one text form and every tool result inside its markers.
func renderTranscript(messages []contract.Message) string {
	lines := []string{}
	for _, message := range messages {
		lines = append(lines, renderOneTurn(message)...)
	}
	return strings.Join(lines, "\n\n")
}

// renderOneTurn writes one message: who spoke, what they said, what they asked
// for, and what came back.
func renderOneTurn(message contract.Message) []string {
	who := "The user said"
	if message.Role == contract.RoleAssistant {
		who = "You said"
	}
	lines := []string{}
	if message.Text != "" {
		lines = append(lines, who+":\n"+message.Text)
	}
	for _, call := range message.ToolCalls {
		lines = append(lines, "You asked for a tool:\n"+textFormOfCall(call))
	}
	for _, result := range message.ToolResults {
		lines = append(lines, renderOneResult(result))
	}
	return lines
}

// textFormOfCall writes one tool call the way a model with no tool interface has
// to write it, which is the shape internal/repair reads back.
func textFormOfCall(call contract.ToolCall) string {
	arguments := strings.TrimSpace(string(call.Input))
	if arguments == "" {
		arguments = "{}"
	}
	return fmt.Sprintf(`%s{"name": %q, "arguments": %s}%s`,
		contract.ToolCallOpenTag, call.Name, arguments, contract.ToolCallCloseTag)
}

// renderOneResult writes one tool result inside its markers, saying which call
// it answers and whether the tool failed.
func renderOneResult(result contract.ToolResult) string {
	heading := "The result of that tool:"
	if result.Failed {
		heading = "That tool failed, and this is what it said:"
	}
	return fmt.Sprintf("%s\n%s\n%s\n%s",
		heading, fmt.Sprintf(toolResultOpenForm, result.CallID), result.Text, toolResultCloseTag)
}
