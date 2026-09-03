package tui

import "strings"

// The shape of the frame, in columns. docs/TUI_DESIGN.md draws it at eighty
// columns with one blank column down each side, so the header, the rules, and
// the status strip run from column two to the column before the last.
const (
	// marginColumns is the blank column down each side of the frame.
	marginColumns = 1
	// gutterColumns is the two columns the accent bar and the tool arrow sit in,
	// which is also how far a reply is indented.
	gutterColumns = 2
	// narrowWidth is the width below which the right side of the header and the
	// key hints are dropped rather than wrapped.
	narrowWidth = 60
	// widestTranscript is the widest the transcript ever wraps, however wide the
	// terminal is, because a very long line is hard to read.
	widestTranscript = 100
	// minimumGap is the smallest run of blanks allowed between the left-hand
	// words of a one-line row and the piece sitting on its right-hand end, so
	// that the two never read as one long word.
	minimumGap = 2
	// smallestWidth and smallestHeight are the smallest frame the screen draws.
	// A terminal smaller than this gets this frame and clips it, which is better
	// than arithmetic that goes negative.
	smallestWidth  = 24
	smallestHeight = 8
)

// The glyphs the frame is drawn with. They are all one column wide except the
// padlock, which is an emoji and takes two.
const (
	ruleGlyph      = '─'
	personBarGlyph = '▎'
	toolArrowGlyph = '▸'
	promptGlyph    = '›'
	lockGlyph      = '🔒'
	maskGlyph      = '•'
	moreGlyph      = '▼'
	filledDotGlyph = '●'
	hollowDotGlyph = '○'
	barFullGlyph   = '▰'
	barEmptyGlyph  = '▱'
)

// span is one run of text drawn in one style. A row is built out of spans, so
// that its width is worked out from the letters alone and the escape codes are
// added only at the very end.
type span struct {
	// style is how this run of text is drawn.
	style style
	// text is the letters themselves, with no escape codes in them.
	text string
}

// row is one line of the frame while it is being built.
type row struct {
	// spans are the runs of text, left to right.
	spans []span
	// width is how many columns those runs take up so far.
	width int
}

// add puts one piece of text on the end of the row.
func (line *row) add(chosen style, text string) {
	if text == "" {
		return
	}
	line.spans = append(line.spans, span{style: chosen, text: text})
	line.width += displayWidth(text)
}

// addSpan puts an already-styled piece of text on the end of the row.
func (line *row) addSpan(piece span) {
	line.add(piece.style, piece.text)
}

// blanks puts a run of spaces on the end of the row, drawn on the plain ground.
func (line *row) blanks(columns int) {
	line.padWith(styleNormal, columns)
}

// padWith puts a run of spaces on the end of the row drawn in one style, so that
// a filled shape such as a pill or a bubble is filled to its own edge rather
// than showing the ground through its padding.
func (line *row) padWith(chosen style, columns int) {
	if columns > 0 {
		line.add(chosen, strings.Repeat(" ", columns))
	}
}

// padTo widens the row with blanks until it is the given number of columns.
func (line *row) padTo(columns int) {
	line.blanks(columns - line.width)
}

// addRightPiece puts one piece on the right-hand end of a row, with a real gap
// in front of it, and drops it altogether when there is not room for both the
// piece and the gap. docs/TUI_DESIGN.md says a narrow terminal degrades by
// dropping the right-hand side, and a piece glued onto the words beside it reads
// as one long word and is then cut in half by the edge of the terminal.
func (line *row) addRightPiece(piece span, width int) {
	room := width - marginColumns - displayWidth(piece.text)
	if room < line.width+minimumGap {
		return
	}
	line.padTo(room)
	line.addSpan(piece)
}

// keepWithin shortens the row so that it fits in a number of columns, dropping
// whole pieces from the right and cutting the last one that still fits. The
// header and the status strip are one line each and are cut rather than wrapped,
// which is how docs/TUI_DESIGN.md says a narrow terminal degrades.
func (line *row) keepWithin(columns int) {
	if line.width <= columns {
		return
	}
	kept := []span{}
	used := 0
	for _, piece := range line.spans {
		room := columns - used
		if room <= 0 {
			break
		}
		shortened := cutTo(piece.text, room)
		if shortened == "" {
			break
		}
		kept = append(kept, span{style: piece.style, text: shortened})
		used += displayWidth(shortened)
	}
	line.spans = kept
	line.width = used
}

// render turns the row into the string the terminal draws. Blanks on the end are
// dropped, because they are invisible and only make the frame bigger.
func (line row) render(colors theme) string {
	built := strings.Builder{}
	for _, piece := range trimTrailingBlanks(line.spans) {
		built.WriteString(colors.wrap(piece.style, piece.text))
	}
	return built.String()
}

// trimTrailingBlanks drops the spaces on the end of a row, and the spans that
// were nothing but spaces. Only blanks on the plain ground are dropped: a blank
// drawn in any other style is paint, such as the right-hand end of a filled
// pill, and dropping it would leave the shape open.
func trimTrailingBlanks(spans []span) []span {
	kept := make([]span, len(spans))
	copy(kept, spans)
	for len(kept) > 0 {
		last := len(kept) - 1
		if kept[last].style != styleNormal {
			break
		}
		trimmed := strings.TrimRight(kept[last].text, " ")
		if trimmed != "" {
			kept[last].text = trimmed
			break
		}
		kept = kept[:last]
	}
	return kept
}

// displayWidth is how many columns a piece of text takes on the screen. Almost
// every glyph is one column; a combining mark is none, an emoji or a Chinese,
// Japanese, or Korean character is two, and an escape sequence, such as a colour
// or a picture, is none at all because the terminal eats it rather than drawing
// it.
func displayWidth(text string) int {
	width := 0
	letters := []rune(text)
	for at := 0; at < len(letters); at++ {
		if letters[at] == escapeStart {
			at = endOfEscape(letters, at)
			continue
		}
		width += runeWidth(letters[at])
	}
	return width
}

// escapeStart is the character every escape sequence begins with.
const escapeStart = '\x1b'

// endOfEscape returns the position of the last character of the escape sequence
// beginning at a position, so that the loop above can step past the whole of it.
// The three shapes that matter here are the colour codes, which end at a letter,
// and the picture protocols, which end at a bell or at a string terminator.
func endOfEscape(letters []rune, start int) int {
	if start+1 >= len(letters) {
		return start
	}
	switch letters[start+1] {
	case '[':
		for at := start + 2; at < len(letters); at++ {
			if letters[at] >= '@' && letters[at] <= '~' {
				return at
			}
		}
	case ']', '_', 'P', '^':
		for at := start + 2; at < len(letters); at++ {
			if letters[at] == '\a' {
				return at
			}
			if letters[at] == escapeStart && at+1 < len(letters) && letters[at+1] == '\\' {
				return at + 1
			}
		}
	}
	return len(letters) - 1
}

// runeWidth is how many columns one character takes.
func runeWidth(letter rune) int {
	switch {
	case inAnyRange(letter, zeroWidthRanges):
		return 0
	case inAnyRange(letter, wideRanges):
		return 2
	default:
		return 1
	}
}

// zeroWidthRanges are the characters that take no room of their own: the
// combining marks, the direction markers, and the variation selectors.
var zeroWidthRanges = [][2]rune{
	{0x0300, 0x036F},
	{0x200B, 0x200F},
	{0xFE00, 0xFE0F},
}

// wideRanges are the blocks of characters a terminal draws two columns wide: the
// Korean jamo, the Chinese, Japanese, and Korean blocks, the full-width forms,
// and the emoji.
var wideRanges = [][2]rune{
	{0x1100, 0x115F},
	{0x2E80, 0xA4CF},
	{0xAC00, 0xD7A3},
	{0xF900, 0xFAFF},
	{0xFE30, 0xFE6F},
	{0xFF00, 0xFF60},
	{0xFFE0, 0xFFE6},
	{0x1F300, 0x1F9FF},
	{0x20000, 0x3FFFD},
}

// inAnyRange says whether a character falls inside one of the blocks given.
func inAnyRange(letter rune, ranges [][2]rune) bool {
	for _, block := range ranges {
		if letter >= block[0] && letter <= block[1] {
			return true
		}
	}
	return false
}

// wrapText breaks text into lines no wider than the width, at a space where it
// can and inside a word when the word is longer than a whole line. Newlines in
// the text are kept, because a reply written in paragraphs is read in
// paragraphs.
func wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	lines := []string{}
	for _, paragraph := range strings.Split(text, "\n") {
		lines = append(lines, wrapOneLine(paragraph, width)...)
	}
	return lines
}

// wrapOneLine breaks a single line, which holds no newlines of its own.
func wrapOneLine(line string, width int) []string {
	letters := []rune(strings.ReplaceAll(line, "\t", "    "))
	wrapped := []string{}
	start := 0
	for start < len(letters) {
		end := lastFitting(letters, start, width)
		wrapped = append(wrapped, strings.TrimRight(string(letters[start:end]), " "))
		start = end
		for start < len(letters) && letters[start] == ' ' {
			start++
		}
	}
	if len(wrapped) == 0 {
		return []string{""}
	}
	return wrapped
}

// lastFitting returns the position to break at: the end of the text when the
// rest of it fits, the space nearest the right edge when there is one, and the
// right edge itself when a single word fills the whole line.
func lastFitting(letters []rune, start int, width int) int {
	used := 0
	lastSpace := -1
	for at := start; at < len(letters); at++ {
		next := used + runeWidth(letters[at])
		if next > width && at > start {
			if lastSpace > start {
				return lastSpace
			}
			return at
		}
		if letters[at] == ' ' {
			lastSpace = at
		}
		used = next
	}
	return len(letters)
}
