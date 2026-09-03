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
}

// answersLine is the last line inside the box: the single keys that answer this
// card, in the words docs/TUI_DESIGN.md uses.
func (shown card) answersLine() string {
	switch shown.kind {
	case cardPreview:
		return "[a] approve once   [A] always this session   [r] reject with a reason"
	case cardHandoff:
		return "[a] I have finished   [r] give up on this"
	case cardQuestion:
		return "type your answer below and press Enter"
	default:
		return "[Esc] dismiss"
	}
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

// titleStyle is how the title in the top rule is drawn: accent on a preview,
// because that is the one the person must answer, error on a failure, and dim on
// the rest.
func (shown card) titleStyle() style {
	switch shown.kind {
	case cardPreview:
		return styleAccent
	case cardError:
		return styleError
	default:
		return styleDim
	}
}

// cardLines draws a card as the rows of its box: a single-line frame in the dim
// colour with the title in the top rule and the answers on the last line inside.
func (screen *Screen) cardLines(shown card) []string {
	outer := screen.width - 2*marginColumns
	if outer < 8 {
		outer = 8
	}
	inner := outer - 4
	drawn := []string{screen.cardTopRow(shown, outer)}
	for _, line := range screen.cardBodyLines(shown, inner) {
		drawn = append(drawn, screen.cardBodyRow(line, inner))
	}
	drawn = append(drawn, screen.cardBodyRow("", inner))
	for _, line := range wrapText(shown.answersLine(), inner) {
		drawn = append(drawn, screen.cardBodyRow(line, inner))
	}
	bottom := row{}
	bottom.blanks(marginColumns)
	bottom.add(styleDim, "└"+strings.Repeat(string(ruleGlyph), outer-2)+"┘")
	drawn = append(drawn, bottom.render(screen.colors))

	// A picture is drawn under the box rather than inside it, because the
	// terminal, not this screen, decides how many rows an inline picture takes.
	if picture, canDraw := screen.picture(shown, inner); canDraw {
		drawn = append(drawn, picture)
	}
	return drawn
}

// cardBodyLines is the text inside the box: the body, and, when the terminal
// cannot draw a picture inline, the path of the screenshot instead.
func (screen *Screen) cardBodyLines(shown card, inner int) []string {
	lines := wrapText(shown.body, inner)
	if shown.picture == "" {
		return lines
	}
	if _, canDraw := screen.picture(shown, inner); canDraw {
		return lines
	}
	lines = append(lines, "open this file to see the page:")
	return append(lines, wrapText(shown.picture, inner)...)
}

// picture is the screenshot drawn inline, and says false when there is none or
// the terminal cannot draw one.
func (screen *Screen) picture(shown card, inner int) (string, bool) {
	if shown.picture == "" {
		return "", false
	}
	return screen.pictureLine(shown.picture, inner)
}

// cardTopRow draws the top of the box, with the title sitting in the rule.
func (screen *Screen) cardTopRow(shown card, outer int) string {
	title := cutTo(shown.title, outer-6)
	line := row{}
	line.blanks(marginColumns)
	line.add(styleDim, "┌ ")
	line.add(shown.titleStyle(), title)
	line.add(styleDim, " "+strings.Repeat(string(ruleGlyph), outer-4-displayWidth(title))+"┐")
	return line.render(screen.colors)
}

// cardBodyRow draws one line inside the box, padded so that the right-hand edge
// of the frame stays straight.
func (screen *Screen) cardBodyRow(text string, inner int) string {
	line := row{}
	line.blanks(marginColumns)
	line.add(styleDim, "│ ")
	line.add(styleNormal, text)
	line.padTo(marginColumns + 2 + inner)
	line.add(styleDim, " │")
	return line.render(screen.colors)
}
