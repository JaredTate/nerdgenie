package loop

import (
	"context"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
)

// BuildInput is everything one call's working context is built from, in the
// order the design's layer table puts them.
type BuildInput struct {
	// Record is the task record, or nil before the first tool call has made
	// one, because a question the agent can answer with no tools gets an answer
	// and no record.
	Record *contract.Record
	// ContextLength is how many tokens the model being called can hold, which
	// is the one number the window is sized from.
	ContextLength int
	// JobSummary is the short form of the job this task belongs to, and is
	// empty when the task belongs to no job.
	JobSummary string
	// Messages are the recent messages and tool results, oldest first.
	Messages []contract.Message
	// Tools is the specification of every tool this turn may use.
	Tools []contract.ToolSpec
	// ToolsOff turns the tools off, which is how the harness asks for the final
	// report and the four review questions.
	ToolsOff bool
	// MemoryHint is the few lines from memory that ride at the end of the
	// prompt.
	MemoryHint []string
}

// ContextBuilder turns one BuildInput into the request the model is sent.
//
// The builder that does the work is internal/context, written against the layer
// table and the cache boundaries of design section 4; TheWorkingContext wraps
// one for the loop. The loop is written against this one method so that a test
// can hand it a builder of its own, and so that serve.go can swap the real one
// in without the loop knowing.
type ContextBuilder interface {
	// Build assembles the working context for one call.
	Build(ctx context.Context, input BuildInput) (contract.Request, error)
}

// TheWorkingContext hands the loop the real working-context builder. The two
// things the loop knows and that builder does not are how big the model's window
// is and whether the tools are off this call, and this is where they are put in.
func TheWorkingContext(builder *workingcontext.Builder) ContextBuilder {
	return realBuilder{builder: builder}
}

// realBuilder is the working-context builder as the loop uses it.
type realBuilder struct {
	builder *workingcontext.Builder
}

// Build asks the real builder for one call's prompt, with the tools left out
// when this is the call that asks for a report with the tools off.
func (real realBuilder) Build(ctx context.Context, input BuildInput) (contract.Request, error) {
	held := contract.Record{}
	if input.Record != nil {
		held = *input.Record
	}
	tools := input.Tools
	if input.ToolsOff {
		tools = nil
	}
	request, err := real.builder.Build(ctx, workingcontext.BuildInput{
		ContextLength: input.ContextLength,
		Record:        held,
		JobSummary:    input.JobSummary,
		Messages:      input.Messages,
		Tools:         tools,
		MemoryHint:    input.MemoryHint,
	})
	if err != nil {
		return contract.Request{}, err
	}
	request.ToolsOff = input.ToolsOff
	return request, nil
}
