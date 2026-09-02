// The idea of one named table of key bindings, rather than key handling spread
// through the drawing code, is ported from OpenCode's keybind definitions at
// ~/Code/opencode/packages/tui/src/config/keybind.ts. The bindings themselves are
// the ones docs/TUI_DESIGN.md lists, and the Go here is written fresh.

package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JaredTate/coeus/internal/contract"
)

// scrollRows is how far one page-up or page-down moves the transcript.
const scrollRows = 10

// pressed takes one key press and does what docs/TUI_DESIGN.md says it does.
// Anything that is not a binding is a character going into the input box.
func (screen *Screen) pressed(key tea.KeyMsg) tea.Cmd {
	if key.Type != tea.KeyCtrlC {
		screen.quitArmed = false
	}
	switch key.Type {
	case tea.KeyCtrlC:
		return screen.pressedQuit()
	case tea.KeyEnter:
		screen.sendWhatWasTyped()
	case tea.KeyCtrlJ:
		screen.input.insert("\n")
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
	case tea.KeyPgUp:
		screen.scrollBy(scrollRows)
	case tea.KeyPgDown:
		screen.scrollBy(-scrollRows)
	case tea.KeySpace:
		screen.input.insert(" ")
	case tea.KeyRunes:
		screen.input.insert(string(key.Runes))
	}
	return nil
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
// into the transcript at once, because they are what the person just did.
func (screen *Screen) sendWhatWasTyped() {
	text := screen.input.text()
	if strings.TrimSpace(text) == "" {
		return
	}
	screen.input.clear()
	screen.rememberTyped(text)
	screen.remember(block{kind: blockPerson, text: text})

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
