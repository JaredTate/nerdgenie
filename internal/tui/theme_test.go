package tui

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
var everyPaintedStyle = []style{
	styleNormal, styleDim, styleAccent, styleBrand, styleDone, styleBold, styleItalic, styleChip,
	styleCard, styleCardDim, styleWarn, styleBad, styleKey, styleMeterOn, styleMeterOff, styleBar, stylePersonBar,
}

// theDigiBytePalette is the colours the screen is allowed, as the brief writes
// them: the five DigiByte colours with the one lighter tint of the accent that
// is allowed for small text, and the four the second look added, the deeper
// navy of a card, the amber of a warning, the red of a failure, and the muted
// blue of an empty meter cell. Nothing else may be written as a colour
// anywhere in the package.
var theDigiBytePalette = []string{
	"#002352", "#FFFFFF", "#0066CC", "#4DA3FF", "#8FA9CC", "#3DDC84",
	"#011B40", "#FFB020", "#FF5C5C", "#5C7BA6",
}

// everyTone is every colour of the palette by its name, so that a test can
// walk them.
var everyTone = []tone{groundTone, textTone, dimTone, accentTone, accentTextTone, doneTone, cardTone, warnTone, badTone, mutedTone}

// shapeOnlyStyles are the styles that draw shapes and never words: the brand
// blue of the block wordmark, the checklist's rules and the pointer; the empty
// cells of a meter, which are meant to recede behind the filled ones; and the
// bar down the side of a card. WCAG's rule for a graphic is three to one,
// where a word needs four and a half.
var shapeOnlyStyles = []style{styleBrand, styleMeterOff, styleBar, stylePersonBar}

// The two contrast floors WCAG 2 gives: four and a half to one for words, and
// three to one for a graphic that carries meaning.
const (
	wordContrast  = 4.5
	shapeContrast = 3.0
)

// relativeLuminance is how bright a colour is to the eye, worked out the way
// WCAG 2 defines it: each of the three channels is taken out of the sRGB
// curve and then weighted, green the most and blue the least.
func relativeLuminance(one tone) float64 {
	red, green, blue := one.redGreenBlue()
	channel := func(value int) float64 {
		part := float64(value) / 255
		if part <= 0.03928 {
			return part / 12.92
		}
		return math.Pow((part+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(red) + 0.7152*channel(green) + 0.0722*channel(blue)
}

// contrastRatio is WCAG 2's contrast between two colours: the brighter one's
// luminance over the darker one's, each with a little added so that black on
// black is not a division by nothing.
func contrastRatio(front tone, back tone) float64 {
	lighter, darker := relativeLuminance(front), relativeLuminance(back)
	if darker > lighter {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

// TestEveryStyleHasEnoughContrastToRead computes WCAG's contrast for every
// paint's letters on its ground at twenty-four bit colour and fails any style
// that carries words under four and a half to one, and any shape-only style
// under three to one. The brand blue is the one exception the palette admits
// to: DigiByte's own blue on DigiByte's own navy is two and three quarters to
// one, and it is kept for the block letters, the rules and the pointer, each
// of which is read by its shape. It is pinned here so that it cannot drift
// darker without anyone noticing.
func TestEveryStyleHasEnoughContrastToRead(t *testing.T) {
	for _, chosen := range everyPaintedStyle {
		drawn := paintFor(chosen)
		ratio := contrastRatio(drawn.front, drawn.back)
		needed := wordContrast
		if slices.Contains(shapeOnlyStyles, chosen) {
			needed = shapeContrast
		}
		if chosen == styleBrand {
			needed = 2.7
		}
		if ratio < needed {
			t.Errorf("the style %d draws %s on %s at a contrast of %.2f to one, and it needs %.1f to be read", chosen, drawn.front.color, drawn.back.color, ratio, needed)
		}
	}
	if ratio := contrastRatio(accentTone, groundTone); ratio >= shapeContrast {
		t.Errorf("the brand blue on the ground now reads at %.2f to one, so the exception for it above can go", ratio)
	}
}

// TestTheSecondLooksTonesStillReadOnALesserTerminal holds that the four
// colours the second look added land away from the ground on a terminal with
// sixteen colours or two hundred and fifty-six, the way the first six do: a
// card the same colour as the ground is no card.
func TestTheSecondLooksTonesStillReadOnALesserTerminal(t *testing.T) {
	for name, depth := range map[string]colorDepth{"sixteen colours": depthSixteen, "two hundred and fifty-six colours": depth256} {
		ground := backgroundOf(groundTone, depth)
		for _, one := range []tone{cardTone, warnTone, badTone, mutedTone} {
			if painted := backgroundOf(one, depth); painted == ground {
				t.Errorf("on %s the colour %s is painted %q, the same as the ground, so nothing drawn in it can be seen", name, one.color, painted)
			}
		}
	}
	for _, chosen := range []style{styleCard, styleCardDim, styleKey, styleBar, stylePersonBar} {
		if painted := themeFor(map[string]string{"TERM": "xterm"}).wrap(chosen, "text"); !strings.Contains(painted, backgroundOf(cardTone, depthSixteen)) {
			t.Errorf("the style %d draws %q on a sixteen-colour terminal, and it sits on the card fill", chosen, painted)
		}
	}
}

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
		{styleCard, textTone, "the words on a card are white"},
		{styleCardDim, dimTone, "the quiet words on a card are the dim blue"},
		{styleWarn, warnTone, "a warning is amber"},
		{styleBad, badTone, "a failure is red"},
		{styleKey, textTone, "a key in the footer is white"},
		{styleMeterOn, accentTextTone, "a filled meter cell is the accent"},
		{styleMeterOff, mutedTone, "an empty meter cell is muted"},
		{styleBar, accentTone, "the agent's bar is DigiByte's own blue"},
		{stylePersonBar, accentTextTone, "the person's bar is the light blue"},
	} {
		if painted := colors.wrap(one.chosen, "text"); !strings.HasPrefix(painted, foregroundOf(one.front, depthTruecolor)) {
			t.Errorf("the style %d draws %q, and %s", one.chosen, painted, one.saying)
		}
	}
	for _, chosen := range []style{styleCard, styleCardDim, styleKey, styleBar, stylePersonBar} {
		if painted := colors.wrap(chosen, "text"); !strings.Contains(painted, backgroundOf(cardTone, depthTruecolor)) {
			t.Errorf("the style %d draws %q, and it sits on the deeper navy of a card", chosen, painted)
		}
	}
	for _, chosen := range []style{styleKey, styleBar, stylePersonBar} {
		if painted := colors.wrap(chosen, "text"); !strings.Contains(painted, boldCode) {
			t.Errorf("the style %d draws %q, and a key and a bar are bold", chosen, painted)
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
		{cardTone, [3]int{1, 27, 64}},
		{warnTone, [3]int{255, 176, 32}},
		{badTone, [3]int{255, 92, 92}},
		{mutedTone, [3]int{92, 123, 166}},
	} {
		red, green, blue := one.named.redGreenBlue()
		if [3]int{red, green, blue} != one.wanted {
			t.Errorf("the colour %q reads back as %d %d %d, and it was written as %v", one.named.color, red, green, blue, one.wanted)
		}
	}
}

func TestTheNearestOfTheTwoHundredAndFiftySixColoursIsInsideTheCube(t *testing.T) {
	for _, one := range everyTone {
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
