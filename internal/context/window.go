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
// less everything above the cache line, less the record's live part. What is
// left is filled with the most recent results, in full, newest first.
//
// The order is the one the instruction text promises the model: the record's
// live half, the pinned evidence, the recent messages, and the memory hint.
func (builder *Builder) messagesFor(input BuildInput, live string, request contract.Request) ([]contract.Message, error) {
	above := EstimateRequestTokens(request)
	liveRecord := []contract.Message{}
	if live != "" {
		liveRecord = append(liveRecord, asUserMessage(recordSecondHalfHeading, live))
	}
	pinned := []contract.Message{}
	if len(input.Pinned) > 0 {
		pinned = append(pinned, asUserMessage(pinnedHeading, pinnedText(input.Pinned, builder.boundary)))
	}
	hint := []contract.Message{}
	if len(input.MemoryHint) > 0 {
		hint = append(hint, asUserMessage(memoryHintHeading, memoryHintText(input.MemoryHint)))
	}

	fixed := above + totalTokens(liveRecord)
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

	below := make([]contract.Message, 0, len(liveRecord)+len(pinned)+len(input.Messages)+len(hint))
	below = append(below, liveRecord...)
	below = append(below, pinned...)
	below = append(below, fitNewestFirst(wrapToolResults(input.Messages, builder.boundary), room)...)
	return append(below, hint...), nil
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
