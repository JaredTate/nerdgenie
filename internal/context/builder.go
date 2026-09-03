// The layered prompt with a small fixed number of cache points, placed by the
// builder rather than worked out from the text at request time, is Hermes'
// design, at ~/Code/hermes-agent/agent/prompt_caching.py and
// ~/Code/hermes-agent/agent/prompt_cache_boundary.py, where only the code that
// assembled a prompt knows where its stable part ends and it says so rather than
// leaving a later pass to guess. OpenClaw does the same thing with one line in
// ~/Code/openclaw/src/agents/system-prompt.ts, where SYSTEM_PROMPT_CACHE_BOUNDARY
// closes the stable prefix and everything that changes per turn is pushed below
// it. The rule that the user's own words are carried word for word and never
// summarized is Hermes' too, from
// ~/Code/hermes-agent/agent/context_compressor.py; here it covers the whole
// record rather than only the messages. The Go is written fresh.

package context

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The names of the system blocks, which say what each block holds. They are in
// the order the design's layer table puts them.
const (
	// BlockInstructions holds what the model is told about the harness.
	BlockInstructions = "instructions"
	// BlockPersona holds the three persona files.
	BlockPersona = "persona"
	// BlockTools stands for the tool list, which the provider renders.
	BlockTools = "tools"
	// BlockJob holds the summary of the job the task belongs to.
	BlockJob = "job summary"
	// BlockRecord holds the record's goal and rules.
	BlockRecord = "record goal and rules"
)

// The lines that open the parts of the prompt this package writes, so that the
// model knows what it is reading.
const (
	toolsHeading            = "**Your tools.** The tools you may call are sent with this message, each with its name, what it does, and the fields it takes."
	jobHeading              = "**The job this task belongs to.** It changes only when one of its tasks finishes."
	recordFirstHalfHeading  = "**The task record, part one: the goal and the rules.** These change rarely."
	recordSecondHalfHeading = "**The task record, part two: the header, the work, and the lessons.** These change every turn."
	pinnedHeading           = "**Pinned evidence, kept word for word.** It stays in front of you until it is unpinned."
	memoryHintHeading       = "**Memory hint.** Up to three lines from a search of what you know."
)

// Pin is one piece of evidence that never leaves the window until somebody
// unpins it, however small the window is.
type Pin struct {
	// ID is the result the evidence came from, such as "r7".
	ID string
	// Text is the evidence itself, word for word.
	Text string
}

// Options is what the builder needs for the whole of one task.
type Options struct {
	// Home is the agent's home folder, where the three persona files live.
	Home contract.Home
	// MemoryCaps are the size limits the two persona memory files are cut at.
	MemoryCaps contract.MemoryCaps
	// MaxOutputTokens is the cap on what the model may write on one call, which
	// comes out of the window before anything else does.
	MaxOutputTokens int
	// Boundary is the identifier every tool result is marked with. Leave it
	// empty and the builder makes a random one, which is what the running
	// program does; a test sets it so that its golden files do not change on
	// every run.
	Boundary string
}

// Builder builds the working context for one task, on any model.
type Builder struct {
	home            contract.Home
	memoryCaps      contract.MemoryCaps
	maxOutputTokens int
	boundary        string
}

// BuildInput is everything one turn hands the builder.
type BuildInput struct {
	// ContextLength is how many tokens the model being called can hold, which is
	// the one number the window is sized from.
	ContextLength int
	// Record is the task record. A record that has not been made yet, which is
	// what the first turn holds, leaves the record out of the prompt.
	Record contract.Record
	// JobSummary is the job's goal, rules, and task list, printed by the record
	// package, or empty when the task stands on its own.
	JobSummary string
	// Pinned is the evidence that never leaves the window.
	Pinned []Pin
	// Messages are the recent messages and tool results, oldest first.
	Messages []contract.Message
	// Tools is the specification of every tool this turn may call.
	Tools []contract.ToolSpec
	// MemoryHint is up to three lines from a search of memory.
	MemoryHint []string
}

// New makes a builder for one task. Everything it takes comes from the
// configuration and the home folder, and none of it changes while the task runs.
func New(options Options) (*Builder, error) {
	if options.Home.Root == "" {
		return nil, errors.New("a working context needs the home folder the persona files live in, so pass the home the configuration package built")
	}
	if options.MemoryCaps.UserFactsBytes <= 0 || options.MemoryCaps.WorldFactsBytes <= 0 {
		return nil, fmt.Errorf("the memory caps are %d bytes for USER.md and %d for MEMORY.md, and both have to be above zero, so pass the caps from the configuration",
			options.MemoryCaps.UserFactsBytes, options.MemoryCaps.WorldFactsBytes)
	}
	if options.MaxOutputTokens <= 0 {
		return nil, fmt.Errorf("the output cap is %d tokens, and it has to be above zero, so pass the one from the configuration", options.MaxOutputTokens)
	}
	boundary := options.Boundary
	if boundary == "" {
		made, err := NewBoundary()
		if err != nil {
			return nil, err
		}
		boundary = made
	}
	return &Builder{
		home:            options.Home,
		memoryCaps:      options.MemoryCaps,
		maxOutputTokens: options.MaxOutputTokens,
		boundary:        boundary,
	}, nil
}

// Boundary is the identifier this task's tool results are marked with. The turn
// loop reads it so that it can mark a result it hands over outside a built
// context with the same one.
func (builder *Builder) Boundary() string { return builder.boundary }

// Build makes the working context for one call. The same input always builds the
// same bytes, and everything above cache boundary C is byte-identical from one
// turn to the next unless the persona files, the tools, the job, or the record's
// goal and rules have actually changed.
func (builder *Builder) Build(ctx context.Context, input BuildInput) (contract.Request, error) {
	if err := ctx.Err(); err != nil {
		return contract.Request{}, fmt.Errorf("the turn was given up on before its working context was built: %w", err)
	}
	persona, err := readPersona(builder.home, builder.memoryCaps)
	if err != nil {
		return contract.Request{}, err
	}
	stable, live := splitRecord(input.Record)

	request := contract.Request{
		SystemBlocks:    builder.systemBlocks(persona, input, stable),
		Tools:           input.Tools,
		MaxOutputTokens: builder.maxOutputTokens,
	}
	messages, err := builder.messagesFor(input, live, request)
	if err != nil {
		return contract.Request{}, err
	}
	request.Messages = messages
	return request, nil
}

// systemBlocks builds everything above the cache line, in the order of the
// design's layer table, with the three cache boundaries on the blocks that end
// each layer. Boundary A ends the rules and the persona, B ends the tools, and C
// ends the record's goal and rules.
//
// A layer with nothing in it is left out rather than sent as an empty block,
// because an empty block is refused on the wire.
func (builder *Builder) systemBlocks(persona string, input BuildInput, stable string) []contract.SystemBlock {
	blocks := []contract.SystemBlock{{Name: BlockInstructions, Text: InstructionText}}
	if persona != "" {
		blocks = append(blocks, contract.SystemBlock{Name: BlockPersona, Text: persona})
	}
	blocks[len(blocks)-1].Boundary = contract.CacheBoundaryA

	if len(input.Tools) > 0 {
		blocks = append(blocks, contract.SystemBlock{
			Name: BlockTools, Text: toolsHeading, Boundary: contract.CacheBoundaryB,
		})
	}
	if input.JobSummary != "" {
		blocks = append(blocks, contract.SystemBlock{
			Name: BlockJob, Text: jobHeading + "\n\n" + input.JobSummary,
		})
	}
	if stable != "" {
		blocks = append(blocks, contract.SystemBlock{
			Name: BlockRecord, Text: recordFirstHalfHeading + "\n\n" + stable, Boundary: contract.CacheBoundaryC,
		})
	}
	return blocks
}

// asUserMessage is one piece of the prompt below the cache line, written as a
// message from the user, which is the only role the harness may write in.
func asUserMessage(heading string, body string) contract.Message {
	return contract.Message{Role: contract.RoleUser, Text: heading + "\n\n" + body}
}

// pinnedText is the pinned evidence, each pin marked as data so that words
// inside a page the user pinned are still never instructions.
func pinnedText(pins []Pin, boundary string) string {
	parts := make([]string, 0, len(pins))
	for _, pin := range pins {
		parts = append(parts, WrapAsData(boundary, "pinned "+pin.ID+"\n"+pin.Text))
	}
	return strings.Join(parts, "\n\n")
}

// memoryHintText is the three lines of memory that ride at the end of the
// prompt. More lines than the design allows are cut, because a hint that grows
// is a hint nobody budgeted for.
func memoryHintText(lines []string) string {
	if len(lines) > contract.MemoryHintLines {
		lines = lines[:contract.MemoryHintLines]
	}
	return "- " + strings.Join(lines, "\n- ")
}
