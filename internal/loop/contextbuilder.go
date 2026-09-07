package loop

import (
	"context"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
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
	// StandingOrder is the text of the work folder's NERDGENIE.md, or empty when
	// the folder has none. It is read once when the task starts.
	StandingOrder string
	// RecentWork is the few tasks most recently finished, newest first, so the
	// model can say where things stand even before this task has made a record
	// of its own. It is gathered once when the task starts and rides on every
	// call the task makes.
	RecentWork []workingcontext.RecentTask
	// Messages are the recent messages and tool results, oldest first.
	Messages []contract.Message
	// Tools is the specification of every tool this turn may use.
	Tools []contract.ToolSpec
	// Pinned is the evidence that never leaves the window until the model
	// unpins it, however small the window is.
	Pinned []workingcontext.Pin
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

// Build asks the real builder for one call's prompt. A call with the tools off
// keeps the tools on the request and says so with the flag, so that the
// prompt's front is the same bytes as the working calls' and the provider
// only tells the model not to call: on 6 September 2026 the done check and
// the review dropped the tools from the wire, the prompt then differed from
// the tool list on, and the daemon's cache was lost twenty-five times with
// the conversation unchanged.
func (real realBuilder) Build(ctx context.Context, input BuildInput) (contract.Request, error) {
	held := contract.Record{}
	if input.Record != nil {
		held = *input.Record
	}
	request, err := real.builder.Build(ctx, workingcontext.BuildInput{
		ContextLength: input.ContextLength,
		Record:        held,
		JobSummary:    input.JobSummary,
		StandingOrder: input.StandingOrder,
		RecentWork:    input.RecentWork,
		Messages:      input.Messages,
		Pinned:        input.Pinned,
		Tools:         input.Tools,
		MemoryHint:    input.MemoryHint,
	})
	if err != nil {
		return contract.Request{}, err
	}
	request.ToolsOff = input.ToolsOff
	return request, nil
}
