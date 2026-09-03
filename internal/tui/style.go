package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The DigiByte palette, written as the lipgloss colours the frame is designed
// in. The ground is DigiByte's blue lightened so that white letters sit on it,
// the accent is DigiByte's own blue, and the last two are kept for the two
// things that must never be missed. Each colour also names the nearest of the
// sixteen colours every terminal has, for the terminals that have no more.
//
// The escape codes are written here rather than by lipgloss.Style.Render
// because a lipgloss renderer reports "no colour at all" whenever the writer it
// was built on is not a terminal, which every test process is; painting through
// it would drop the colours out of exactly the golden files that have to prove
// them. The colour values and the borders are lipgloss's; the depth is ours.
var (
	// groundTone is the light DigiByte blue behind every row of the frame.
	groundTone = tone{color: lipgloss.Color("#1E90FF"), basic: 4}
	// textTone is the white every ordinary word is drawn in.
	textTone = tone{color: lipgloss.Color("#FFFFFF"), basic: 15}
	// dimTone is the pale blue the quiet parts of the frame are drawn in.
	dimTone = tone{color: lipgloss.Color("#CFE6FF"), basic: 7}
	// accentTone is DigiByte's own blue, which fills the pills, the buttons and
	// the person's bubble and draws the health dot.
	accentTone = tone{color: lipgloss.Color("#0066CC"), basic: 12}
	// warnTone is the gold that draws the border of the card the person must
	// answer, because gold on blue is the loudest pairing the palette has.
	warnTone = tone{color: lipgloss.Color("#FFD166"), basic: 11}
	// troubleTone is the red a failure is drawn in.
	troubleTone = tone{color: lipgloss.Color("#FF6B6B"), basic: 9}
)

// The two escape codes that are not colours: bold letters, and the swap of the
// foreground and the background that draws the cursor on a terminal told to use
// no colour at all. resetCode puts the terminal back after every piece.
const (
	boldCode    = "\x1b[1m"
	reverseCode = "\x1b[7m"
	resetCode   = "\x1b[0m"
)

// tone is one colour of the palette: the value lipgloss names it by, and the
// nearest of the sixteen colours a terminal with nothing better can draw.
type tone struct {
	// color is the colour itself, as lipgloss writes it.
	color lipgloss.Color
	// basic is the number of the nearest of the sixteen ordinary colours.
	basic int
}

// redGreenBlue reads a palette colour back as the three numbers a terminal
// wants, and reads as black when the colour was not written as six hex digits.
func (one tone) redGreenBlue() (int, int, int) {
	digits := strings.TrimPrefix(string(one.color), "#")
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
	// styleDim is the pale blue every quiet thing is drawn in: the header, the
	// rules, and the status strip.
	styleDim
	// styleAccent is DigiByte's own blue, which draws the prompt glyph, the
	// health dot, and a running task.
	styleAccent
	// styleError is the red a failure is drawn in.
	styleError
	// styleBold is white letters made bold, which is what light markdown uses
	// for bold text and headings and what fills the budget bar.
	styleBold
	// styleReverse swaps the letters and the ground, which is how the cursor is
	// drawn when it sits inside the text rather than at the end.
	styleReverse
	// styleWarn is the gold that draws the border and the title of the card the
	// person has to answer.
	styleWarn
	// styleChip is white letters on the accent, which is the filled shape the
	// tool pills, the answer buttons and the person's bubble are made of.
	styleChip
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
// the colour of the ground behind them, and bold when the style is a loud one.
func (colors theme) code(chosen style) string {
	if colors.depth == depthNone {
		return ""
	}
	front, back, heavy := paintFor(chosen)
	built := foregroundOf(front, colors.depth) + backgroundOf(back, colors.depth)
	if heavy {
		built += boldCode
	}
	return built
}

// paintFor says which colour draws a style's letters, which colour draws the
// ground behind them, and whether the letters are made bold. Every style but the
// chip is drawn on the DigiByte ground, which is what fills the whole frame.
func paintFor(chosen style) (tone, tone, bool) {
	switch chosen {
	case styleDim:
		return dimTone, groundTone, false
	case styleAccent:
		return accentTone, groundTone, true
	case styleError:
		return troubleTone, groundTone, true
	case styleBold:
		return textTone, groundTone, true
	case styleWarn:
		return warnTone, groundTone, true
	case styleChip:
		return textTone, accentTone, true
	default:
		return textTone, groundTone, false
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
