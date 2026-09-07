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

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The names of the system blocks, which say what each block holds. They are in
// the order the design's layer table puts them.
const (
	// BlockInstructions holds what the model is told about the harness.
	BlockInstructions = "instructions"
	// BlockPersona holds SOUL.md and the list of skills the model may load.
	BlockPersona = "persona"
	// BlockTools stands for the tool list, which the provider renders.
	BlockTools = "tools"
	// BlockJob holds the summary of the job the task belongs to.
	BlockJob = "job summary"
	// BlockRecentWork holds a few lines on the tasks most recently finished or
	// set down, for the times the current record is empty.
	BlockRecentWork = "recent work"
	// BlockRecord holds the record's goal and rules.
	BlockRecord = "record goal and rules"
)

// The lines that open the parts of the prompt this package writes, so that the
// model knows what it is reading.
const (
	toolsHeading            = "**Your tools.** The tools you may call are sent with this message, each with its name, what it does, and the fields it takes."
	jobHeading              = "**The job this task belongs to.** It changes only when one of its tasks finishes."
	standingOrderHeading    = "**The project's standing order (AGENTS.md, %d lines):** its rules and how to run and test it. The task's own rules come after it and win."
	recordFirstHalfHeading  = "**The task record, part one: the goal and the rules.** These change rarely."
	recordSecondHalfHeading = "**The task record, part two: the work and the lessons.**"
	recordResultsHeading    = "**The task record, part three: every result so far.**"
	recordHeaderHeading     = "**The task record, last of all: where the work stands.**"
	whatIsKnownHeading      = "**What you know, from USER.md and MEMORY.md.** These are yours to add to with the `memory` tool."
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
	// Skills are the name and one-line description of every skill the model
	// may load with the `skill` tool, as the store lists them. They are read
	// once, when the builder is made, because they ride above cache boundary
	// A and nothing up there may move during a task; the wiring makes a
	// builder per task, so a skill added mid-task shows on the next one.
	Skills []contract.SkillSummary
}

// Builder builds the working context for one task, on any model.
type Builder struct {
	home            contract.Home
	memoryCaps      contract.MemoryCaps
	maxOutputTokens int
	boundary        string
	skills          string
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
	// StandingOrder is the text of the work folder's AGENTS.md, the project's
	// rules and how to run and test it, or empty when the folder has none. It
	// rides under the job summary, cut at MaxStandingOrderLines.
	StandingOrder string
	// RecentWork is a few just-finished or set-down tasks, newest first, so the
	// model can say where things stand when the current record is empty. It
	// rides above the record in the caching part of the prompt because it holds
	// still through a sitting. Empty leaves the prompt byte-for-byte as it was.
	RecentWork []RecentTask
	// Pinned is the evidence that never leaves the window.
	Pinned []Pin
	// Messages are the recent messages and tool results, oldest first.
	Messages []contract.Message
	// Tools is the specification of every tool this turn may call.
	Tools []contract.ToolSpec
	// MemoryHint is up to three lines from a search of memory.
	MemoryHint []string
}

// New makes a builder. Everything it takes comes from the configuration and the
// home folder, and none of it changes while the builder lives.
//
// The daemon makes one at startup and shares it across every task, so the
// boundary it makes here is one per process and not, as an earlier draft of this
// comment said, one per task. That is safe, and it is worth saying why rather
// than leaving a reader to hope: the boundary is not a secret the wrapper
// depends on. WrapAsData takes every copy of it out of the text before wrapping,
// so a page that learned one task's boundary and wrote it back in the next gains
// nothing, which TestABoundaryLearnedInOneTaskCannotBreakOutOfAnother holds.
// Making it truly per task needs one line where the daemon builds the loop's
// dependencies, not a change here.
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
		skills:          skillsText(options.Skills),
	}, nil
}

// Boundary is the identifier this builder's tool results are marked with. The
// turn loop reads it so that it can mark a result it hands over outside a built
// context with the same one. See New for why one boundary serving every task of
// a run is safe.
func (builder *Builder) Boundary() string { return builder.boundary }

// Build makes the working context for one call. The same input always builds the
// same bytes, and everything above cache boundary C is byte-identical from one
// turn to the next unless the persona files, the tools, the job, or the record's
// goal and rules have actually changed.
func (builder *Builder) Build(ctx context.Context, input BuildInput) (contract.Request, error) {
	if err := ctx.Err(); err != nil {
		return contract.Request{}, fmt.Errorf("the turn was given up on before its working context was built: %w", err)
	}
	persona, err := readPersona(builder.home)
	if err != nil {
		return contract.Request{}, err
	}
	persona = joinBlocks(persona, builder.skills)
	known, err := readWhatIsKnown(builder.home, builder.memoryCaps)
	if err != nil {
		return contract.Request{}, err
	}
	parts := splitRecord(input.Record)

	request := contract.Request{
		SystemBlocks:    builder.systemBlocks(persona, input),
		Tools:           input.Tools,
		MaxOutputTokens: builder.maxOutputTokens,
	}
	messages, err := builder.messagesFor(input, parts, known, request)
	if err != nil {
		return contract.Request{}, err
	}
	request.Messages = messages
	return request, nil
}

// systemBlocks builds everything above the cache line, in the order of the
// design's layer table, with the cache boundaries on the blocks that end each
// layer. Boundary A ends the rules and the persona, and B ends the tools, which
// is where the stable prefix ends since 7 September 2026.
//
// The job summary, the recent work and the record's goal and rules rode here
// too until then, and on 6 September that cost every task start the whole
// tool list: the wire puts the tools after the system prompt, so a system
// prompt that changes per task pushes the tools out of the cache, and 25 of 32
// task starts re-read ten to fourteen thousand tokens. They ride as the first
// messages below the tools now, in perTaskFront, where a new task costs only
// their own size. With no tools the prefix ends at the persona and boundary A.
//
// A layer with nothing in it is left out rather than sent as an empty block,
// because an empty block is refused on the wire.
func (builder *Builder) systemBlocks(persona string, input BuildInput) []contract.SystemBlock {
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
	return blocks
}

// joinBlocks puts two pieces of one system block together with a blank line
// between them, and leaves out whichever of them is empty, so that a block never
// begins or ends with a blank line.
func joinBlocks(first string, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "\n\n" + second
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
