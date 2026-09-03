package tui

import "strings"

// maxTranscriptBlocks is how many blocks the transcript keeps. The oldest is
// dropped when a new one arrives past the cap, because a screen that keeps every
// block of a day-long task eventually fills the machine's memory.
const maxTranscriptBlocks = 500

// maxBlockRunes is the most text one block keeps. A reply longer than this keeps
// its end, because the newest text is the part being read and the whole of it is
// in the program's log.
const maxBlockRunes = 20000

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
	added.text = keepTail(added.text)
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

// keepTail shortens text that is longer than one block may keep, saying that it
// has done so rather than quietly losing the beginning.
func keepTail(text string) string {
	letters := []rune(text)
	if len(letters) <= maxBlockRunes {
		return text
	}
	return "(the earlier part of this is not shown here; it is in the log)\n" +
		string(letters[len(letters)-maxBlockRunes:])
}

// transcriptRows draws the newest blocks, with one blank line between blocks,
// and stops as soon as it has as many rows as were asked for. Drawing from the
// bottom up is what makes a frame cost the same on the thousandth block as on
// the first.
func (screen *Screen) transcriptRows(wanted int) []string {
	gathered := []string{}
	for at := len(screen.blocks) - 1; at >= 0 && len(gathered) < wanted; at-- {
		lines := screen.blockLines(screen.blocks[at])
		if at > 0 && blankBetween(screen.blocks[at-1].kind, screen.blocks[at].kind) {
			lines = append([]string{""}, lines...)
		}
		gathered = append(lines, gathered...)
	}
	return gathered
}

// blankBetween says whether two blocks want a blank line between them. A
// person's message and a card each stand alone with air around them; the agent's
// replies and its tool lines run together, because they are one turn.
func blankBetween(above blockKind, below blockKind) bool {
	return !(partOfATurn(above) && partOfATurn(below))
}

// partOfATurn says whether a block is part of what the agent did in one turn.
func partOfATurn(kind blockKind) bool {
	return kind == blockReply || kind == blockTool
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

// personLines draws a message the person typed: a filled bubble leaning against
// the right-hand edge of the frame, with the thick left edge that the design
// drew as a bar beside the message kept as the bubble's own edge.
func (screen *Screen) personLines(text string) []string {
	lines := []row{}
	for _, wrapped := range wrapText(text, screen.bubbleWidth()-bubbleFrame) {
		line := row{}
		line.add(styleChip, wrapped)
		lines = append(lines, line)
	}
	return screen.bubbleRows(lines, personBubble())
}

// replyLines draws what the agent wrote back: an outlined bubble against the
// left-hand edge, with markdown rendered lightly inside it.
func (screen *Screen) replyLines(text string) []string {
	return screen.bubbleRows(markdownRows(text, screen.bubbleWidth()-bubbleFrame), agentBubble())
}

// toolLines draws one tool call as a small filled pill holding the tool, its
// main argument, and a short summary, and never the result text, which
// "/tasks 17" and "read r3" show on purpose.
func (screen *Screen) toolLines(text string) []string {
	return screen.pillRows(text)
}

// visibleTranscript is the rows of the transcript that fit in the space it has,
// pushed to the bottom, less however far the person has scrolled up. A scroll
// that has run off the top is pulled back here, which is the one place that
// knows how many rows there really are.
func (screen *Screen) visibleTranscript(height int) []string {
	if height < 1 {
		return []string{}
	}
	if len(screen.blocks) == 0 {
		return screen.bannerRows(height)
	}
	gathered := screen.transcriptRows(height + screen.scrollBack)
	screen.scrollBack = min(screen.scrollBack, max(len(gathered)-height, 0))
	end := len(gathered) - screen.scrollBack
	start := max(end-height, 0)
	shown := gathered[start:end]
	return append(make([]string, height-len(shown)), shown...)
}

// scrolledUp says whether the person has scrolled away from the newest content,
// which is what puts the "more" marker in the status strip.
func (screen *Screen) scrolledUp() bool {
	return screen.scrollBack > 0
}

// scrollBy moves the view up or down inside the transcript. It never goes below
// the newest row here, and how far up it may go is found when the frame is next
// drawn.
func (screen *Screen) scrollBy(rows int) {
	screen.scrollBack = max(screen.scrollBack+rows, 0)
}

// ruleRow draws one thin dim line across the frame, which is what separates the
// header and the input box from the transcript.
func (screen *Screen) ruleRow() string {
	line := row{}
	line.blanks(marginColumns)
	line.add(styleDim, strings.Repeat(string(ruleGlyph), screen.width-2*marginColumns))
	return line.render(screen.colors)
}
