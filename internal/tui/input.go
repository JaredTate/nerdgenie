package tui

import "strings"

// The bounds on what a person can type. A message longer than the cap is a
// mistake or a runaway paste, and either way the screen refuses the rest rather
// than growing without end.
const (
	// maxInputRunes is the most one message may hold.
	maxInputRunes = 8000
	// maxInputRows is the tallest the input box grows before it scrolls inside.
	maxInputRows = 5
	// maxHistoryEntries is how many past messages Up walks back through.
	maxHistoryEntries = 100
)

// editor is the input box's own state: the letters typed so far, where the
// cursor is among them, and whether what is typed is hidden.
type editor struct {
	// letters is what has been typed, as characters rather than bytes.
	letters []rune
	// cursor is where the next character goes, from zero to the length.
	cursor int
	// secret is true while the program is asking for a password, and then
	// nothing typed is ever drawn, kept, or logged.
	secret bool
}

// text is what has been typed, as a string.
func (typed editor) text() string {
	return string(typed.letters)
}

// setText replaces everything typed and puts the cursor at the end.
func (typed *editor) setText(text string) {
	typed.letters = []rune(text)
	if len(typed.letters) > maxInputRunes {
		typed.letters = typed.letters[:maxInputRunes]
	}
	typed.cursor = len(typed.letters)
}

// clear empties the box and puts the cursor back at the start.
func (typed *editor) clear() {
	typed.letters = nil
	typed.cursor = 0
}

// insert puts text in at the cursor, and refuses whatever would take the message
// past the cap.
func (typed *editor) insert(text string) {
	added := []rune(text)
	room := maxInputRunes - len(typed.letters)
	if room <= 0 {
		return
	}
	if len(added) > room {
		added = added[:room]
	}
	rest := append([]rune{}, typed.letters[typed.cursor:]...)
	typed.letters = append(append(typed.letters[:typed.cursor:typed.cursor], added...), rest...)
	typed.cursor += len(added)
}

// backspace removes the character before the cursor.
func (typed *editor) backspace() {
	if typed.cursor == 0 {
		return
	}
	typed.letters = append(typed.letters[:typed.cursor-1], typed.letters[typed.cursor:]...)
	typed.cursor--
}

// moveBy walks the cursor left or right, and never off either end.
func (typed *editor) moveBy(places int) {
	typed.cursor += places
	if typed.cursor < 0 {
		typed.cursor = 0
	}
	if typed.cursor > len(typed.letters) {
		typed.cursor = len(typed.letters)
	}
}

// shownLetters is what the input box draws: the letters themselves, or one
// bullet for each of them while the program is asking for a secret. It is always
// a fresh slice, so that nothing the box draws can reach back into what was
// typed.
func (typed editor) shownLetters() []rune {
	shown := make([]rune, len(typed.letters))
	for at := range shown {
		shown[at] = typed.letters[at]
		if typed.secret {
			shown[at] = maskGlyph
		}
	}
	return shown
}

// inputRows draws the rows under the second rule: the dim title above the
// input box when the program has asked for something in particular, the box
// itself, and the footer's row of key hints under it, which the status strip
// then follows as the frame's last row.
func (screen *Screen) inputRows() []string {
	drawn := []string{}
	for _, line := range wrapText(screen.inputTitle(), screen.width-2*marginColumns) {
		if line == "" {
			continue
		}
		above := row{}
		above.blanks(marginColumns)
		above.add(styleDim, line)
		drawn = append(drawn, above.render(screen.colors))
	}
	drawn = append(drawn, screen.editorRows()...)
	return append(drawn, screen.hintsRow())
}

// inputTitle is the dim line above the box: what secret the program is asking
// for, or the question that comes with rejecting a preview.
func (screen *Screen) inputTitle() string {
	switch {
	case screen.input.secret:
		return screen.secretPrompt
	case screen.askingWhyNot:
		return "Why not? Type a reason and press Enter."
	default:
		return ""
	}
}

// promptText is the glyph at the left of the box: the accent chevron normally,
// and a padlock while a secret is being typed, or a star where the terminal
// cannot draw one.
func (screen *Screen) promptText() string {
	if !screen.input.secret {
		return string(promptGlyph)
	}
	if screen.canDrawEmoji {
		return string(lockGlyph)
	}
	return "*"
}

// editorRows draws the box itself: the prompt glyph, then the text, breaking at
// the right-hand edge and growing to five rows before it scrolls inside.
func (screen *Screen) editorRows() []string {
	promptWidth := displayWidth(screen.promptText())
	textWidth := screen.width - 2*marginColumns - promptWidth - 1
	if textWidth < 1 {
		textWidth = 1
	}

	letters := screen.input.shownLetters()
	cursorAt := screen.input.cursor
	if cursorAt >= len(letters) {
		// The cursor sits past the last character, so it is drawn as the
		// underscore docs/TUI_DESIGN.md shows rather than over a character.
		letters = append(letters, '_')
		cursorAt = -1
	}
	pieces := chunkRunes(letters, textWidth)
	first := max(len(pieces)-maxInputRows, 0)

	drawn := []string{}
	start := 0
	for number, piece := range pieces {
		if number >= first {
			drawn = append(drawn, screen.editorRow(number == first, piece, cursorAt-start, promptWidth))
		}
		start += len(piece)
	}
	return drawn
}

// editorRow draws one row of the box, with the cursor on it when the cursor
// falls inside this row.
func (screen *Screen) editorRow(withPrompt bool, piece []rune, cursorAt int, promptWidth int) string {
	line := row{}
	line.blanks(marginColumns)
	if withPrompt {
		line.add(styleAccent, screen.promptText())
		line.blanks(1)
	} else {
		line.blanks(promptWidth + 1)
	}
	if cursorAt >= 0 && cursorAt < len(piece) {
		line.add(screen.inputStyle(), string(piece[:cursorAt]))
		line.add(styleReverse, string(piece[cursorAt]))
		line.add(screen.inputStyle(), string(piece[cursorAt+1:]))
		return line.render(screen.colors)
	}
	line.add(screen.inputStyle(), string(piece))
	return line.render(screen.colors)
}

// inputStyle draws the box dim while a card is waiting for an answer, because
// the card has the keys until it is answered.
func (screen *Screen) inputStyle() style {
	if screen.focusedCard() != nil {
		return styleDim
	}
	return styleNormal
}

// chunkRunes breaks characters into pieces that each fit in the width, and
// always returns at least one piece so that an empty box still has a row.
func chunkRunes(letters []rune, width int) [][]rune {
	pieces := [][]rune{}
	rest := letters
	for len(rest) > 0 {
		end := hardFit(rest, width)
		pieces = append(pieces, rest[:end])
		rest = rest[end:]
	}
	if len(pieces) == 0 {
		return [][]rune{{}}
	}
	return pieces
}

// canDrawEmoji says whether the terminal's language settings promise that it can
// draw an emoji, which is what decides between the padlock and the star.
func canDrawEmoji(environment func(string) string) bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		setting := strings.ToLower(environment(name))
		if setting == "" {
			continue
		}
		return strings.Contains(setting, "utf-8") || strings.Contains(setting, "utf8")
	}
	return false
}
