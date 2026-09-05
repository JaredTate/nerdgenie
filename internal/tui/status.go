package tui

import (
	"strconv"
	"strings"
	"time"
)

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

// stateStyle draws a broken link loud, in bold white, and everything else dim.
func (screen *Screen) stateStyle() style {
	if screen.state == stateDisconnected {
		return styleBold
	}
	return styleDim
}

// busy says whether the program is doing something the person is waiting on,
// which is the only time a spinner is due.
func (screen *Screen) busy() bool {
	return screen.state == stateThinking || screen.state == stateUsingTool
}

// statusRow draws the status strip, the last row of the frame, under the
// footer's key hints: the spinner when one is due, the state in plain words,
// the budget while a task runs, and the older mark while the person is
// scrolled up, so that they know why new text is not appearing.
func (screen *Screen) statusRow() string {
	line := row{}
	line.blanks(marginColumns)
	if screen.spinnerShowing() {
		line.add(styleAccent, screen.spinnerFrame()+" ")
	}
	line.add(screen.stateStyle(), screen.stateWords())
	if watched := screen.callWords(); watched != "" {
		line.add(styleDim, " · "+watched)
	}
	if screen.budget != "" {
		line.add(styleDim, " · "+screen.budget)
		filled, empty := screen.budgetBar()
		line.add(styleMeterOn, filled)
		line.add(styleMeterOff, empty)
	}
	if screen.scrolledUp() {
		line.add(styleDim, " · "+string(olderGlyph)+" older")
	}
	line.keepWithin(screen.width - marginColumns)
	return line.render(screen.colors)
}

// callWords is what the strip says about the model call in progress: how long it
// has been running and how many tokens it has written, such as "14 s · 212
// tokens". It is empty whenever no call is running or the program did not say
// when this one began, because a count from a moment the screen does not know is
// worse than no count at all.
//
// The count is drawn from the moment the program reports the call, not from the
// moment the spinner is due, so it never blinks in and out with the spinner's
// own delay and hold.
func (screen *Screen) callWords() string {
	if screen.state != stateThinking || screen.callStarted.IsZero() {
		return ""
	}
	running := screen.now.Sub(screen.callStarted)
	if running < 0 {
		running = 0
	}
	return strconv.Itoa(int(running/time.Second)) + " s · " + strconv.Itoa(screen.streamed) + " tokens"
}

// progressCells is how many cells the budget bar is drawn out of. Four is short
// enough to leave room for the key hints at the design's eighty columns and long
// enough to read at a glance.
const progressCells = 4

// budgetBar is the thin bar in the status strip: the filled cells and the empty
// ones, or two empty strings when no task is running.
//
// The program reports its budget in plain words, such as "86 rounds, 51 min
// left", so the first number in that line is the count and the largest count
// seen since this task started is a full bar. There is no fuller measure to be
// had, and a bar measured against the fullest report of this task is the truth
// as the screen knows it.
func (screen *Screen) budgetBar() (string, string) {
	if !screen.taskRunning() || screen.budgetMost <= 0 || screen.budgetNow <= 0 {
		return "", ""
	}
	filled := min((screen.budgetNow*progressCells+screen.budgetMost-1)/screen.budgetMost, progressCells)
	return " " + strings.Repeat(string(barFullGlyph), filled), strings.Repeat(string(barEmptyGlyph), progressCells-filled)
}

// taskRunning says whether a task is running right now, which is the only time
// the budget bar and the accent-coloured task words are drawn.
func (screen *Screen) taskRunning() bool {
	return screen.taskState == "running"
}
