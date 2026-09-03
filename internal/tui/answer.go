package tui

import "github.com/JaredTate/coeus/internal/contract"

// showCard puts a card in the transcript, gives it the keys, and says in the
// status strip that the work has stopped and is waiting for the person. The
// screenshot on a handoff is read and encoded here, once, rather than on every
// frame the card is drawn in.
func (screen *Screen) showCard(shown card) {
	screen.flushDeltas()
	screen.cardsShown++
	shown.number = screen.cardsShown
	if shown.picture != "" {
		shown.drawn, _ = screen.pictureLine(shown.picture, screen.transcriptWidth())
	}
	if shown.takesKeys() {
		screen.waitingFor = shown.number
	}
	screen.remember(block{kind: blockCard, shown: shown})
	screen.setState(stateWaitingForYou, "")
}

// withdrawFromCard tells the program that the person walked away from the card
// on the screen, so that nothing is left waiting for an answer that will never
// come. internal/contract calls this a cancel rather than a no, because the
// person refused nothing; they closed a box.
func (screen *Screen) withdrawFromCard() {
	shown := screen.focusedCard()
	if shown == nil {
		return
	}
	shown.answered = true
	identifier := shown.id
	screen.waitingFor = 0
	screen.askingWhyNot = false
	screen.input.clear()
	screen.setState(stateIdle, "")
	screen.remember(block{kind: blockTool, text: "walked away without answering"})
	screen.tell(contract.SocketEnvelope{Type: contract.SocketCancel, ID: identifier})
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
	screen.waitingFor = 0
	screen.askingWhyNot = false
	screen.input.clear()
	screen.setState(stateIdle, "")

	envelope := approveEnvelope(shown.id, answer)
	if answer == contract.AnswerReject {
		envelope = contract.SocketEnvelope{Type: contract.SocketDeny, ID: shown.id, Reason: reason}
	}
	screen.remember(block{kind: blockTool, text: answerWords(answer, reason)})
	screen.tell(envelope)
}

// approveEnvelope is the yes the screen sends. internal/contract names the text
// for one of the two yeses: an approve carrying ApproveAlwaysText means every
// call like this one for the rest of the session, and an approve carrying
// nothing means this one call.
func approveEnvelope(id string, answer contract.PreviewAnswer) contract.SocketEnvelope {
	envelope := contract.SocketEnvelope{Type: contract.SocketApprove, ID: id}
	if answer == contract.AnswerAlways {
		envelope.Text = contract.ApproveAlwaysText
	}
	return envelope
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
		if screen.blocks[at].kind != blockCard || screen.blocks[at].shown.kind != cardQuestion {
			continue
		}
		if !screen.blocks[at].shown.answered {
			screen.blocks[at].shown.answered = true
			screen.setState(stateIdle, "")
		}
		return
	}
}
