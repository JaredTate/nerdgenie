// The idea of one named table of key bindings, rather than key handling spread
// through the drawing code, is ported from OpenCode's keybind definitions at
// ~/Code/opencode/packages/tui/src/config/keybind.ts. The bindings themselves are
// the ones docs/TUI_DESIGN.md lists, and the Go here is written fresh.

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// heldWithControl says whether one letter was pressed with the control key held
// and nothing else, which is how Ctrl+C and Ctrl+J are told apart from the
// letters themselves.
func heldWithControl(key tea.KeyPressMsg, letter rune) bool {
	return key.Code == letter && key.Mod == tea.ModCtrl
}

// pressed takes one key press and gives it to whatever holds the keys: the card
// that is waiting for an answer, the reason prompt, or the input box. Two keys
// belong to the screen as a whole before any of them: Ctrl+B, which puts the
// side panel away and brings it back, and the keys that scroll. The focus takes
// its own keys after the palette, which needs Tab to complete a command, and
// before a card, so that Tab still walks the pills while a card waits.
func (screen *Screen) pressed(key tea.KeyPressMsg) tea.Cmd {
	if heldWithControl(key, 'c') {
		return screen.pressedQuit()
	}
	screen.quitArmed = false
	if heldWithControl(key, 'b') {
		screen.panelHidden = !screen.panelHidden
		return nil
	}
	if screen.scrolledWithTheKey(key) {
		return nil
	}
	if screen.input.secret {
		if screen.pressedAtTheMaskedPrompt(key) {
			return nil
		}
		return screen.pressedInTheInputBox(key)
	}
	if screen.askingWhyNot {
		if screen.pressedWhileGivingAReason(key) {
			return nil
		}
		return screen.pressedInTheInputBox(key)
	}
	if screen.paletteOpen && screen.pressedWhileThePaletteIsOpen(key) {
		return nil
	}
	if screen.pressedOnTheFocus(key) {
		return nil
	}
	if screen.focusedCard() != nil {
		screen.pressedWhileACardWaits(key)
		return nil
	}
	return screen.pressedInTheInputBox(key)
}

// pressedWhileThePaletteIsOpen holds the three keys the palette takes: Tab and
// Enter complete the command that matches, and Escape closes the list and leaves
// what was typed alone.
func (screen *Screen) pressedWhileThePaletteIsOpen(key tea.KeyPressMsg) bool {
	switch key.Code {
	case tea.KeyTab, tea.KeyEnter:
		return screen.completeCommand()
	case tea.KeyEsc:
		screen.closePalette()
		return true
	}
	return false
}

// pressedWhileACardWaits holds the single-key answers a card takes. Every other
// key is swallowed, so that stray typing never lands in a box the person cannot
// use; the keys that scroll the transcript were taken before the card saw them.
func (screen *Screen) pressedWhileACardWaits(key tea.KeyPressMsg) {
	if key.Code == tea.KeyEsc {
		screen.withdrawFromCard()
		return
	}
	// A key that stands for no printable character at all, such as an arrow or
	// a control combination, is none of the three answers.
	switch key.Text {
	case "a":
		screen.answerCard(contract.AnswerOnce, "")
	case "A":
		screen.answerCard(contract.AnswerAlways, "")
	case "r":
		screen.askWhyNot()
	}
}

// pressedWhileGivingAReason holds the two keys that end the reason prompt, and
// lets every other key through to the input box so that the reason can be typed.
func (screen *Screen) pressedWhileGivingAReason(key tea.KeyPressMsg) bool {
	switch key.Code {
	case tea.KeyEnter:
		screen.answerCard(contract.AnswerReject, screen.input.text())
		return true
	case tea.KeyEsc:
		screen.askingWhyNot = false
		screen.input.clear()
		return true
	}
	return false
}

// pressedInTheInputBox is the ordinary case: a binding that edits what is being
// typed, or a character going into it.
func (screen *Screen) pressedInTheInputBox(key tea.KeyPressMsg) tea.Cmd {
	screen.editWithTheKey(key)
	screen.judgePalette()
	return nil
}

// editWithTheKey is the table of bindings the input box holds. Ctrl+J is asked
// about first, because a control combination carries the plain letter as its
// code and would otherwise be typed as one.
func (screen *Screen) editWithTheKey(key tea.KeyPressMsg) {
	if heldWithControl(key, 'j') {
		screen.input.insert("\n")
		return
	}
	switch key.Code {
	case tea.KeyEnter:
		screen.sendWhatWasTyped()
	case tea.KeyBackspace:
		screen.input.backspace()
	case tea.KeyLeft:
		screen.input.moveBy(-1)
	case tea.KeyRight:
		screen.input.moveBy(1)
	case tea.KeyHome:
		screen.input.cursor = 0
	case tea.KeyEnd:
		screen.input.moveBy(len(screen.input.letters))
	case tea.KeyUp:
		screen.recallBy(-1)
	case tea.KeyDown:
		screen.recallBy(1)
	case tea.KeyEsc:
		screen.pressedStop()
	default:
		// Everything left that stands for printable characters, the space bar
		// among them, is typed into the box.
		if key.Text != "" {
			screen.typeInto(key.Text)
		}
	}
}

// typeInto puts characters in the input box, and opens the command palette when
// the first of them is the slash that starts a command.
func (screen *Screen) typeInto(letters string) {
	opening := screen.input.cursor == 0 && strings.HasPrefix(letters, "/")
	screen.input.insert(letters)
	if opening {
		screen.openPalette()
	}
}

// pressedStop sends the stop command, which is what Esc does when there is
// nothing else for it to close. It fires whenever there is something to stop:
// the model thinking, a tool running, or a task working its way through, in any
// combination. A plain reply is not a task, and the person who wants a rambling
// answer to stop must not have to wait for it to finish.
func (screen *Screen) pressedStop() {
	if !screen.busy() && !screen.taskRunning() {
		return
	}
	screen.tell(contract.SocketEnvelope{Type: contract.SocketCommand, Text: "stop"})
}

// pressedQuit holds the rule that Ctrl+C twice quits, so that one stray press
// never throws away what the person was typing.
func (screen *Screen) pressedQuit() tea.Cmd {
	if screen.quitArmed {
		return tea.Quit
	}
	screen.quitArmed = true
	return nil
}

// sendWhatWasTyped sends the input box to the program: as a slash command when
// it starts with a slash, and as a message otherwise. The person's own words go
// into the transcript at once, because they are what the person just did, and
// the view comes back to the newest row to show them.
func (screen *Screen) sendWhatWasTyped() {
	text := screen.input.text()
	if strings.TrimSpace(text) == "" {
		return
	}
	screen.input.clear()
	screen.rememberTyped(text)
	screen.showTheNewest()
	screen.remember(block{kind: blockPerson, text: text})
	screen.answerTheQuestion()

	envelope := contract.SocketEnvelope{Type: contract.SocketMessage, Text: text}
	if command, isCommand := slashCommand(text); isCommand {
		envelope = contract.SocketEnvelope{Type: contract.SocketCommand, Text: command}
	}
	screen.tell(envelope)
}

// slashCommand reads a typed line as a slash command, and says false when it is
// an ordinary message.
func slashCommand(text string) (string, bool) {
	body, isCommand := strings.CutPrefix(strings.TrimSpace(text), "/")
	return body, isCommand && body != ""
}

// tell hands one envelope to the program and puts an error card in the
// transcript when it cannot be delivered, so that nothing is ever lost quietly.
func (screen *Screen) tell(envelope contract.SocketEnvelope) {
	if err := screen.link.Send(envelope); err != nil {
		screen.showTrouble(err.Error())
	}
}

// rememberTyped keeps what was sent so that Up walks back through it, and never
// keeps more than the cap.
func (screen *Screen) rememberTyped(text string) {
	screen.history = append(screen.history, text)
	if len(screen.history) > maxHistoryEntries {
		screen.history = screen.history[len(screen.history)-maxHistoryEntries:]
	}
	screen.historyAt = len(screen.history)
}

// recallBy walks up or down through what has been sent before and puts it in the
// input box, and walking past the newest empties the box again.
func (screen *Screen) recallBy(places int) {
	if len(screen.history) == 0 {
		return
	}
	screen.historyAt += places
	if screen.historyAt < 0 {
		screen.historyAt = 0
	}
	if screen.historyAt >= len(screen.history) {
		screen.historyAt = len(screen.history)
		screen.input.clear()
		return
	}
	screen.input.setText(screen.history[screen.historyAt])
}

// pasted puts text the terminal handed over all at once into the input box, the
// way typing it would have, with Windows and old Mac line ends made into plain
// ones so that a line break is always one character. A paste is activity, so
// it disarms a half-pressed quit like any key does.
func (screen *Screen) pasted(text string) {
	screen.quitArmed = false
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	screen.input.insert(text)
}
