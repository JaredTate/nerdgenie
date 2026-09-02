package tui

import "github.com/JaredTate/coeus/internal/contract"

// showCard puts a card in the transcript, gives it the keys, and says in the
// status strip that the work has stopped and is waiting for the person.
func (screen *Screen) showCard(shown card) {
	screen.flushDeltas()
	screen.remember(block{kind: blockCard, shown: shown})
	screen.setState(stateWaitingForYou, "")
}

// askWhyNot moves the keys to the input box so that the person can say why they
// are turning a preview down.
func (screen *Screen) askWhyNot() {
	screen.askingWhyNot = true
	screen.input.clear()
}

// answerCard sends the person's answer to the card that holds the keys and
// writes what they said into the transcript in one dim line.
func (screen *Screen) answerCard(answer contract.PreviewAnswer, reason string) {
	shown := screen.focusedCard()
	if shown == nil {
		return
	}
	shown.answered = true
	screen.askingWhyNot = false
	screen.input.clear()
	screen.setState(stateIdle, "")

	envelope := contract.SocketEnvelope{Type: contract.SocketApprove, ID: shown.id, Text: string(answer)}
	if answer == contract.AnswerReject {
		envelope = contract.SocketEnvelope{Type: contract.SocketDeny, ID: shown.id, Reason: reason}
	}
	screen.remember(block{kind: blockTool, text: answerWords(answer, reason)})
	screen.tell(envelope)
}

// answerWords is the one dim line the transcript keeps for an answer, so that
// the person can read back what they agreed to.
func answerWords(answer contract.PreviewAnswer, reason string) string {
	switch answer {
	case contract.AnswerOnce:
		return "approved once"
	case contract.AnswerAlways:
		return "approved for this session"
	case contract.AnswerReject:
		if reason == "" {
			return "rejected"
		}
		return "rejected: " + reason
	default:
		return "answered"
	}
}

// answerTheQuestion marks the newest question card as answered when the person
// types a reply to it, so that the status strip stops saying that the screen is
// waiting for them.
func (screen *Screen) answerTheQuestion() {
	for at := len(screen.blocks) - 1; at >= 0; at-- {
		if screen.blocks[at].kind != blockCard {
			continue
		}
		if screen.blocks[at].shown.kind == cardQuestion && !screen.blocks[at].shown.answered {
			screen.blocks[at].shown.answered = true
			screen.setState(stateIdle, "")
		}
		return
	}
}
