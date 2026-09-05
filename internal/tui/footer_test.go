package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// hintsRowOf is the last row of the frame, which the footer makes its row of
// key hints, with the escape codes taken out.
func hintsRowOf(screen *Screen) string {
	rows := strings.Split(plainText(screen.frame()), "\n")
	return rows[len(rows)-1]
}

// withExtraHints runs a test with the other worker's hints standing in, and
// puts the package's own variable back after it.
func withExtraHints(t *testing.T, hints []string) {
	t.Helper()
	before := extraHints
	extraHints = func() []string { return hints }
	t.Cleanup(func() { extraHints = before })
}

// TestTheFooterIsTwoRowsTheStatusStripAndThenTheKeyHints holds the shape of
// the footer: the status strip as it was, one row up, and under it a row of
// the keys the screen answers to, each key and its verb, with the stop key
// shown only while there is something to stop.
func TestTheFooterIsTwoRowsTheStatusStripAndThenTheKeyHints(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	rows := strings.Split(plainText(screen.frame()), "\n")
	strip, hints := rows[len(rows)-2], rows[len(rows)-1]
	if !strings.Contains(strip, "connecting") {
		t.Errorf("the row above the hints is %q, and it is the status strip", strip)
	}
	for _, wanted := range []string{"↵  send", "^J  newline", "^C  quit"} {
		if !strings.Contains(hints, wanted) {
			t.Errorf("the hints row is %q, and it should hold %q", hints, wanted)
		}
	}
	if strings.Contains(hints, "esc") {
		t.Errorf("the hints row is %q, and Escape stops nothing while nothing runs", hints)
	}
	if strings.Contains(strip, "send") || strings.Contains(strip, "quit") {
		t.Errorf("the status strip is %q, and the key hints have moved off it", strip)
	}

	screen.setState(stateThinking, "")
	if hints := hintsRowOf(screen); !strings.Contains(hints, "esc  stop") {
		t.Errorf("the hints row is %q while the model thinks, and Escape stops it", hints)
	}
}

// TestEachHintIsAKeycapAndItsVerbTwoBlanksApart pins the drawing in OpenCode's
// style: the key in bold white on the card fill with a blank of padding on
// each side, its verb dim after it, and two blanks between one hint and the
// next.
func TestEachHintIsAKeycapAndItsVerbTwoBlanksApart(t *testing.T) {
	screen := newThemedScreen(80, 24)
	colors := screen.colors
	rows := strings.Split(screen.frame(), "\n")
	hints := rows[len(rows)-1]
	wanted := colors.wrap(styleKey, " ↵ ") + colors.wrap(styleDim, " send") + colors.wrap(styleNormal, "  ") + colors.wrap(styleKey, " ^J ") + colors.wrap(styleDim, " newline")
	if !strings.Contains(hints, wanted) {
		t.Errorf("the hints row is %q, and it should hold %q", hints, wanted)
	}
}

// TestTheHintsRowTakesTheOtherWorkersHintsAndDropsWholeHintsFromTheRight
// holds the seam with focus.go: whatever extraHints answers is drawn after
// the screen's own hints, and a terminal too narrow for them all loses whole
// hints from the right rather than half of one.
func TestTheHintsRowTakesTheOtherWorkersHintsAndDropsWholeHintsFromTheRight(t *testing.T) {
	withExtraHints(t, []string{"tab focus", "↵ expand", "^B panel", "⇞⇟ scroll"})

	wide, _ := newTestScreen(120, 24)
	hints := hintsRowOf(wide)
	for _, wanted := range []string{"↵  send", "^J  newline", "^C  quit", "tab  focus", "↵  expand", "^B  panel", "⇞⇟  scroll"} {
		if !strings.Contains(hints, wanted) {
			t.Errorf("at a hundred and twenty columns the hints row is %q, and it should hold %q", hints, wanted)
		}
	}
	if strings.Index(hints, "quit") > strings.Index(hints, "focus") {
		t.Errorf("the hints row is %q, and the other worker's hints come after the screen's own", hints)
	}

	narrow, _ := newTestScreen(40, 24)
	hints = hintsRowOf(narrow)
	if displayWidth(hints) > 40 {
		t.Errorf("at forty columns the hints row is %d columns wide: %q", displayWidth(hints), hints)
	}
	if !strings.Contains(hints, "↵  send") {
		t.Errorf("at forty columns the hints row is %q, and the first hint always fits", hints)
	}
	if strings.Contains(hints, "scroll") {
		t.Errorf("at forty columns the hints row is %q, and the last hints are dropped", hints)
	}
	for _, pair := range [][2]string{{"^J", "newline"}, {"^C", "quit"}, {"tab", "focus"}, {"^B", "panel"}} {
		if strings.Contains(hints, pair[0]+" ") != strings.Contains(hints, pair[1]) {
			t.Errorf("at forty columns the hints row is %q, and a hint is drawn whole or not at all", hints)
		}
	}
}

// TestTheHintsChangeWithWhatTheScreenIsWaitingFor holds that a card with
// single-key answers, the palette, and a masked prompt each put their own
// keys in the footer, because the keys the person can press are not the same
// in each.
func TestTheHintsChangeWithWhatTheScreenIsWaitingFor(t *testing.T) {
	card, _ := screenWithLink()
	send(card, aPreview())
	if hints := hintsRowOf(card); !strings.Contains(hints, "a  approve") || !strings.Contains(hints, "A  always") || !strings.Contains(hints, "r  reject") {
		t.Errorf("with a preview waiting the hints row is %q, and it names the three answers", hints)
	}

	palette, _ := screenWithCommands()
	press(palette, '/')
	if hints := hintsRowOf(palette); !strings.Contains(hints, "tab  complete") || !strings.Contains(hints, "esc  close") {
		t.Errorf("with the palette open the hints row is %q, and it names Tab and Escape", hints)
	}

	secret, _ := screenWithLink()
	send(secret, contract.SocketEnvelope{Type: contract.SocketAsk, ID: "s1", Title: "the vault password", MaskInput: true})
	if hints := hintsRowOf(secret); !strings.Contains(hints, "esc  cancel") || !strings.Contains(hints, "nothing is shown") {
		t.Errorf("with a masked prompt the hints row is %q, and it says Escape cancels and nothing is shown", hints)
	}
}

// TestAHintIsItsFirstWordAsTheKeyAndTheRestAsTheVerb holds the reading of a
// hint such as "⇞⇟ scroll" into the keycap and the words, and that a hint
// with no verb is drawn as a key alone.
func TestAHintIsItsFirstWordAsTheKeyAndTheRestAsTheVerb(t *testing.T) {
	for _, one := range []struct {
		hint, key, verb string
	}{
		{"↵ send", "↵", "send"}, {"⇞⇟ scroll", "⇞⇟", "scroll"}, {"↵ send the reason", "↵", "send the reason"}, {"esc", "esc", ""},
	} {
		if key, verb := keyAndVerb(one.hint); key != one.key || verb != one.verb {
			t.Errorf("the hint %q reads as the key %q and the verb %q, want %q and %q", one.hint, key, verb, one.key, one.verb)
		}
	}
}
