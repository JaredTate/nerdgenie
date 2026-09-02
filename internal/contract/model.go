package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ProviderKind names the wire protocol a model provider speaks.
type ProviderKind string

const (
	// ProviderAnthropic is the Anthropic Messages API.
	ProviderAnthropic ProviderKind = "anthropic"
	// ProviderOpenAICompatible is the OpenAI Chat Completions API at a base
	// address, which covers OpenAI itself, llama-server, LM Studio, Ollama, and
	// every cloud gateway.
	ProviderOpenAICompatible ProviderKind = "openai-compatible"
)

// CacheBoundary marks a place in the system prompt where everything above may be
// reused from the provider's cache on the next call.
//
// Design section 4 names three of them. A is after the harness rules and the
// persona, B is after the tools, and C is after the record's goal and rules.
// Each provider decides how to express a boundary on the wire.
type CacheBoundary string

const (
	// CacheBoundaryNone means the block ends no cache boundary.
	CacheBoundaryNone CacheBoundary = ""
	// CacheBoundaryA ends the harness rules and the persona.
	CacheBoundaryA CacheBoundary = "A"
	// CacheBoundaryB ends the tool list.
	CacheBoundaryB CacheBoundary = "B"
	// CacheBoundaryC ends the record's goal and rules.
	CacheBoundaryC CacheBoundary = "C"
)

// SystemBlock is one piece of the system prompt, in the order the harness builds
// it: the harness rules and persona, the tools, the summary of the job when the
// task belongs to one, and the record's goal and rules.
type SystemBlock struct {
	// Name says what the block holds, for the cost line and for tests.
	Name string
	// Text is the block's content.
	Text string
	// Boundary marks a cache boundary at the end of this block, or is empty.
	Boundary CacheBoundary
}

// Role says who wrote a message in the working context.
type Role string

const (
	// RoleUser marks a message from the person or from a tool result.
	RoleUser Role = "user"
	// RoleAssistant marks a message the model wrote.
	RoleAssistant Role = "assistant"
)

// ToolCall is the model asking the harness to run one tool.
type ToolCall struct {
	// ID is the provider's identifier for the call, echoed back on the result.
	ID string
	// Name is the tool's name.
	Name string
	// Input is the arguments as the model wrote them, still unparsed.
	Input json.RawMessage
}

// ToolResult is what the harness gives back for one tool call.
type ToolResult struct {
	// CallID is the identifier of the call this answers.
	CallID string
	// Text is the result, already capped and marked as data rather than
	// instructions.
	Text string
	// Failed says the tool reported an error rather than a result.
	Failed bool
}

// Message is one turn of the conversation inside the working context.
type Message struct {
	// Role says who wrote it.
	Role Role
	// Text is the plain text of the message, which may be empty when the message
	// carries only tool calls or tool results.
	Text string
	// ToolCalls are the calls the model asked for, on an assistant message.
	ToolCalls []ToolCall
	// ToolResults are the results the harness returned, on a user message.
	ToolResults []ToolResult
}

// Request is everything the harness sends the model on one call.
type Request struct {
	// SystemBlocks is the system prompt in order, with its cache boundaries.
	SystemBlocks []SystemBlock
	// Messages is the working context's recent conversation.
	Messages []Message
	// Tools is the specification of every tool the model may call.
	Tools []ToolSpec
	// ToolsOff turns the tools off, which is how the harness asks for the final
	// report when a task's budget has run out.
	ToolsOff bool
	// MaxOutputTokens caps the reply.
	MaxOutputTokens int
}

// FinishReason says why the model stopped writing.
type FinishReason string

const (
	// FinishEnd means the model finished its answer.
	FinishEnd FinishReason = "end"
	// FinishToolCalls means the model stopped to ask for tools.
	FinishToolCalls FinishReason = "tool calls"
	// FinishLength means the reply hit the output cap.
	FinishLength FinishReason = "length"
	// FinishStopped means the harness or the provider cut the reply short.
	FinishStopped FinishReason = "stopped"
)

// Usage is the token count the provider reported for one call, which the harness
// writes into the record's cost line every turn.
type Usage struct {
	// InputTokens is everything the provider read.
	InputTokens int
	// CachedInputTokens is how much of the input the provider reused from its
	// cache, and it is a part of InputTokens rather than an addition to it.
	CachedInputTokens int
	// OutputTokens is what the model wrote.
	OutputTokens int
}

// Reply is the whole of what one model call produced.
type Reply struct {
	// Text is the model's plain text, already streamed to the caller in deltas.
	Text string
	// ToolCalls are the tools it asked for.
	ToolCalls []ToolCall
	// Finish says why it stopped.
	Finish FinishReason
	// Usage is the token count for this call.
	Usage Usage
}

// Model is one language model reached through one provider.
//
// Send streams the reply: every piece of text goes to onDelta as it arrives, and
// the full reply comes back at the end. A caller that does not want the deltas
// passes nil. Retries, the fallback chain, and the cache markers are the
// provider package's work, not the caller's.
type Model interface {
	// Name is the alias from the configuration, such as "local".
	Name() string
	// ContextLength is how many tokens the model can hold on one call.
	ContextLength() int
	// Send makes one call and returns the whole reply.
	Send(ctx context.Context, request Request, onDelta func(delta string)) (Reply, error)
}

// ErrContextOverflow means the request was longer than the model's window. The
// provider never retries it, because a retry would fail the same way.
var ErrContextOverflow = errors.New("the request is longer than the model's context window, so shrink the working context before calling again")

// ErrStalledStream means the provider opened a stream and then sent nothing for
// longer than the harness was willing to wait.
var ErrStalledStream = errors.New("the model stream sent nothing before the deadline, so retry the call or fall back to the next model")

// RateLimitedError means the provider refused the call and said when to try
// again.
type RateLimitedError struct {
	// RetryAfter is how long the provider asked the caller to wait.
	RetryAfter time.Duration
}

// Error says what went wrong and how long to wait.
func (limited RateLimitedError) Error() string {
	return fmt.Sprintf("the provider is rate limiting this key, so wait %s and try again", limited.RetryAfter)
}
