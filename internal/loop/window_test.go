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

// TestTheOldestHalfOfTheMessagesLeavesAtOnceWhenTheCapIsReached is the fix for
// the live game build's cliff. The loop used to drop one message every call once
// it held MaxMessagesKept, so from round a hundred on the front of the
// conversation moved on every call, the daemon could reuse nothing past the
// system prompt, and a round that had cost twenty seconds cost a hundred and
// seventy. Dropping the oldest half at once leaves the front alone for the next
// fifty rounds, and what the model reads is the same either way: the newest
// messages, with the record and the log behind them.
func TestTheOldestHalfOfTheMessagesLeavesAtOnceWhenTheCapIsReached(t *testing.T) {
	running := &run{}
	for round := 1; round <= MaxMessagesKept/2; round++ {
		for _, message := range aRoundOfMessages(round) {
			running.remember(message)
		}
	}
	if len(running.messages) != MaxMessagesKept {
		t.Fatalf("after %d rounds the loop holds %d messages, want the cap of %d", MaxMessagesKept/2, len(running.messages), MaxMessagesKept)
	}

	oneMore := aRoundOfMessages(MaxMessagesKept/2 + 1)
	running.remember(oneMore[0])

	kept := len(running.messages)
	if kept > MaxMessagesKept/2 {
		t.Errorf("one message past the cap leaves %d messages, want the oldest half gone at once, so at most %d", kept, MaxMessagesKept/2)
	}
	if kept < MaxMessagesKept/2-1 {
		t.Errorf("one message past the cap leaves %d messages, and only the oldest half and an orphaned result may go", kept)
	}
	if first := running.messages[0]; first.Role != contract.RoleAssistant || len(first.ToolCalls) == 0 {
		t.Errorf("the window now begins with %+v, want the model's own call, because a result whose call has left is refused on the wire", first)
	}
	if last := running.messages[kept-1]; last.Text != oneMore[0].Text {
		t.Errorf("the newest message reads %q, want the one just remembered", last.Text)
	}

	running.remember(oneMore[1])
	if len(running.messages) != kept+1 {
		t.Errorf("the next message after the drop leaves %d messages, want %d: nothing else leaves until the cap is reached again",
			len(running.messages), kept+1)
	}
}
