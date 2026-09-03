// The same call made again and again: the trial saw one shell command fill the
// screen thirteen times over, because a model that has got stuck runs the same
// line until its budget is spent. The pill that call already has takes the
// newest line and a count instead, so the screen says how many times in one row.
package tui

import (
	"strconv"
	"strings"
	"testing"
)

// theStuckCommand is the shell line the trial saw repeated.
const theStuckCommand = "▸ shell make test"

// runTheStuckCommand sends the tool line for one more run of the same command:
// the line while it runs, and the same line with its result once it has.
func runTheStuckCommand(screen *Screen, call int) {
	send(screen, aToolLine(theStuckCommand))
	send(screen, aToolLine(theStuckCommand+" · r"+strconv.Itoa(call)+" shell: 12 lines"))
}

func TestTheSameCallMadeAgainAndAgainIsOnePillWithACount(t *testing.T) {
	screen := aTrialScreen()
	for call := 1; call <= 13; call++ {
		runTheStuckCommand(screen, call)
	}

	frame := plainText(screen.frame())
	if drawn := strings.Count(frame, "shell make test"); drawn != 1 {
		t.Errorf("the same command run thirteen times is drawn %d times, and it should be one pill with a count:\n%s", drawn, frame)
	}
	if !strings.Contains(frame, "× 13") {
		t.Errorf("the pill does not say how many times the command was run, and the count is the whole point:\n%s", frame)
	}
	if !strings.Contains(frame, "r13") || strings.Contains(frame, "r12") {
		t.Errorf("the pill should carry the newest run's result and not an older one:\n%s", frame)
	}
}

func TestADifferentCallBetweenTwoOfTheSameKeepsThemApart(t *testing.T) {
	screen := aTrialScreen()
	runTheStuckCommand(screen, 1)
	send(screen, aToolLine("▸ read note.txt"))
	send(screen, aToolLine("▸ read note.txt · r2 read: 1 line"))
	runTheStuckCommand(screen, 3)

	if said := pillTexts(screen); len(said) != 3 {
		t.Errorf("two runs with another call between them drew the pills %q, and a count is only for the same call in a row", said)
	}
	if frame := plainText(screen.frame()); strings.Contains(frame, string(repeatGlyph)) {
		t.Errorf("a count is drawn where no call was made twice in a row:\n%s", frame)
	}
}

func TestAHeartbeatCarryingTheSameLineIsNotACallMadeAgain(t *testing.T) {
	screen := aTrialScreen()
	for range 8 {
		send(screen, aToolLine(theStuckCommand))
	}

	if frame := plainText(screen.frame()); strings.Contains(frame, string(repeatGlyph)) {
		t.Errorf("the same line on eight heartbeats is one call, and it was given a count:\n%s", frame)
	}
}
