// Package context builds the working context for one model call from the
// layers of the design, sized to the model.
//
// The working context is the text actually sent to the model on one call. It is
// built fresh every turn and thrown away afterwards, which is what lets a task
// be put down for days and picked up on a different model. This package builds
// it in the order design section 4 lays out, from the part that changes least to
// the part that changes most: the harness rules, SOUL.md and the list of skills,
// ending cache boundary A; the tools, ending boundary B; the summary of the job
// when the task belongs to one; a few recent tasks when the caller passes them,
// for the times the current record is empty; and the record's goal and rules.
// Boundary C ends this stable prefix, on the record's goal and rules or, when
// there is no record yet, on the recent-work block instead. Everything above
// boundary C goes into the system prompt and is byte-identical from one turn to
// the next, so the provider can reuse it.
//
// Everything below that line is ordered by one question: does this block only
// grow at its end, or is it written anew? A prompt cache keeps what two calls
// share from the first byte and stops at the first byte that differs, so a block
// that is rewritten drags everything under it along with it, while a block that
// only grows costs nothing until the point where it grew. So the blocks that
// only grow come first — what the agent knows from USER.md and MEMORY.md, the
// pinned evidence, and the messages and tool results oldest first — and then the
// tail, which is everything a turn writes anew: the record's work and lessons,
// whose situation the loop rewrites after every round, the record's list of
// results, which grows by a line every round, the three lines of memory hint,
// and the record's header, whose budget line and cost line are written afresh
// every single call.
//
// The numbers are why the tail is where it is. The record's body used to be the
// first thing under the cache line, above the conversation. Measured on the
// local llama-server daemon, one Coeus call then read 18,658
// prompt tokens of which the daemon reused 4,322, which is the system blocks and
// not one byte more: 14,336 tokens read from scratch, forty seconds of prefill
// for forty-five generated tokens, on a task another agent finished in two
// minutes reading about six hundred tokens a call. With the body in the tail,
// two rounds of the forty-step fixture that follow each other share 82, 85 and
// 86 per cent of the prompt at rounds 10, 20 and 30, where they shared 33, 24
// and 19.
//
// What the agent knows sits at the top of that run rather than in the tail
// because USER.md and MEMORY.md hold still through almost every task: up there
// they are read once and reused for nothing afterwards, and only the rare turn
// that saves a fact pays for the conversation under them.
//
// One rule sizes the whole thing. The window left for results is the model's
// context length, less the output cap, less everything above the cache line,
// less the record's live part. Results fill that window newest first and in
// full, and when it is full the oldest result's whole text leaves. Its line in
// the record and its copy in the event log stay behind, so "read r7" brings it
// back. Nothing is ever summarized and nothing is ever rewritten. Pinned
// evidence never leaves, and a set of pins too large for the window is an error
// naming the pin to drop rather than a prompt the model cannot be sent.
//
// Every piece of text that did not come from the user is wrapped in a data
// marker carrying a random identifier made once per task, because words inside a
// web page or a file are never instructions. That covers the tool results in the
// messages, the pinned evidence, and the record's own list of results, which
// holds the first line of every tool result the task has seen.
package context
