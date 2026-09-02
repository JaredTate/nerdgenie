package tui

import "github.com/JaredTate/coeus/internal/contract"

// receive takes one message the running program sent and changes the screen to
// match it. A kind this screen does not understand is ignored rather than
// refused, so that an older screen and a newer program can still work together.
func (screen *Screen) receive(envelope contract.SocketEnvelope) {
	switch envelope.Type {
	case contract.SocketDelta:
		screen.pending += envelope.Text
	case contract.SocketReply:
		screen.finishReply(envelope.Text)
	case contract.SocketPreview:
		screen.showCard(cardFrom(envelope, cardPreview, previewTitle))
	case contract.SocketAsk:
		if envelope.Fields[maskedField] == "true" {
			screen.askForSecret(envelope)
			return
		}
		screen.showCard(cardFrom(envelope, cardQuestion, questionTitle))
	case contract.SocketHandoff:
		screen.showCard(cardFrom(envelope, cardHandoff, handoffTitle))
	case contract.SocketStatus:
		screen.readStatus(envelope)
	case contract.SocketError:
		screen.flushDeltas()
		screen.showTrouble(troubleWords(envelope))
	}
}

// cardFrom turns one message from the program into a card. The title in the
// rule is always the one docs/TUI_DESIGN.md gives that kind of card, so that the
// frame reads the same every time; a title the program sent that says something
// else becomes the first line inside the box.
func cardFrom(envelope contract.SocketEnvelope, kind cardKind, title string) card {
	body := envelope.Text
	if envelope.Title != "" && envelope.Title != title {
		body = envelope.Title + "\n" + body
	}
	picture := ""
	if len(envelope.Attachments) > 0 {
		picture = envelope.Attachments[0]
	}
	return card{kind: kind, id: envelope.ID, title: title, body: body, picture: picture}
}

// troubleWords is what an error message says, which is its text, its reason, or
// both when the program sent both.
func troubleWords(envelope contract.SocketEnvelope) string {
	switch {
	case envelope.Text != "" && envelope.Reason != "":
		return envelope.Text + " " + envelope.Reason
	case envelope.Reason != "":
		return envelope.Reason
	case envelope.Text != "":
		return envelope.Text
	default:
		return "the program reported a problem and did not say what it was"
	}
}

// flushDeltas moves the text that has arrived since the last heartbeat into the
// reply block on the screen, making one when there is none open. This is what
// coalescing means: the deltas pile up as they arrive and the frame changes once
// every thirty milliseconds rather than once per word.
func (screen *Screen) flushDeltas() {
	if screen.pending == "" {
		return
	}
	if open := screen.openReply(); open != nil {
		open.text = keepTail(open.text + screen.pending)
	} else {
		screen.remember(block{kind: blockReply, text: screen.pending})
		screen.streaming = true
	}
	screen.pending = ""
}

// finishReply closes the reply that was being streamed. The program's finished
// text is the authority when it sent any, because the deltas were only its
// working out.
func (screen *Screen) finishReply(text string) {
	screen.flushDeltas()
	open := screen.openReply()
	switch {
	case open == nil && text != "":
		screen.remember(block{kind: blockReply, text: text})
	case open != nil && text != "":
		open.text = keepTail(text)
	}
	screen.streaming = false
}

// openReply is the reply block still being streamed into, or nothing when the
// last reply is finished.
func (screen *Screen) openReply() *block {
	if !screen.streaming || len(screen.blocks) == 0 {
		return nil
	}
	last := &screen.blocks[len(screen.blocks)-1]
	if last.kind != blockReply {
		return nil
	}
	return last
}

// readStatus takes what the program says about itself and puts it in the header,
// the status strip, and the command palette.
func (screen *Screen) readStatus(envelope contract.SocketEnvelope) {
	if listed, sent := envelope.Fields[commandsField]; sent {
		screen.learnCommands(listed)
	}
}
