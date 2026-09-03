// The three answers on a preview card, and the idea of turning a permission
// question into a small focused prompt with single-key answers, are ported from
// OpenCode's approval prompt at
// ~/Code/opencode/packages/tui/src/routes/session/permission.tsx. The code here
// is written fresh in Go; nothing was copied.

package tui

import "strings"

// cardKind is one of the four things that need the person, each with its own
// title in the top rule of the box.
type cardKind int

const (
	// cardPreview shows exactly what is about to happen and waits for an answer.
	cardPreview cardKind = iota
	// cardQuestion is the model asking something in plain words.
	cardQuestion
	// cardHandoff gives the browser or the desktop back to the person.
	cardHandoff
	// cardError says something went wrong and what to do about it.
	cardError
)

// The titles docs/TUI_DESIGN.md gives the four cards.
const (
	previewTitle  = "Ask me first"
	questionTitle = "Question"
	handoffTitle  = "Your turn in the browser"
	errorTitle    = "Something went wrong"
)

// card is one thing the screen is waiting on the person for.
type card struct {
	// kind says which of the four this is.
	kind cardKind
	// id is what an answer names, so that the answer the screen sends and the
	// request the program made point at the same thing.
	id string
	// title is the line in the top rule of the box.
	title string
	// body is the exact text or command that is about to run.
	body string
	// picture is the path of the screenshot on a handoff, or empty.
	picture string
	// answered is what the person said, once they have said it, which is what
	// stops a card that has been dealt with from taking the keys again.
	answered bool
	// number is this card's own count on the screen it was shown on, which is
	// how the screen finds the card that holds the single keys whatever else has
	// arrived above it since.
	number int
	// drawn is the screenshot already turned into the escape sequence this
	// terminal draws a picture with, encoded once when the card was made rather
	// than again on every frame.
	drawn string
}

// answerButton is one single-key answer drawn as a small button: the key itself
// in a filled shape, and the words that say what pressing it does.
type answerButton struct {
	// key is the single key the person presses.
	key string
	// words say what that key does, in the words docs/TUI_DESIGN.md uses.
	words string
}

// answerButtons are the single keys that answer this card. A question is
// answered in the input box and so has none.
func (shown card) answerButtons() []answerButton {
	switch shown.kind {
	case cardPreview:
		return []answerButton{
			{key: "a", words: "approve once"},
			{key: "A", words: "always this session"},
			{key: "r", words: "reject with a reason"},
		}
	case cardHandoff:
		return []answerButton{{key: "a", words: "I have finished"}, {key: "r", words: "give up on this"}}
	case cardQuestion:
		return nil
	default:
		return []answerButton{{key: "Esc", words: "dismiss"}}
	}
}

// buttonWidth is how many columns one button and its words take, with the two
// blanks that separate it from the button after it.
func (one answerButton) buttonWidth() int {
	return displayWidth(one.key) + displayWidth(one.words) + 7
}

// answerRows draws the buttons at the foot of a card: all on one row when they
// fit, and one to a row when they do not, so that a key is never split from the
// words that say what it does.
func (screen *Screen) answerRows(shown card, inner int) []row {
	buttons := shown.answerButtons()
	if len(buttons) == 0 {
		return wrapSpans([]span{{style: styleDim, text: "type your answer below and press Enter"}}, inner)
	}
	needed := 0
	for _, one := range buttons {
		needed += one.buttonWidth()
	}
	if needed-2 <= inner {
		return []row{cutRow(buttonsInARow(buttons), inner)}
	}
	drawn := []row{}
	for _, one := range buttons {
		drawn = append(drawn, cutRow(buttonsInARow([]answerButton{one}), inner))
	}
	return drawn
}

// cutRow shortens a row so that it fits inside a box, which is what a terminal
// too narrow for the words on a button gets.
func cutRow(line row, inner int) row {
	line.keepWithin(inner)
	return line
}

// buttonsInARow draws one row of buttons, each key in a filled shape with its
// words beside it.
func buttonsInARow(buttons []answerButton) row {
	line := row{}
	for at, one := range buttons {
		if at > 0 {
			line.blanks(2)
		}
		line.add(styleChip, "[ "+one.key+" ]")
		line.add(styleNormal, " "+one.words)
	}
	return line
}

// takesKeys says whether this card holds the single keys. A preview and a
// handoff do until they are answered; a question is answered in the input box,
// and an error needs no answer at all.
func (shown card) takesKeys() bool {
	if shown.answered {
		return false
	}
	return shown.kind == cardPreview || shown.kind == cardHandoff
}

// titleStyle is how the border and the title of a card are drawn: the bright
// gold of the palette on a preview, because that is the one card the person must
// answer and gold on the blue ground is the loudest pairing there is; the error
// colour on a failure; and dim on the rest.
func (shown card) titleStyle() style {
	switch shown.kind {
	case cardPreview:
		return styleWarn
	case cardError:
		return styleError
	default:
		return styleDim
	}
}

// cardLines draws a card as the rows of its box: a single-line frame in the dim
// colour with the title in the top rule and the answers on the last line inside.
func (screen *Screen) cardLines(shown card) []string {
	outer := screen.transcriptColumns() - 2*marginColumns
	if outer < 8 {
		outer = 8
	}
	inner := outer - 4
	drawn := []string{screen.cardTopRow(shown, outer)}
	for _, line := range screen.cardBodyLines(shown, inner) {
		drawn = append(drawn, screen.cardBodyRow(spansOf(styleNormal, line), inner, shown))
	}
	drawn = append(drawn, screen.cardBodyRow(nil, inner, shown))
	for _, line := range screen.answerRows(shown, inner) {
		drawn = append(drawn, screen.cardBodyRow(line.spans, inner, shown))
	}
	bottom := row{}
	bottom.blanks(marginColumns)
	bottom.add(shown.titleStyle(), "└"+strings.Repeat(string(ruleGlyph), outer-2)+"┘")
	drawn = append(drawn, bottom.render(screen.colors))

	// A picture is drawn under the box rather than inside it, because the
	// terminal, not this screen, decides how many rows an inline picture takes.
	if shown.drawn != "" {
		drawn = append(drawn, shown.drawn)
	}
	return drawn
}

// cardBodyLines is the text inside the box: the body, and, when the terminal
// cannot draw a picture inline, the path of the screenshot instead.
func (screen *Screen) cardBodyLines(shown card, inner int) []string {
	lines := wrapText(shown.body, inner)
	if shown.picture == "" || shown.drawn != "" {
		return lines
	}
	lines = append(lines, "open this file to see the page:")
	return append(lines, wrapText(shown.picture, inner)...)
}

// picture is the screenshot drawn inline, and says false when there is none or
// the terminal cannot draw one. The bytes were read and encoded when the card
// was made, so a card sitting on the screen costs nothing to draw again.

// cardTopRow draws the top of the box, with the title sitting in the rule.
func (screen *Screen) cardTopRow(shown card, outer int) string {
	title := cutTo(shown.title, outer-6)
	line := row{}
	line.blanks(marginColumns)
	line.add(shown.titleStyle(), "┌ ")
	line.add(shown.titleStyle(), title)
	line.add(shown.titleStyle(), " "+strings.Repeat(string(ruleGlyph), outer-4-displayWidth(title))+"┐")
	return line.render(screen.colors)
}

// cardBodyRow draws one line inside the box, padded so that the right-hand edge
// of the frame stays straight.
func (screen *Screen) cardBodyRow(pieces []span, inner int, shown card) string {
	line := row{}
	line.blanks(marginColumns)
	line.add(shown.titleStyle(), "│ ")
	for _, piece := range pieces {
		line.addSpan(piece)
	}
	line.padTo(marginColumns + 2 + inner)
	line.add(shown.titleStyle(), " │")
	return line.render(screen.colors)
}

// spansOf is one piece of plain text as the one styled run a card body row draws
// it as.
func spansOf(chosen style, text string) []span {
	if text == "" {
		return nil
	}
	return []span{{style: chosen, text: text}}
}
