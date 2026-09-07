package loop

import (
	"strconv"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aRoundOfMessages is one round as the loop remembers it: the model's reply
// asking for a call, and the result that answers it.
func aRoundOfMessages(number int) []contract.Message {
	id := "c" + strconv.Itoa(number)
	return []contract.Message{
		{Role: contract.RoleAssistant, Text: "round " + strconv.Itoa(number), ToolCalls: []contract.ToolCall{{ID: id, Name: "read"}}},
		{Role: contract.RoleUser, ToolResults: []contract.ToolResult{{CallID: id, Text: "the notes"}}},
	}
}

// TestRememberKeepsEveryMessagePastTheCapAndSaysTheWindowIsFull: remember
// used to drop the oldest half of the messages the moment MaxMessagesKept was
// passed, which kept the front of the prompt still for fifty rounds but left
// the model reading a conversation cut in the middle, and a hundred messages
// of it re-read from cold. Now remember only keeps and counts; the round that
// sees the window full opens a fresh one, oriented, with the ask last, which
// freshwindow_test.go proves.
func TestRememberKeepsEveryMessagePastTheCapAndSaysTheWindowIsFull(t *testing.T) {
	running := &run{}
	for round := 1; round <= MaxMessagesKept/2; round++ {
		for _, message := range aRoundOfMessages(round) {
			running.remember(message)
		}
	}
	if len(running.messages) != MaxMessagesKept {
		t.Fatalf("after %d rounds the loop holds %d messages, want the cap of %d", MaxMessagesKept/2, len(running.messages), MaxMessagesKept)
	}
	if running.windowIsFull() {
		t.Error("the window says it is full at the cap, and it is full only past it")
	}

	for _, message := range aRoundOfMessages(MaxMessagesKept/2 + 1) {
		running.remember(message)
	}

	if len(running.messages) != MaxMessagesKept+2 {
		t.Errorf("one round past the cap leaves %d messages, want %d: nothing leaves in remember, the round opens a fresh window",
			len(running.messages), MaxMessagesKept+2)
	}
	if !running.windowIsFull() {
		t.Error("the window does not say it is full past the cap")
	}
	if first := running.messages[0]; first.Text != "round 1" {
		t.Errorf("the window now begins with %+v, want the first round still there until the fresh window opens", first)
	}
}
