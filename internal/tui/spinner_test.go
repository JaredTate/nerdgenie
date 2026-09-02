package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
)

// advance moves the fake clock on and gives the screen the heartbeat that
// follows it, which is the only way time reaches the screen.
func advance(screen *Screen, clock *testkit.FakeClock, duration time.Duration) {
	clock.Advance(duration)
	screen.Update(tickMessage{at: clock.Now()})
}

// statusStrip is the last row of the frame, which is the only row a spinner is
// ever allowed to appear on.
func statusStrip(screen *Screen) string {
	rows := strings.Split(screen.View(), "\n")
	return rows[len(rows)-1]
}

func TestTheSpinnerWaitsHalfASecondBeforeItAppears(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.setState(stateThinking, "")

	advance(screen, clock, 400*time.Millisecond)
	if screen.spinnerShowing() {
		t.Error("the spinner appeared after four hundred milliseconds, and it must wait five hundred")
	}

	advance(screen, clock, 100*time.Millisecond)
	if !screen.spinnerShowing() {
		t.Error("the spinner did not appear after five hundred milliseconds, and by then the wait is worth showing")
	}
	if !strings.Contains(statusStrip(screen), "·") {
		t.Errorf("the status strip is %q, and the spinner belongs on it", statusStrip(screen))
	}
}

func TestTheSpinnerStaysThreeSecondsEvenWhenTheReplyArrivesAtOnce(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.setState(stateThinking, "")

	advance(screen, clock, 500*time.Millisecond)
	advance(screen, clock, 500*time.Millisecond)
	screen.setState(stateIdle, "")
	if !screen.spinnerShowing() {
		t.Error("the spinner went the moment the reply arrived at one second, and it must stay three seconds so that it never flickers")
	}

	advance(screen, clock, 2*time.Second)
	if !screen.spinnerShowing() {
		t.Error("the spinner went at three seconds, and it appeared at five hundred milliseconds, so it owes another half second")
	}

	advance(screen, clock, 600*time.Millisecond)
	if screen.spinnerShowing() {
		t.Error("the spinner was still there after its three seconds were up, and nothing is waiting on the program")
	}
}

func TestTheSpinnerIsDrawnOnlyInTheStatusStrip(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.remember(block{kind: blockReply, text: "a reply that is already on the screen"})
	screen.setState(stateThinking, "")
	advance(screen, clock, 600*time.Millisecond)

	rows := strings.Split(screen.View(), "\n")
	for number, line := range rows[:len(rows)-1] {
		if strings.Contains(line, "·  ") || strings.Contains(line, "  ·") {
			t.Errorf("row %d is %q, and the spinner is only ever drawn in the status strip", number+1, line)
		}
	}
}

func TestTheSpinnerWalksThroughItsFourFrames(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.setState(stateThinking, "")
	advance(screen, clock, spinnerDelay)

	seen := []string{}
	for range 2 * len(spinnerFrames) {
		seen = append(seen, screen.spinnerFrame())
		advance(screen, clock, spinnerRate)
	}
	wanted := append(append([]string{}, spinnerFrames...), spinnerFrames...)
	for at := range wanted {
		if seen[at] != wanted[at] {
			t.Fatalf("step %d of the spinner drew %q, and the four-frame dot cycle draws %q there", at, seen[at], wanted[at])
		}
	}
}
