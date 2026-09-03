package context

import (
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// ErrNoRoomForTheRecord means the model cannot hold even the things that must be
// in every prompt. There is nothing the builder can drop, so the caller has to
// use a model with a longer window.
var ErrNoRoomForTheRecord = errors.New("the rules, the persona, the tools and the task record do not fit in this model's window, so use a model with a longer context")

// ErrPinnedEvidenceTooLarge means the pins alone fill the window. A pin never
// leaves on its own, so the build refuses and names one to let go of rather than
// quietly dropping something the user asked to keep.
var ErrPinnedEvidenceTooLarge = errors.New("the pinned evidence alone does not fit in this model's window, so unpin something")

// messagesFor builds everything below the cache line and applies the one rule of
// the design: the window is the model's context length, less the output cap,
// less everything above the cache line, less the parts of the record that can
// never be dropped. What is left is filled with the most recent results, in
// full, newest first.
//
// The order is the order of how often each part changes, so that a provider's
// prompt cache can reuse as much as possible. First what only grows or barely
// moves: what the agent knows from USER.md and MEMORY.md, the pinned evidence,
// and the messages and tool results oldest first, which are only ever appended
// to. Then the tail, which is everything a turn writes anew: the record's body,
// whose situation the loop rewrites after every round, the list of results,
// which grows by a line every round, the memory hint, and last of all the two
// lines of the record's header.
//
// The record's body used to sit above the messages, and a measurement on the
// local daemon says what that cost. On one Coeus call it reported 18,658 prompt
// tokens of which 4,322 were reused, which is the system blocks and not one
// byte more, so 14,336 tokens were read from scratch: forty seconds of prefill
// for forty-five generated tokens, while opencode on the same model and the
// same task read about six hundred tokens a call. A block that is rewritten
// every turn drags everything under it along with it, so a growing,
// otherwise-identical conversation under the body was thrown away on every
// call.
func (builder *Builder) messagesFor(input BuildInput, parts recordParts, known string, request contract.Request) ([]contract.Message, error) {
	above := EstimateRequestTokens(request)
	recordBody := []contract.Message{}
	if parts.Body != "" {
		recordBody = append(recordBody, asUserMessage(recordSecondHalfHeading, parts.Body))
	}
	recordResults := []contract.Message{}
	if parts.Results != "" {
		recordResults = append(recordResults, asUserMessage(recordResultsHeading, parts.Results))
	}
	recordHeader := []contract.Message{}
	if parts.Standing != "" {
		recordHeader = append(recordHeader, asUserMessage(recordHeaderHeading, parts.Standing))
	}
	pinned := []contract.Message{}
	if len(input.Pinned) > 0 {
		pinned = append(pinned, asUserMessage(pinnedHeading, pinnedText(input.Pinned, builder.boundary)))
	}
	whatIsKnown := []contract.Message{}
	if known != "" {
		whatIsKnown = append(whatIsKnown, asUserMessage(whatIsKnownHeading, known))
	}
	hint := []contract.Message{}
	if len(input.MemoryHint) > 0 {
		hint = append(hint, asUserMessage(memoryHintHeading, memoryHintText(input.MemoryHint)))
	}

	fixed := above + totalTokens(recordBody) + totalTokens(whatIsKnown) + totalTokens(recordResults) + totalTokens(recordHeader)
	room := input.ContextLength - builder.maxOutputTokens - fixed
	if room <= 0 {
		return nil, roomRanOut(fixed, input, builder.maxOutputTokens)
	}
	room -= totalTokens(pinned) + totalTokens(hint)
	if room < 0 {
		if len(input.Pinned) > 0 {
			return nil, pinsDoNotFit(input.Pinned, totalTokens(pinned))
		}
		return nil, roomRanOut(fixed+totalTokens(hint), input, builder.maxOutputTokens)
	}

	below := make([]contract.Message, 0,
		len(whatIsKnown)+len(pinned)+len(input.Messages)+len(recordBody)+len(recordResults)+len(hint)+len(recordHeader))
	below = append(below, whatIsKnown...)
	below = append(below, pinned...)
	below = append(below, fitNewestFirst(wrapToolResults(input.Messages, builder.boundary), room)...)
	below = append(below, recordBody...)
	below = append(below, recordResults...)
	below = append(below, hint...)
	return append(below, recordHeader...), nil
}

// fitNewestFirst keeps as many of the most recent messages as the window holds,
// in full. When the window is full the oldest message's whole text leaves; its
// one line in the record and its copy in the event log stay behind, so "read r7"
// brings it back. Nothing is ever summarized and nothing is ever cut short.
func fitNewestFirst(messages []contract.Message, room int) []contract.Message {
	kept := 0
	for at := len(messages) - 1; at >= 0; at-- {
		cost := estimateMessage(messages[at])
		if cost > room {
			break
		}
		room -= cost
		kept++
	}
	return withoutOrphanResults(messages[len(messages)-kept:])
}

// withoutOrphanResults drops any message answering a tool call that has left the
// window, because a result with no call behind it is refused on the wire.
func withoutOrphanResults(messages []contract.Message) []contract.Message {
	called := map[string]bool{}
	kept := make([]contract.Message, 0, len(messages))
	for _, message := range messages {
		if answersALostCall(message, called) {
			continue
		}
		for _, call := range message.ToolCalls {
			called[call.ID] = true
		}
		kept = append(kept, message)
	}
	return kept
}

// answersALostCall says whether a message carries a result for a call that is no
// longer in the window.
func answersALostCall(message contract.Message, called map[string]bool) bool {
	for _, result := range message.ToolResults {
		if !called[result.CallID] {
			return true
		}
	}
	return false
}

// wrapToolResults marks every tool result as data, on a copy, because the caller
// keeps the conversation between turns and a result marked twice would carry two
// boundaries.
func wrapToolResults(messages []contract.Message, boundary string) []contract.Message {
	marked := make([]contract.Message, 0, len(messages))
	for _, message := range messages {
		if len(message.ToolResults) == 0 {
			marked = append(marked, message)
			continue
		}
		copied := message
		copied.ToolResults = make([]contract.ToolResult, 0, len(message.ToolResults))
		for _, result := range message.ToolResults {
			result.Text = WrapAsData(boundary, result.Text)
			copied.ToolResults = append(copied.ToolResults, result)
		}
		marked = append(marked, copied)
	}
	return marked
}

// totalTokens is what a run of messages costs.
func totalTokens(messages []contract.Message) int {
	counted := 0
	for _, message := range messages {
		counted += estimateMessage(message)
	}
	return counted
}

// roomRanOut says how far past the model's window the parts that can never be
// dropped already are.
func roomRanOut(needed int, input BuildInput, outputCap int) error {
	return fmt.Errorf("%w: they are about %d tokens and the model holds %d, of which %d are kept for the reply",
		ErrNoRoomForTheRecord, needed, input.ContextLength, outputCap)
}

// pinsDoNotFit names the pin to let go of, which is the largest one, because
// dropping it frees the most room.
func pinsDoNotFit(pins []Pin, cost int) error {
	largest := pins[0]
	for _, pin := range pins {
		if len(pin.Text) > len(largest.Text) {
			largest = pin
		}
	}
	return fmt.Errorf("%w: the pins are about %d tokens, and the largest is %s at about %d",
		ErrPinnedEvidenceTooLarge, cost, largest.ID, EstimateTokens(largest.Text))
}
