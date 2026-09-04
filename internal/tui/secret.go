package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// askForSecret puts the input box into secret mode, which is what an ask with
// MaskInput set asks for. From here until the person answers or cancels, every
// character typed is drawn as a bullet, and none of it reaches the transcript,
// the history, or anything else that is written down.
func (screen *Screen) askForSecret(envelope contract.SocketEnvelope) {
	screen.flushDeltas()
	screen.input.clear()
	screen.input.secret = true
	screen.secretID = envelope.ID
	screen.secretPrompt = secretWords(envelope)
	screen.askingWhyNot = false
	screen.setState(stateWaitingForYou, "")
}

// secretWords is the line above the box saying what is being asked for.
func secretWords(envelope contract.SocketEnvelope) string {
	switch {
	case envelope.Title != "":
		return envelope.Title
	case envelope.Text != "":
		return envelope.Text
	default:
		return "a secret"
	}
}

// pressedAtTheMaskedPrompt holds the two keys that end secret mode. Every other
// key goes to the input box, where it is drawn as a bullet.
func (screen *Screen) pressedAtTheMaskedPrompt(key tea.KeyPressMsg) bool {
	switch key.Code {
	case tea.KeyEnter:
		screen.sendSecret()
		return true
	case tea.KeyEsc:
		screen.cancelSecret()
		return true
	}
	return false
}

// sendSecret hands what was typed to the program in the field made for it, and
// then forgets it. The screen never keeps a copy.
func (screen *Screen) sendSecret() {
	envelope := contract.SocketEnvelope{
		Type:   contract.SocketSecret,
		ID:     screen.secretID,
		Secret: screen.input.text(),
	}
	screen.leaveSecretMode()
	screen.tell(envelope)
}

// cancelSecret withdraws from the masked prompt, so that the program is not left
// waiting out its whole deadline for an answer that will never come. It is a
// cancel rather than a no because the person refused nothing; they closed a box.
func (screen *Screen) cancelSecret() {
	envelope := contract.SocketEnvelope{
		Type:   contract.SocketCancel,
		ID:     screen.secretID,
		Reason: "the person closed the prompt without entering the secret",
	}
	screen.leaveSecretMode()
	screen.tell(envelope)
}

// leaveSecretMode empties the box and puts it back to ordinary typing.
func (screen *Screen) leaveSecretMode() {
	screen.input.clear()
	screen.input.secret = false
	screen.secretID = ""
	screen.secretPrompt = ""
	screen.setState(stateIdle, "")
}
