package tui

// programState is what the program said it was doing last, which is the one
// thing the status strip is allowed to say.
type programState int

const (
	// stateConnecting is the state the first frame is drawn in, before the
	// screen has reached the program at all.
	stateConnecting programState = iota
	// stateIdle means the program is running and has nothing to do.
	stateIdle
	// stateThinking means a model call is in flight.
	stateThinking
	// stateUsingTool means a tool is running, and the detail names it.
	stateUsingTool
	// stateWaitingForYou means a card is on the screen and the work has stopped.
	stateWaitingForYou
	// statePaused means the scheduled work is switched off.
	statePaused
	// stateDisconnected means the link dropped and the screen is dialling again.
	stateDisconnected
)

// stateWords is the state in the plain words docs/TUI_DESIGN.md uses.
func (screen *Screen) stateWords() string {
	switch screen.state {
	case stateIdle:
		return "idle"
	case stateThinking:
		return "thinking"
	case stateUsingTool:
		return "using " + screen.detail
	case stateWaitingForYou:
		return "waiting for you"
	case statePaused:
		return "paused"
	case stateDisconnected:
		return "disconnected, reconnecting"
	default:
		return "connecting"
	}
}

// stateStyle draws a broken link in the error colour and everything else dim.
func (screen *Screen) stateStyle() style {
	if screen.state == stateDisconnected {
		return styleError
	}
	return styleDim
}

// busy says whether the program is doing something the person is waiting on,
// which is the only time a spinner is due.
func (screen *Screen) busy() bool {
	return screen.state == stateThinking || screen.state == stateUsingTool
}

// statusRow draws the one row at the bottom: the spinner when one is due, the
// state in plain words, the budget while a task runs, and the key hints on the
// right.
func (screen *Screen) statusRow() string {
	line := row{}
	line.blanks(marginColumns)
	if screen.spinnerShowing() {
		line.add(styleAccent, screen.spinnerFrame()+" ")
	}
	line.add(screen.stateStyle(), screen.stateWords())
	if screen.budget != "" {
		line.add(styleDim, " · "+screen.budget)
	}
	if screen.scrolledUp() {
		line.add(styleDim, " · "+string(moreGlyph)+" more")
	}
	hints := screen.keyHints()
	if hints != "" && screen.width >= narrowWidth {
		line.padTo(screen.width - marginColumns - displayWidth(hints))
		line.add(styleDim, hints)
	}
	return line.render(screen.colors)
}

// keyHints are the three key hints on the right of the status strip, which
// change with what the screen is waiting for.
func (screen *Screen) keyHints() string {
	switch {
	case screen.input.secret:
		return "Enter send · Esc cancel · nothing is shown"
	case screen.paletteOpen:
		return "Tab complete · Enter run · Esc close"
	case screen.askingWhyNot:
		return "Enter send the reason · Esc go back"
	case screen.focusedCard() != nil:
		return screen.focusedCard().keyHints()
	case screen.busy():
		return "Enter send · Ctrl+J newline · Esc stop"
	default:
		return "Enter send · Ctrl+J newline · Ctrl+C quit"
	}
}

// keyHints for a card are the single keys that answer it.
func (shown card) keyHints() string {
	switch shown.kind {
	case cardPreview:
		return "a approve · A always · r reject"
	case cardHandoff:
		return "a finished · r give up"
	default:
		return "Enter answer · Esc dismiss"
	}
}
