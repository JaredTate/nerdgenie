// A paste is text the terminal hands over all at once rather than key by key.
// It goes into the input box exactly as typing would, so a person can paste a
// prompt, and the two markers the terminal sends around it do nothing on their
// own.
package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAPasteGoesIntoTheInputBoxLikeTyping(t *testing.T) {
	screen := aTrialScreen()
	screen.Update(tea.PasteMsg{Content: "build me a tic-tac-toe game"})
	if got := screen.input.text(); got != "build me a tic-tac-toe game" {
		t.Fatalf("the paste did not land in the input box: %q", got)
	}
	press(screen, '!')
	if got := screen.input.text(); got != "build me a tic-tac-toe game!" {
		t.Fatalf("typing after the paste did not continue at its end: %q", got)
	}
}

func TestAPasteKeepsItsLineBreaksAsPlainOnes(t *testing.T) {
	screen := aTrialScreen()
	screen.Update(tea.PasteMsg{Content: "first line\r\nsecond line\rthird line"})
	if got := screen.input.text(); got != "first line\nsecond line\nthird line" {
		t.Fatalf("line ends were not made plain: %q", got)
	}
}

func TestAPasteLongerThanTheBoxIsCutAtTheCap(t *testing.T) {
	screen := aTrialScreen()
	screen.Update(tea.PasteMsg{Content: strings.Repeat("x", maxInputRunes+50)})
	if got := len(screen.input.letters); got != maxInputRunes {
		t.Fatalf("a runaway paste was not cut at the cap: %d letters", got)
	}
}

func TestThePasteMarkersOnTheirOwnChangeNothing(t *testing.T) {
	screen := aTrialScreen()
	screen.Update(tea.PasteStartMsg{})
	screen.Update(tea.PasteEndMsg{})
	if got := screen.input.text(); got != "" {
		t.Fatalf("the paste markers put something in the box: %q", got)
	}
}
