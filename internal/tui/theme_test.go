package tui

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
var everyPaintedStyle = []style{styleNormal, styleDim, styleAccent, styleBrand, styleDone, styleBold, styleItalic, styleChip}

// theDigiBytePalette is the five colours the screen is allowed, as the brief
// writes them, with the one lighter tint of the accent that is allowed for
// small text. Nothing else may be written as a colour anywhere in the package.
var theDigiBytePalette = []string{"#002352", "#FFFFFF", "#0066CC", "#4DA3FF", "#8FA9CC", "#3DDC84"}

// hexColours matches a colour written as a hash and six hex digits.
var hexColours = regexp.MustCompile(`#[0-9A-Fa-f]{6}\b`)

// TestNoColourOutsideTheDigiBytePaletteIsWrittenAnywhereInThePackage reads
// every source file of the package for a colour written as hex digits and
// holds that the set it finds is exactly the palette, so that a stray colour
// added anywhere is caught, not only one added to the palette's own list.
func TestNoColourOutsideTheDigiBytePaletteIsWrittenAnywhereInThePackage(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package's source files failed: %v", err)
	}
	written := map[string]string{}
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s failed: %v", path, err)
		}
		for _, found := range hexColours.FindAllString(string(source), -1) {
			written[strings.ToUpper(found)] = path
		}
	}
	allowed := map[string]bool{}
	for _, one := range theDigiBytePalette {
		allowed[one] = true
	}
	for colour, path := range written {
		if !allowed[colour] {
			t.Errorf("%s writes the colour %s, and the palette is the five DigiByte colours %v and nothing else", path, colour, theDigiBytePalette)
		}
	}
	found := []string{}
	for colour := range written {
		found = append(found, colour)
	}
	sort.Strings(found)
	wanted := append([]string{}, theDigiBytePalette...)
	sort.Strings(wanted)
	if strings.Join(found, " ") != strings.Join(wanted, " ") {
		t.Errorf("the package writes the colours %v, and the palette is exactly %v", found, wanted)
	}
}

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

// TestEachStyleDrawsTheColourItIsNamedFor pins which colour of the palette each
// style draws its letters in: the accent text is the lighter tint, the brand
// style is DigiByte's own blue, a check mark is green, and the wish line is dim
// letters slanted.
func TestEachStyleDrawsTheColourItIsNamedFor(t *testing.T) {
	colors := themeFor(map[string]string{"COLORTERM": "truecolor"})
	for _, one := range []struct {
		chosen style
		front  tone
		saying string
	}{
		{styleNormal, textTone, "ordinary words are white"},
		{styleDim, dimTone, "quiet words are the dim blue"},
		{styleAccent, accentTextTone, "accent text is the lighter tint of the accent"},
		{styleBrand, accentTone, "the brand style is DigiByte's own blue"},
		{styleDone, doneTone, "a check mark is green"},
		{styleItalic, dimTone, "the wish line is dim"},
	} {
		if painted := colors.wrap(one.chosen, "text"); !strings.HasPrefix(painted, foregroundOf(one.front, depthTruecolor)) {
			t.Errorf("the style %d draws %q, and %s", one.chosen, painted, one.saying)
		}
	}
	if slanted := colors.wrap(styleItalic, "text"); !strings.Contains(slanted, italicCode) {
		t.Errorf("the wish line is drawn %q, and it is slanted", slanted)
	}
	for _, chosen := range []style{styleNormal, styleDim, styleAccent, styleBrand, styleDone, styleBold, styleChip} {
		if painted := colors.wrap(chosen, "text"); strings.Contains(painted, italicCode) {
			t.Errorf("the style %d draws %q slanted, and only the wish line is", chosen, painted)
		}
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

// TestTheGroundAndTheAccentStillReadOnALesserTerminal holds that the ground and
// the accent land on two different colours when the terminal has only the
// sixteen ordinary colours or the two hundred and fifty-six colour table,
// because a check mark drawn in the same colour as its ground is no check mark.
func TestTheGroundAndTheAccentStillReadOnALesserTerminal(t *testing.T) {
	for name, depth := range map[string]colorDepth{"sixteen colours": depthSixteen, "two hundred and fifty-six colours": depth256} {
		ground := backgroundOf(groundTone, depth)
		for _, one := range []tone{textTone, dimTone, accentTone, accentTextTone, doneTone} {
			if painted := backgroundOf(one, depth); painted == ground {
				t.Errorf("on %s the colour %s is painted %q, the same as the ground, so nothing drawn in it can be seen", name, one.color, painted)
			}
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
		{groundTone, [3]int{0, 35, 82}},
		{textTone, [3]int{255, 255, 255}},
		{dimTone, [3]int{143, 169, 204}},
		{accentTone, [3]int{0, 102, 204}},
		{accentTextTone, [3]int{77, 163, 255}},
		{doneTone, [3]int{61, 220, 132}},
	} {
		red, green, blue := one.named.redGreenBlue()
		if [3]int{red, green, blue} != one.wanted {
			t.Errorf("the colour %q reads back as %d %d %d, and it was written as %v", one.named.color, red, green, blue, one.wanted)
		}
	}
}

func TestTheNearestOfTheTwoHundredAndFiftySixColoursIsInsideTheCube(t *testing.T) {
	for _, one := range []tone{groundTone, textTone, dimTone, accentTone, accentTextTone, doneTone} {
		if nearest := one.nearest256(); nearest < 16 || nearest > 231 {
			t.Errorf("the colour %q lands on %d, and the six-by-six-by-six cube runs from 16 to 231", one.color, nearest)
		}
	}
}

// TestTheViewAsksTheTerminalForTheDigiByteGroundBehindWhiteLetters holds that
// the screen hands Bubble Tea the ground and the text colour as the terminal's
// own background and foreground for as long as the program runs, so that every
// cell is DigiByte dark blue whether or not a row painted it, and that a
// terminal told to use no colour is asked for nothing at all.
func TestTheViewAsksTheTerminalForTheDigiByteGroundBehindWhiteLetters(t *testing.T) {
	themed := newThemedScreen(80, 24)
	shown := themed.View()
	if shown.BackgroundColor != (color.RGBA{R: 0, G: 35, B: 82, A: 255}) {
		t.Errorf("the view asks the terminal for the background %v, and the ground is DigiByte dark blue #002352", shown.BackgroundColor)
	}
	if shown.ForegroundColor != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("the view asks the terminal for the foreground %v, and the letters are white", shown.ForegroundColor)
	}

	plain, _ := newTestScreen(80, 24)
	if shown := plain.View(); shown.BackgroundColor != nil || shown.ForegroundColor != nil {
		t.Errorf("a terminal told NO_COLOR is still asked for the colours %v and %v", shown.BackgroundColor, shown.ForegroundColor)
	}
}
