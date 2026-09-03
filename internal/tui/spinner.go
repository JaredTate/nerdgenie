// The rule that a spinner waits half a second before it appears and then stays
// for three seconds, so that a quick answer never makes it flash, is ported from
// OpenCode's startup loading component at
// ~/Code/opencode/packages/tui/src/component/startup-loading.tsx. The Go here is
// written fresh and reads the time from contract.Clock rather than the machine.

package tui

import "time"

// The spinner's timing, all of it from docs/TUI_DESIGN.md.
const (
	// spinnerDelay is how long the screen waits before showing a spinner at all,
	// so that a quick answer never makes one flash.
	spinnerDelay = 500 * time.Millisecond
	// spinnerHold is the shortest time a spinner stays once it has appeared.
	spinnerHold = 3 * time.Second
	// spinnerRate is how long one frame of the spinner is drawn for, which is
	// eight frames a second.
	spinnerRate = 125 * time.Millisecond
)

// spinnerFrames is the four-frame dot cycle, drawn only in the status strip and
// never in the transcript.
var spinnerFrames = []string{"·  ", " · ", "  ·", " · "}

// setState records what the program said it was doing and starts or stops the
// clock the spinner is timed against.
func (screen *Screen) setState(doing programState, detail string) {
	screen.state = doing
	screen.detail = detail
	if screen.busy() {
		if screen.busySince.IsZero() {
			screen.busySince = screen.now
		}
	} else {
		screen.busySince = time.Time{}
	}
	screen.judgeSpinner()
}

// judgeSpinner decides whether a spinner is on the screen. It appears once the
// wait has lasted longer than the delay, and once it has appeared it stays for at
// least the hold, however quickly the answer then arrives.
func (screen *Screen) judgeSpinner() {
	if screen.busy() {
		if screen.spinnerSince.IsZero() && screen.now.Sub(screen.busySince) >= spinnerDelay {
			screen.spinnerSince = screen.now
		}
		return
	}
	if !screen.spinnerSince.IsZero() && screen.now.Sub(screen.spinnerSince) >= spinnerHold {
		screen.spinnerSince = time.Time{}
	}
}

// spinnerShowing says whether a spinner is on the screen right now.
func (screen *Screen) spinnerShowing() bool {
	return !screen.spinnerSince.IsZero()
}

// spinnerFrame is the frame of the dot cycle the spinner is on.
func (screen *Screen) spinnerFrame() string {
	elapsed := screen.now.Sub(screen.spinnerSince)
	if elapsed < 0 {
		elapsed = 0
	}
	return spinnerFrames[int(elapsed/spinnerRate)%len(spinnerFrames)]
}
