package context

import "github.com/JaredTate/coeus/internal/contract"

// CharactersPerToken is the one ratio this package counts tokens with, and it is
// deliberately the only one here, so that every part of a prompt is measured the
// same way. Four characters to a token is the rule of thumb for English written
// as plain text, and it is what the window rule is sized by.
//
// It is an estimate and not a tokenizer. Every model has its own tokenizer, the
// harness has to work on all of them, and a wrong estimate costs a few tokens of
// window rather than a broken prompt. The count is made over bytes, so text in a
// script that takes more than one byte to a character is counted high, which
// leaves the window smaller than it needs to be rather than larger. The record
// package keeps its own estimate, over words, for the size of a record alone; a
// test here compares the two on the forty-step fixture and they agree.
const CharactersPerToken = 4

// EstimateTokens counts the tokens in one piece of text, rounding up so that a
// short piece never counts as nothing at all.
func EstimateTokens(text string) int {
	return (len(text) + CharactersPerToken - 1) / CharactersPerToken
}

// EstimateRequestTokens counts everything the model reads on one call: the
// system blocks, the messages with their tool calls and results, and the
// specification of every tool. It does not count the framing each wire protocol
// wraps those in, which is a few tokens a message.
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
// arguments the model wrote, and the results that came back.
func estimateMessage(message contract.Message) int {
	counted := EstimateTokens(message.Text)
	for _, call := range message.ToolCalls {
		counted += EstimateTokens(call.Name) + EstimateTokens(string(call.Input))
	}
	for _, result := range message.ToolResults {
		counted += EstimateTokens(result.CallID) + EstimateTokens(result.Text)
	}
	return counted
}

// estimateToolSpec counts one tool as the model sees it: its name, its
// description, and the name, type, and description of every field.
func estimateToolSpec(spec contract.ToolSpec) int {
	counted := EstimateTokens(spec.Name) + EstimateTokens(spec.Description)
	for _, field := range spec.Fields {
		counted += EstimateTokens(field.Name) + EstimateTokens(field.Type) + EstimateTokens(field.Description)
	}
	return counted
}
