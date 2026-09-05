package loop

import "github.com/JaredTate/nerdgenie/internal/contract"

// remember appends one message to the conversation the model sees, keeping only
// the most recent ones, because every buffer here has a cap. When the cap is
// reached the oldest half leaves at once, so that the front of the prompt then
// holds still for another fifty rounds and a prompt cache can reuse it; the
// cut then moves forward to the model's own call, because a result whose call
// has left is refused on the wire.
func (running *run) remember(message contract.Message) {
	if message.Text == "" && len(message.ToolCalls) == 0 && len(message.ToolResults) == 0 {
		return
	}
	running.messages = append(running.messages, message)
	if len(running.messages) <= MaxMessagesKept {
		return
	}
	cut := len(running.messages) - MaxMessagesKept/2
	for cut < len(running.messages)-1 && running.messages[cut].Role != contract.RoleAssistant {
		cut++
	}
	running.messages = running.messages[cut:]
}
