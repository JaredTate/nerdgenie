package tui

import (
	"image/color"
	"strconv"
	"strings"
)

// The DigiByte palette, written as the six hex digits the frame is designed in
// and named once, here. The first five are DigiByte's: the dark blue ground,
// white letters, DigiByte's own blue for the accent, a desaturated light blue
// for the quiet parts, and a green for check marks. The accent alone has a
// second, lighter tint, because pure DigiByte blue on the dark blue ground is
// too low in contrast for small text; the pure blue keeps the big shapes, which
// are the wordmark's GENIE, the rules, the pointer, and the bar down the side
// of the agent's card. The second look added four more: a navy a step deeper
// than the ground for the cards, the pills and the keycaps, an amber and a red
// for the meters and the failures, and a muted blue for an empty meter cell.
// Each colour also names the nearest of the sixteen colours every terminal
// has, for the terminals that have no more. Colour never carries a meaning on
// its own: a glyph carries it too, so a terminal with no colour still reads
// the frame.
//
// The escape codes are written here rather than by a terminal styling library,
// because such a library reports "no colour at all" whenever the writer it was
// built on is not a terminal, which every test process is; painting through one
// would drop the colours out of exactly the golden files that have to prove
// them.
var (
	// groundTone is DigiByte's dark blue, behind every row of the frame and,
	// for as long as the program runs, behind the terminal itself.
	groundTone = tone{color: "#002352", basic: 4}
	// textTone is the white every ordinary word is drawn in.
	textTone = tone{color: "#FFFFFF", basic: 15}
	// dimTone is the desaturated light blue the quiet parts of the frame are
	// drawn in: the header, the rules, the status strip, and every label.
	dimTone = tone{color: "#8FA9CC", basic: 7}
	// accentTone is DigiByte's own blue, which draws the wordmark's GENIE, the
	// rules of the checklist, the pointer beside the running task, and fills
	// the pills, the buttons and the person's bubble.
	accentTone = tone{color: "#0066CC", basic: 12}
	// accentTextTone is the lighter tint of the accent that small accent text
	// is drawn in, such as the prompt glyph, a job's number, and the progress
	// line, because the pure blue is too dark to read at that size on the
	// ground.
	accentTextTone = tone{color: "#4DA3FF", basic: 12}
	// doneTone is the green of a check mark, of a context with room to spare,
	// of a warm cache, and of a test line that says every test passes.
	doneTone = tone{color: "#3DDC84", basic: 10}
	// cardTone is a navy a step deeper than the ground. It fills the flat
	// cards the talk is drawn on, the tool pills, and the keycaps in the
	// footer, so that each sits a little below the ground rather than inside
	// a box. On a sixteen-colour terminal it is black, the one colour that
	// reads as deeper than the ground's blue.
	cardTone = tone{color: "#011B40", basic: 0}
	// warnTone is the amber of a measure that is getting high: a context past
	// half full, a cache only partly warm.
	warnTone = tone{color: "#FFB020", basic: 11}
	// badTone is the red of something wrong: a context nearly full, a cold
	// cache, a call that failed, a failure in the record, a test line that
	// says tests are failing.
	badTone = tone{color: "#FF5C5C", basic: 9}
	// mutedTone is the quiet blue-grey of an empty meter cell, which is meant
	// to recede behind the filled cells beside it.
	mutedTone = tone{color: "#5C7BA6", basic: 8}
)

// The escape codes that are not colours: bold letters, slanted letters, and the
// swap of the foreground and the background that draws the cursor on a terminal
// told to use no colour at all. resetCode puts the terminal back after every
// piece.
const (
	boldCode    = "\x1b[1m"
	italicCode  = "\x1b[3m"
	reverseCode = "\x1b[7m"
	resetCode   = "\x1b[0m"
)

// tone is one colour of the palette: the six hex digits it is written as, and
// the nearest of the sixteen colours a terminal with nothing better can draw.
type tone struct {
	// color is the colour itself, as a hash and six hex digits.
	color string
	// basic is the number of the nearest of the sixteen ordinary colours.
	basic int
}

// redGreenBlue reads a palette colour back as the three numbers a terminal
// wants, and reads as black when the colour was not written as six hex digits.
func (one tone) redGreenBlue() (int, int, int) {
	digits := strings.TrimPrefix(one.color, "#")
	if len(digits) != 6 {
		return 0, 0, 0
	}
	parts := [3]int{}
	for at := range parts {
		read, err := strconv.ParseInt(digits[2*at:2*at+2], 16, 32)
		if err != nil {
			return 0, 0, 0
		}
		parts[at] = int(read)
	}
	return parts[0], parts[1], parts[2]
}

// rgba is the colour as the standard library writes one, which is what Bubble
// Tea takes when it is asked to set the terminal's own colours.
func (one tone) rgba() color.Color {
	red, green, blue := one.redGreenBlue()
	return color.RGBA{R: uint8(red), G: uint8(green), B: uint8(blue), A: 0xff}
}

// nearest256 is where this colour lands in the six-by-six-by-six cube that fills
// the middle of the two hundred and fifty-six colour table, which runs from
// sixteen to two hundred and thirty-one.
func (one tone) nearest256() int {
	red, green, blue := one.redGreenBlue()
	return 16 + 36*cubeStep(red) + 6*cubeStep(green) + cubeStep(blue)
}

// cubeStep rounds one of the three numbers to the nearest of the six steps the
// colour cube has.
func cubeStep(value int) int {
	return (value*5 + 127) / 255
}

// colorDepth is how much colour this terminal promises it can draw.
type colorDepth int

const (
	// depthNone is a terminal that was told to use no colour, or one that says
	// nothing about itself at all.
	depthNone colorDepth = iota
	// depthSixteen is the sixteen colours every terminal has had since the
	// nineteen eighties.
	depthSixteen
	// depth256 is the two hundred and fifty-six colour table.
	depth256
	// depthTruecolor is twenty-four bit colour, where the palette is drawn
	// exactly as it was written.
	depthTruecolor
)

// style is one of the ways a piece of the frame can be drawn. The colours are
// the DigiByte palette above and nothing else, so that the reply is still the
// loudest thing on the screen even though the whole frame is painted.
type style int

const (
	// styleNormal is white letters on the DigiByte ground, and is what a reply
	// and the words inside a card are drawn in.
	styleNormal style = iota
	// styleDim is the light blue every quiet thing is drawn in: the header, the
	// rules, the status strip, the labels, and everything on the checklist that
	// is not the item the work is at.
	styleDim
	// styleAccent is accent text, in the lighter tint and bold: the prompt
	// glyph, the health dot, a job's number, the progress line, the context
	// share once it is filling up, and the card the person must answer.
	styleAccent
	// styleBrand is DigiByte's own blue, bold, for the big shapes that read at
	// that depth: the wordmark's GENIE, the rules of the checklist, and the
	// pointer beside the running task.
	styleBrand
	// styleDone is the green of a check mark beside something finished.
	styleDone
	// styleBold is white letters made bold, which is the loudest thing on the
	// ground: the wordmark's NERD, the checklist's header, light markdown's
	// bold text and headings, the fill of the budget bar, and a failure.
	styleBold
	// styleItalic is dim letters slanted, which draws the one line of the
	// welcome that is a wish.
	styleItalic
	// styleReverse swaps the letters and the ground, which is how the cursor is
	// drawn when it sits inside the text rather than at the end.
	styleReverse
	// styleChip is white letters on the accent, which is the filled shape the
	// answer buttons on a card are made of.
	styleChip
	// styleCard is white letters on the card fill: the words of a reply, of
	// the person's own message, and of a tool pill's name.
	styleCard
	// styleCardDim is dim letters on the card fill: a pill's argument and its
	// summary, and a code span inside a reply.
	styleCardDim
	// styleWarn is amber on the ground: a measure that is getting high.
	styleWarn
	// styleBad is red on the ground: a failure, a context nearly full, a cold
	// cache, and the glyph of a call that failed.
	styleBad
	// styleKey is bold white on the card fill, which is the keycap a key in
	// the footer is drawn as, the badge a result id sits in at the end of a
	// pill, and the bold text of a reply, since a bold word on a card is the
	// same paint.
	styleKey
	// styleMeterOn is the accent for the filled cells of a meter that has no
	// warning to give, such as the budget bar in the status strip.
	styleMeterOn
	// styleMeterOff is the muted blue of a meter's empty cells.
	styleMeterOff
	// styleBar is DigiByte's own blue on the card fill, which draws the bar
	// down the left of the agent's card, joined to the card by its ground.
	styleBar
	// stylePersonBar is the light blue on the card fill, which draws the bar
	// down the left of the person's card.
	stylePersonBar
)

// theme turns a style into the escape codes that draw it. It answers one
// question once, when the screen is built: how much colour this terminal has.
type theme struct {
	// depth is how much colour the terminal promised it can draw.
	depth colorDepth
}

// newTheme reads the environment for how much colour the terminal has.
func newTheme(environment func(string) string) theme {
	return theme{depth: colorDepthOf(environment)}
}

// colorDepthOf reads the two settings a terminal describes its colours with.
// NO_COLOR turns everything off, COLORTERM is where a terminal says it has
// twenty-four bit colour, and TERM is where it says it has two hundred and
// fifty-six. A terminal that says nothing is drawn plain, because guessing at
// colour a terminal cannot draw leaves escape codes on the screen as letters.
func colorDepthOf(environment func(string) string) colorDepth {
	if environment("NO_COLOR") != "" {
		return depthNone
	}
	deep := strings.ToLower(environment("COLORTERM"))
	terminal := strings.ToLower(environment("TERM"))
	switch {
	case strings.Contains(deep, "truecolor"), strings.Contains(deep, "24bit"), strings.Contains(terminal, "direct"):
		return depthTruecolor
	case terminal == "", terminal == "dumb":
		return depthNone
	case strings.Contains(terminal, "256color"):
		return depth256
	default:
		return depthSixteen
	}
}

// terminalColors are the ground and the text colour as the colours the terminal
// itself is asked to use for as long as the program runs, so that every cell is
// DigiByte dark blue whether or not a row painted it, and nothing at all on a
// terminal told to use no colour.
func (colors theme) terminalColors() (color.Color, color.Color) {
	if colors.depth == depthNone {
		return nil, nil
	}
	return groundTone.rgba(), textTone.rgba()
}

// wrap puts the escape codes for a style around a piece of text. The cursor is
// the one style that survives a terminal with no colour, because swapping the
// letters and the ground is not a colour and the person still has to see where
// they are typing.
func (colors theme) wrap(chosen style, text string) string {
	if text == "" {
		return ""
	}
	if chosen == styleReverse {
		return colors.cursorCode() + text + resetCode
	}
	if colors.depth == depthNone {
		return text
	}
	return colors.code(chosen) + text + resetCode
}

// cursorCode draws the cursor: the palette turned around, so that the cursor is
// a white block with a blue letter in it, and the terminal's own swap of the
// foreground and the background where there is no colour to turn around.
func (colors theme) cursorCode() string {
	if colors.depth == depthNone {
		return reverseCode
	}
	return foregroundOf(groundTone, colors.depth) + backgroundOf(textTone, colors.depth)
}

// code is the escape sequence one style begins with: the colour of its letters,
// the colour of the ground behind them, bold when the style is a loud one, and
// slanted when it is the wish.
func (colors theme) code(chosen style) string {
	if colors.depth == depthNone {
		return ""
	}
	drawn := paintFor(chosen)
	built := foregroundOf(drawn.front, colors.depth) + backgroundOf(drawn.back, colors.depth)
	if drawn.heavy {
		built += boldCode
	}
	if drawn.slanted {
		built += italicCode
	}
	return built
}

// paint is how one style is drawn: the colour of its letters, the colour of the
// ground behind them, and whether the letters are bold or slanted.
type paint struct {
	// front is the colour of the letters.
	front tone
	// back is the colour of the ground behind them.
	back tone
	// heavy says whether the letters are bold.
	heavy bool
	// slanted says whether the letters are italic.
	slanted bool
}

// paintFor says how a style is drawn. Every style is drawn on the DigiByte
// ground, which is what fills the whole frame, except the chip, which sits on
// the accent, and the card styles, which sit on the deeper navy of a card.
func paintFor(chosen style) paint {
	switch chosen {
	case styleDim:
		return paint{front: dimTone, back: groundTone}
	case styleAccent:
		return paint{front: accentTextTone, back: groundTone, heavy: true}
	case styleBrand:
		return paint{front: accentTone, back: groundTone, heavy: true}
	case styleDone:
		return paint{front: doneTone, back: groundTone}
	case styleBold:
		return paint{front: textTone, back: groundTone, heavy: true}
	case styleItalic:
		return paint{front: dimTone, back: groundTone, slanted: true}
	case styleChip:
		return paint{front: textTone, back: accentTone, heavy: true}
	case styleWarn:
		return paint{front: warnTone, back: groundTone}
	case styleBad:
		return paint{front: badTone, back: groundTone}
	case styleMeterOn:
		return paint{front: accentTextTone, back: groundTone}
	case styleMeterOff:
		return paint{front: mutedTone, back: groundTone}
	case styleCard, styleCardDim, styleKey, styleBar, stylePersonBar:
		return cardPaintFor(chosen)
	default:
		return paint{front: textTone, back: groundTone}
	}
}

// cardPaintFor says how the styles that sit on a card are drawn: white and
// dim words, the bold white of a keycap, and the two blues of the bars.
func cardPaintFor(chosen style) paint {
	switch chosen {
	case styleCardDim:
		return paint{front: dimTone, back: cardTone}
	case styleKey:
		return paint{front: textTone, back: cardTone, heavy: true}
	case styleBar:
		return paint{front: accentTone, back: cardTone, heavy: true}
	case stylePersonBar:
		return paint{front: accentTextTone, back: cardTone, heavy: true}
	default:
		return paint{front: textTone, back: cardTone}
	}
}

// foregroundOf is the escape sequence that draws letters in one colour at one
// depth.
func foregroundOf(one tone, depth colorDepth) string {
	return colorCode(one, depth, "38", 30, 90)
}

// backgroundOf is the escape sequence that paints the ground in one colour at
// one depth.
func backgroundOf(one tone, depth colorDepth) string {
	return colorCode(one, depth, "48", 40, 100)
}

// colorCode writes one colour as the escape sequence for the depth in hand. The
// first number tells the terminal whether this is the letters or the ground, and
// the last two are where the sixteen ordinary colours and the eight bright ones
// begin for that half.
func colorCode(one tone, depth colorDepth, half string, ordinary int, bright int) string {
	switch depth {
	case depthTruecolor:
		red, green, blue := one.redGreenBlue()
		return "\x1b[" + half + ";2;" + strconv.Itoa(red) + ";" + strconv.Itoa(green) + ";" + strconv.Itoa(blue) + "m"
	case depth256:
		return "\x1b[" + half + ";5;" + strconv.Itoa(one.nearest256()) + "m"
	case depthSixteen:
		if one.basic >= 8 {
			return "\x1b[" + strconv.Itoa(bright+one.basic-8) + "m"
		}
		return "\x1b[" + strconv.Itoa(ordinary+one.basic) + "m"
	default:
		return ""
	}
}
