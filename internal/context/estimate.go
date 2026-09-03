package context

import "github.com/JaredTate/coeus/internal/contract"

// The three numbers this package counts tokens with. They live here together,
// because the size of a prompt is measured the same way everywhere in Coeus and
// there is only one place to change it.
//
// It is an estimate and not a tokenizer. Every model has its own tokenizer, the
// harness has to work on all of them, and a wrong estimate costs a few tokens of
// window rather than a broken prompt. The numbers were measured against a real
// call: the forty-step fixture at round forty, built for the local Qwen and sent
// through llama-server, is 19,961 characters of text over 82 messages with 43
// tool calls and 43 results in them, and the daemon reported reading 8,013
// tokens. The estimate below comes to 8,439 for the same prompt, which is five
// per cent high. High is the safe side: an estimate that ran low would build a
// prompt the model refuses, and one that runs high only leaves a little of the
// window unused. The live test measures it again on every wave gate.
const (
	// TokensPerHundredCharacters is the ratio the text itself is counted at.
	// Three characters to a token is denser than the four a token of plain
	// English usually holds, because an agent's prompt is prose mixed with JSON,
	// file paths, and identifiers, and those tokenize far more densely.
	TokensPerHundredCharacters = 33
	// TokensPerMessage is what each wire protocol wraps one message in: the
	// role, the envelope around the content, and the marks a chat template puts
	// between one message and the next.
	TokensPerMessage = 8
	// TokensPerToolPart is what one tool call or one tool result costs in
	// envelope on top of its own text: the identifier, the type, and the names
	// of the fields around it.
	TokensPerToolPart = 12
)

// EstimateTokens counts the tokens in one piece of text, rounding up so that a
// short piece never counts as nothing at all. The count is made over bytes, so
// text in a script that takes more than one byte to a character counts high,
// which is again the safe side.
func EstimateTokens(text string) int {
	return (len(text)*TokensPerHundredCharacters + 99) / 100
}

// EstimateRequestTokens counts everything the model reads on one call: the
// system blocks, the messages with their tool calls and results, and the
// specification of every tool, each with the envelope its wire protocol wraps it
// in.
func EstimateRequestTokens(request contract.Request) int {
	counted := 0
	for _, block := range request.SystemBlocks {
		counted += EstimateTokens(block.Text)
	}
	for _, message := range request.Messages {
		counted += estimateMessage(message)
	}
	for _, spec := range request.Tools {
		counted += estimateToolSpec(spec)
	}
	return counted
}

// estimateMessage counts one message: what was said, the tool calls with the
// arguments the model wrote, the results that came back, and the envelope around
// all of it.
func estimateMessage(message contract.Message) int {
	counted := TokensPerMessage + EstimateTokens(message.Text)
	for _, call := range message.ToolCalls {
		counted += TokensPerToolPart + EstimateTokens(call.Name) + EstimateTokens(string(call.Input))
	}
	for _, result := range message.ToolResults {
		counted += TokensPerToolPart + EstimateTokens(result.CallID) + EstimateTokens(result.Text)
	}
	return counted
}

// estimateToolSpec counts one tool as the model sees it: its name, its
// description, the name, type, and description of every field, and the schema
// the provider writes around them.
func estimateToolSpec(spec contract.ToolSpec) int {
	counted := TokensPerToolPart + EstimateTokens(spec.Name) + EstimateTokens(spec.Description)
	for _, field := range spec.Fields {
		counted += TokensPerToolPart + EstimateTokens(field.Name) +
			EstimateTokens(field.Type) + EstimateTokens(field.Description)
	}
	return counted
}
