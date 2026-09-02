package tui

import "strings"

// maxTranscriptBlocks is how many blocks the transcript keeps. The oldest is
// dropped when a new one arrives past the cap, because a screen that keeps every
// block of a day-long task eventually fills the machine's memory.
const maxTranscriptBlocks = 500

// blockKind is one of the four things docs/TUI_DESIGN.md draws in the
// transcript.
type blockKind int

const (
	// blockPerson is a message the person typed, drawn with an accent bar.
	blockPerson blockKind = iota
	// blockReply is what the agent wrote back, indented and plain.
	blockReply
	// blockTool is one dim line for one tool call, and never its result text.
	blockTool
	// blockCard is one of the four things that need the person.
	blockCard
)

// block is one thing in the transcript.
type block struct {
	// kind says which of the four this is.
	kind blockKind
	// text is the message, the reply, or the tool line.
	text string
	// shown is the card, when the kind is a card.
	shown card
}

// remember puts one block on the end of the transcript and drops the oldest when
// the transcript is full.
func (screen *Screen) remember(added block) {
	screen.blocks = append(screen.blocks, added)
	if len(screen.blocks) > maxTranscriptBlocks {
		screen.blocks = screen.blocks[len(screen.blocks)-maxTranscriptBlocks:]
	}
	screen.scrollBack = 0
}

// transcriptWidth is how many columns the text of a block wraps at: the frame
// less its margins and the two-column gutter, and never more than the widest a
// line is comfortable to read.
func (screen *Screen) transcriptWidth() int {
	width := screen.width - 2*marginColumns - gutterColumns
	if width > widestTranscript {
		width = widestTranscript
	}
	if width < 1 {
		width = 1
	}
	return width
}

// transcriptLines draws every block, newest last, with one blank line between
// blocks, which is the whole of the transcript before it is cut to the rows that
// fit.
func (screen *Screen) transcriptLines() []string {
	lines := []string{}
	for number, item := range screen.blocks {
		if number > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, screen.blockLines(item)...)
	}
	return lines
}

// blockLines draws one block as the rows it takes up.
func (screen *Screen) blockLines(item block) []string {
	switch item.kind {
	case blockPerson:
		return screen.personLines(item.text)
	case blockTool:
		return screen.toolLines(item.text)
	case blockCard:
		return screen.cardLines(item.shown)
	case blockReply:
		return screen.replyLines(item.text)
	}
	return nil
}

// personLines draws a message the person typed, with the two-column accent bar
// down its left side.
func (screen *Screen) personLines(text string) []string {
	drawn := []string{}
	for _, wrapped := range wrapText(text, screen.transcriptWidth()) {
		line := row{}
		line.blanks(marginColumns)
		line.add(styleAccent, string(personBarGlyph))
		line.add(styleNormal, " "+wrapped)
		drawn = append(drawn, line.render(screen.colors))
	}
	return drawn
}

// replyLines draws what the agent wrote back: indented two columns, no bar, and
// markdown rendered lightly.
func (screen *Screen) replyLines(text string) []string {
	drawn := []string{}
	for _, line := range markdownRows(text, screen.transcriptWidth()) {
		full := row{}
		full.blanks(marginColumns + gutterColumns)
		for _, piece := range line.spans {
			full.addSpan(piece)
		}
		drawn = append(drawn, full.render(screen.colors))
	}
	return drawn
}

// toolLines draws one tool call as one dim line under the arrow. The line holds
// the tool, its main argument, and a short summary, and never the result text,
// which "/tasks 17" and "read r3" show on purpose.
func (screen *Screen) toolLines(text string) []string {
	drawn := []string{}
	for number, wrapped := range wrapText(text, screen.transcriptWidth()-2) {
		line := row{}
		line.blanks(marginColumns + gutterColumns)
		if number == 0 {
			line.add(styleDim, string(toolArrowGlyph)+" ")
		} else {
			line.blanks(2)
		}
		line.add(styleDim, wrapped)
		drawn = append(drawn, line.render(screen.colors))
	}
	return drawn
}

// visibleTranscript is the rows of the transcript that fit in the space it has,
// pushed to the bottom, less however far the person has scrolled up.
func (screen *Screen) visibleTranscript(height int) []string {
	if height < 1 {
		return []string{}
	}
	all := screen.transcriptLines()
	end := len(all) - screen.scrollBack
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	shown := all[start:end]
	blank := make([]string, height-len(shown))
	return append(blank, shown...)
}

// scrolledUp says whether the person has scrolled away from the newest content,
// which is what puts the "more" marker in the status strip.
func (screen *Screen) scrolledUp() bool {
	return screen.scrollBack > 0
}

// scrollBy moves the view up or down inside the transcript, and never past
// either end of it.
func (screen *Screen) scrollBy(rows int) {
	furthest := len(screen.transcriptLines())
	screen.scrollBack += rows
	if screen.scrollBack < 0 {
		screen.scrollBack = 0
	}
	if screen.scrollBack > furthest {
		screen.scrollBack = furthest
	}
}

// ruleRow draws one thin dim line across the frame, which is what separates the
// header and the input box from the transcript.
func (screen *Screen) ruleRow() string {
	line := row{}
	line.blanks(marginColumns)
	line.add(styleDim, strings.Repeat(string(ruleGlyph), screen.width-2*marginColumns))
	return line.render(screen.colors)
}
