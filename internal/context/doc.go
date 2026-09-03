// Package context builds the working context for one model call from the
// layers of the design, sized to the model.
//
// The working context is the text actually sent to the model on one call. It is
// built fresh every turn and thrown away afterwards, which is what lets a task
// be put down for days and picked up on a different model. This package builds
// it in the order design section 4 lays out, from the part that changes least to
// the part that changes most: the harness rules and SOUL.md, ending cache
// boundary A; the tools, ending boundary B; the summary of the job when the task
// belongs to one; and the record's goal and rules, ending boundary C. Everything
// above boundary C goes into the system prompt and is byte-identical from one
// turn to the next, so the provider can reuse it. Everything below it goes into
// the messages in the design's own order — the record's work and lessons, any
// pinned evidence, the recent messages and tool results, what the agent knows
// from USER.md and MEMORY.md, three lines of memory hint — followed by a tail
// holding the two parts that change on every call: the
// record's list of results, which grows by a line every round, and the record's
// header, whose budget line and cost line are written anew every time. The tail
// exists because a prompt cache keeps what two calls share from the first byte
// and stops at the first byte that differs, so anything that changes drags
// everything under it along with it.
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
