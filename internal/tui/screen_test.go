package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
)

// startOfTest is the moment every fake clock in these tests begins at, so that
// nothing here depends on the real time of day.
var startOfTest = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

// plainEnvironment answers the environment questions the screen asks with
// colour switched off and nothing else set, which is what keeps the golden files
// readable as plain text.
func plainEnvironment(name string) string {
	if name == "NO_COLOR" {
		return "1"
	}
	return ""
}

// testkitClock is a fake clock starting at the moment every test here begins at.
func testkitClock() *testkit.FakeClock {
	return testkit.NewFakeClock(startOfTest)
}

// newTestScreen builds a screen of one size with a fake clock and no link to a
// running program, which is the state the first frame is drawn in.
func newTestScreen(width int, height int) (*Screen, *testkit.FakeClock) {
	clock := testkitClock()
	screen := New(Options{
		Clock:       clock,
		Environment: plainEnvironment,
		Width:       width,
		Height:      height,
	})
	return screen, clock
}

func TestTheFirstFrameIsDrawnBeforeTheSocketConnects(t *testing.T) {
	screen, _ := newTestScreen(80, 24)

	frame := screen.frame()
	testkit.Golden(t, "first-frame.txt", []byte(frame))

	lines := strings.Split(frame, "\n")
	if len(lines) != 24 {
		t.Fatalf("the frame is %d rows and the terminal is 24 rows", len(lines))
	}
	for number, line := range lines {
		if width := len([]rune(line)); width > 80 {
			t.Errorf("row %d is %d columns wide and the terminal is 80 columns: %q", number+1, width, line)
		}
	}
	if !strings.Contains(lines[0], "coeus · connecting") {
		t.Errorf("the header is %q, and it should name the program and say it is connecting", lines[0])
	}
	if !strings.Contains(lines[23], "connecting") {
		t.Errorf("the status strip is %q, and it should say connecting", lines[23])
	}
	if !strings.Contains(lines[22], "›") {
		t.Errorf("the input row is %q, and it should hold the prompt glyph", lines[22])
	}
}
