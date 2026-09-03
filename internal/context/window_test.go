package context

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestABigModelKeepsEveryResultOnTheTable proves the window is never smaller
// than the task needs: a model with room for the whole conversation gets it.
func TestABigModelKeepsEveryResultOnTheTable(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.ContextLength = 200000
	input.Messages = roundsOfConversation(20, 400)

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	if kept := resultsIn(request); kept != 20 {
		t.Errorf("a 200k model kept %d of the 20 results, and it has room for them all", kept)
	}
}

// TestASmallModelPutsTheOldestResultsBackOnTheShelf proves the other half of the
// one rule: the window is never bigger than the model can hold. The oldest
// results leave in full, and the newest are still there in full, because nothing
// is ever summarized.
func TestASmallModelPutsTheOldestResultsBackOnTheShelf(t *testing.T) {
	builder := newTestBuilder(t, Options{MaxOutputTokens: 1000})
	input := sampleInput()
	input.ContextLength = 6000
	input.Messages = roundsOfConversation(20, 400)

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	kept := resultsIn(request)
	if kept == 0 || kept >= 20 {
		t.Fatalf("a 6k model kept %d of the 20 results, want some but not all", kept)
	}
	whole := allMessageText(request)
	if !strings.Contains(whole, "the result of round 20") {
		t.Error("the newest result left the window, and the window fills newest first")
	}
	if strings.Contains(whole, "the result of round 1,") {
		t.Error("the oldest result is still in the window, and it should have gone back on the shelf")
	}
	if EstimateRequestTokens(request)+1000 > input.ContextLength {
		t.Errorf("the built prompt is %d tokens and the model holds %d less the output cap",
			EstimateRequestTokens(request), input.ContextLength)
	}
}

// TestAResultNeverOutlivesTheCallItAnswers proves a tool result whose call left
// the window leaves with it, because a result with no call behind it is refused
// on the wire.
func TestAResultNeverOutlivesTheCallItAnswers(t *testing.T) {
	builder := newTestBuilder(t, Options{MaxOutputTokens: 1000})
	input := sampleInput()
	input.ContextLength = 6000
	input.Messages = roundsOfConversation(20, 400)

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	called := map[string]bool{}
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			if !called[result.CallID] {
				t.Errorf("the result of %s is in the window and the call it answers is not", result.CallID)
			}
		}
		for _, call := range message.ToolCalls {
			called[call.ID] = true
		}
	}
}

// TestPinnedEvidenceNeverLeavesTheWindow proves a pin outlives every result
// around it, however small the window is.
func TestPinnedEvidenceNeverLeavesTheWindow(t *testing.T) {
	builder := newTestBuilder(t, Options{MaxOutputTokens: 1000})
	input := sampleInput()
	input.ContextLength = 6000
	input.Messages = roundsOfConversation(20, 400)
	input.Pinned = []Pin{{ID: "r1", Text: "the anniversary is the seventeenth of January"}}

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	if !strings.Contains(allMessageText(request), "the anniversary is the seventeenth of January") {
		t.Error("the pinned evidence left the window, and a pin never leaves")
	}
}

// TestPinsTooBigForTheWindowNameThePinToDrop proves the build refuses rather
// than quietly dropping something the user pinned, and that the refusal says
// which pin to let go of.
func TestPinsTooBigForTheWindowNameThePinToDrop(t *testing.T) {
	builder := newTestBuilder(t, Options{MaxOutputTokens: 1000})
	input := sampleInput()
	input.ContextLength = 6000
	input.Pinned = []Pin{
		{ID: "r1", Text: "a short pin"},
		{ID: "r2", Text: strings.Repeat("a very long page of evidence. ", 2000)},
	}

	_, err := builder.Build(t.Context(), input)
	if !errors.Is(err, ErrPinnedEvidenceTooLarge) {
		t.Fatalf("pins that do not fit gave back %v, want the pinned-evidence error", err)
	}
	if !strings.Contains(err.Error(), "r2") {
		t.Errorf("the refusal does not name the pin to drop: %v", err)
	}
}

// TestAWindowWithNoRoomLeftIsRefused proves a model too small to hold the rules,
// the persona, the tools and the record is told so, rather than being sent a
// prompt it will refuse.
func TestAWindowWithNoRoomLeftIsRefused(t *testing.T) {
	builder := newTestBuilder(t, Options{MaxOutputTokens: 1000})
	input := sampleInput()
	input.ContextLength = 1200

	_, err := builder.Build(t.Context(), input)
	if !errors.Is(err, ErrNoRoomForTheRecord) {
		t.Fatalf("a model with no room gave back %v, want the no-room error", err)
	}
}

// TestTheMemoryHintIsCappedAtThreeLines proves the hint stays the three lines
// the design promises, whatever the caller passes.
func TestTheMemoryHintIsCappedAtThreeLines(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.MemoryHint = []string{"one", "two", "three", "four", "five"}

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	whole := allMessageText(request)
	if !strings.Contains(whole, "three") {
		t.Errorf("the third line of the memory hint is missing:\n%s", whole)
	}
	if strings.Contains(whole, "four") {
		t.Errorf("the memory hint runs past %d lines:\n%s", contract.MemoryHintLines, whole)
	}
}

// roundsOfConversation makes a conversation of the given number of rounds, each
// one an assistant message asking for a tool and a user message carrying the
// result, with a result of about the given number of characters.
func roundsOfConversation(rounds int, resultSize int) []contract.Message {
	messages := []contract.Message{}
	for number := 1; number <= rounds; number++ {
		callID := fmt.Sprintf("call_%d", number)
		text := fmt.Sprintf("the result of round %d, ", number) + strings.Repeat("page text. ", resultSize/11)
		messages = append(messages,
			contract.Message{
				Role:      contract.RoleAssistant,
				Text:      fmt.Sprintf("Round %d.", number),
				ToolCalls: []contract.ToolCall{{ID: callID, Name: "read", Input: json.RawMessage(`{"path":"a"}`)}},
			},
			contract.Message{
				Role:        contract.RoleUser,
				ToolResults: []contract.ToolResult{{CallID: callID, Text: text}},
			},
		)
	}
	return messages
}

// resultsIn counts the tool results the working context holds.
func resultsIn(request contract.Request) int {
	counted := 0
	for _, message := range request.Messages {
		counted += len(message.ToolResults)
	}
	return counted
}

// allMessageText is everything below the cache line, joined.
func allMessageText(request contract.Request) string {
	parts := []string{}
	for _, message := range request.Messages {
		parts = append(parts, message.Text)
		for _, result := range message.ToolResults {
			parts = append(parts, result.Text)
		}
	}
	return strings.Join(parts, "\n")
}
