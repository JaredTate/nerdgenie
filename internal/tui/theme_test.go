package tui

import (
	"strings"
	"testing"
)

// themeFor builds a theme from a handful of environment settings, which is the
// only thing the colours depend on.
func themeFor(settings map[string]string) theme {
	return newTheme(func(name string) string { return settings[name] })
}

// everyPaintedStyle is every style that draws in colour, which is every style
// but the cursor.
var everyPaintedStyle = []style{styleNormal, styleDim, styleAccent, styleWarn, styleError, styleBold, styleChip}

func TestTheGroundIsPaintedBehindEveryStyleSoThereAreNoDarkGaps(t *testing.T) {
	colors := themeFor(map[string]string{"COLORTERM": "truecolor"})
	for _, chosen := range everyPaintedStyle {
		painted := colors.wrap(chosen, "text")
		if !strings.Contains(painted, "\x1b[48;") {
			t.Errorf("the style %d draws %q with no background at all, and every row is painted edge to edge", chosen, painted)
		}
		if !strings.HasSuffix(painted, resetCode) {
			t.Errorf("the style %d draws %q without putting the terminal back, so the colour would run on", chosen, painted)
		}
	}
	if !strings.Contains(colors.wrap(styleNormal, "text"), backgroundOf(groundTone, depthTruecolor)) {
		t.Error("ordinary text is not drawn on the DigiByte ground")
	}
	if !strings.Contains(colors.wrap(styleChip, "text"), backgroundOf(accentTone, depthTruecolor)) {
		t.Error("a chip is not drawn on the accent, and the pills and the buttons are chips")
	}
}

func TestTheColourDepthFallsBackFromTruecolourToTwoFiftySixAndToSixteen(t *testing.T) {
	for name, expected := range map[string]struct {
		settings map[string]string
		holds    string
	}{
		"a truecolour terminal": {map[string]string{"COLORTERM": "truecolor", "TERM": "xterm-256color"}, "\x1b[38;2;255;255;255m"},
		"a direct-colour term":  {map[string]string{"TERM": "xterm-direct"}, "\x1b[38;2;255;255;255m"},
		"a 256-colour terminal": {map[string]string{"TERM": "xterm-256color"}, "\x1b[38;5;231m"},
		"a 16-colour terminal":  {map[string]string{"TERM": "xterm"}, "\x1b[97m"},
	} {
		painted := themeFor(expected.settings).wrap(styleNormal, "text")
		if !strings.Contains(painted, expected.holds) {
			t.Errorf("on %s the letters are drawn %q, and they should hold %q", name, painted, expected.holds)
		}
	}
}

func TestATerminalThatPromisesNothingAndOneToldNoColourAreBothDrawnPlain(t *testing.T) {
	for name, settings := range map[string]map[string]string{
		"a terminal that says nothing": {},
		"a terminal that says dumb":    {"TERM": "dumb"},
		"a terminal told NO_COLOR":     {"NO_COLOR": "1", "COLORTERM": "truecolor"},
	} {
		colors := themeFor(settings)
		for _, chosen := range everyPaintedStyle {
			if painted := colors.wrap(chosen, "text"); painted != "text" {
				t.Errorf("on %s the style %d drew %q, and a plain terminal gets plain letters", name, chosen, painted)
			}
		}
		if cursor := colors.wrap(styleReverse, "l"); cursor != reverseCode+"l"+resetCode {
			t.Errorf("on %s the cursor drew %q, and swapping the foreground and the background is not a colour", name, cursor)
		}
	}
}

func TestEveryPaletteColourReadsBackAsTheRedGreenAndBlueItWasWritten(t *testing.T) {
	for _, one := range []struct {
		named  tone
		wanted [3]int
	}{
		{groundTone, [3]int{30, 144, 255}},
		{textTone, [3]int{255, 255, 255}},
		{dimTone, [3]int{207, 230, 255}},
		{accentTone, [3]int{0, 102, 204}},
		{warnTone, [3]int{255, 209, 102}},
		{troubleTone, [3]int{255, 107, 107}},
	} {
		red, green, blue := one.named.redGreenBlue()
		if [3]int{red, green, blue} != one.wanted {
			t.Errorf("the colour %q reads back as %d %d %d, and it was written as %v", one.named.color, red, green, blue, one.wanted)
		}
	}
}

func TestTheNearestOfTheTwoHundredAndFiftySixColoursIsInsideTheCube(t *testing.T) {
	for _, one := range []tone{groundTone, textTone, dimTone, accentTone, warnTone, troubleTone} {
		if nearest := one.nearest256(); nearest < 16 || nearest > 231 {
			t.Errorf("the colour %q lands on %d, and the six-by-six-by-six cube runs from 16 to 231", one.color, nearest)
		}
	}
}
