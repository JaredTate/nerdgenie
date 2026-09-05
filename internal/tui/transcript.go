package tui

import (
	"strconv"
	"strings"
)

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
	// repeats is how many times in a row the same call was made, or how many
	// times the same record line came back, when the kind is a tool line. Zero
	// and one both mean once.
	repeats int
	// fromRecord says the pill holds a record line, one line about the newest
	// change to the record, rather than a tool call. It is drawn the same, and
	// it is what a record line that comes back is checked against.
	fromRecord bool
	// task is the number of the task that was running when a tool pill was
	// drawn, so that its result can still be asked for by name after the task
	// has ended and the status no longer names one.
	task string
}

// remember puts one block on the end of the transcript and drops the oldest when
// the transcript is full. A view the person has scrolled up is held where it is,
// so that a reply arriving does not pull them away from what they were reading.
func (screen *Screen) remember(added block) {
	added.text = keepTail(added.text)
	if screen.scrolledUp() {
		screen.scrollBack += screen.rowsAddedBy(added)
	}
	screen.blocks = append(screen.blocks, added)
	if len(screen.blocks) > maxTranscriptBlocks {
		screen.blocks = screen.blocks[len(screen.blocks)-maxTranscriptBlocks:]
	}
}

// transcriptWidth is how many columns the text of a block wraps at: the frame
// less its margins and the two-column gutter, and never more than the widest a
// line is comfortable to read.
func (screen *Screen) transcriptWidth() int {
	width := screen.transcriptColumns() - 2*marginColumns - gutterColumns
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
// and stops as soon as it has as many rows as were asked for. A block that would
// fit in a transcript of its own but not in the room left at the top is not
// drawn at all, because a bubble cut in two is worse than a bubble not shown:
// the frame is left holding a lid with no box under it, which is what the second
// trial saw. A block taller than the whole transcript is drawn anyway and cut,
// because there is no room for it whole anywhere and its newest rows are the
// ones being read. Drawing from the bottom up is what makes a frame cost the
// same on the thousandth block as on the first.
func (screen *Screen) transcriptRows(wanted int) []string {
	rows, _ := screen.newestRows(wanted, true, noFocus)
	return rows
}

// newestRows gathers rows from the newest block backwards until it has as many
// as were asked for, and beside them which block each row belongs to, or minus
// one for the blank between two blocks. With wholeAtTheTop set it keeps the
// rule above, leaving out a block that would fit whole but not in the room
// left; without it the block is gathered and cut, which is what a view scrolled
// up by rows needs, because a view moving three rows at a time has to cross
// every block on its way. The pill at focused is drawn as the focused one.
func (screen *Screen) newestRows(wanted int, wholeAtTheTop bool, focused int) ([]string, []int) {
	gathered := []string{}
	owners := []int{}
	for at := len(screen.blocks) - 1; at >= 0 && len(gathered) < wanted; at-- {
		lines := screen.blockLinesAt(at, at == focused)
		belong := belongingTo(at, len(lines))
		if at > 0 && blankBetween(screen.blocks[at-1].kind, screen.blocks[at].kind) {
			lines = append([]string{""}, lines...)
			belong = append([]int{-1}, belong...)
		}
		if wholeAtTheTop && len(gathered)+len(lines) > wanted && len(lines) <= wanted {
			break
		}
		gathered = append(lines, gathered...)
		owners = append(belong, owners...)
	}
	return gathered, owners
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

// blockLinesAt draws the block at one place in the transcript, which is where
// a pill learns whether it is the focused one.
func (screen *Screen) blockLinesAt(at int, focused bool) []string {
	item := screen.blocks[at]
	if item.kind == blockTool {
		return screen.toolLines(item, focused)
	}
	return screen.blockLines(item)
}

// blockLines draws one block as the rows it takes up, with no focus on it.
func (screen *Screen) blockLines(item block) []string {
	switch item.kind {
	case blockPerson:
		return screen.personLines(item.text)
	case blockTool:
		return screen.toolLines(item, false)
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
// "/tasks 17" and "read r3" show on purpose. A call that was made again and
// again in a row is one pill with a count on the end, such as "× 13", because
// thirteen rows saying one thing tell the person less than one row that says
// how many times. The focused pill is drawn apart from the rest, and a pill
// that has been opened draws its text under itself.
func (screen *Screen) toolLines(item block, focused bool) []string {
	text := item.text
	if item.repeats > 1 {
		text += " " + string(repeatGlyph) + " " + strconv.Itoa(item.repeats)
	}
	return append(screen.pillRows(text, focused), screen.expansionLines(item)...)
}

// visibleTranscript is the rows of the transcript that fit in the space it has,
// pushed to the bottom, less however far the person has scrolled up. A scroll
// that has run off the top is pulled back here, which is the one place that
// knows how many rows there really are. The rule that a block is drawn whole or
// not at all holds only for the view resting on the newest row; a view scrolled
// up is a window moved by rows, and the blocks at its edges are cut. Beside
// the rows comes which block each one belongs to, or minus one, which is what
// a click on a row is looked up in; the pill at focused is drawn focused.
func (screen *Screen) visibleTranscript(height int, focused int) ([]string, []int) {
	if height < 1 {
		return []string{}, []int{}
	}
	if len(screen.blocks) == 0 {
		return screen.welcomeRows(height), nobodys(height)
	}
	gathered, owners := screen.newestRows(height+screen.scrollBack, !screen.scrolledUp(), focused)
	screen.scrollBack = min(screen.scrollBack, max(len(gathered)-height, 0))
	end := len(gathered) - screen.scrollBack
	start := max(end-height, 0)
	shown := gathered[start:end]
	padding := height - len(shown)
	return append(make([]string, padding), shown...), append(nobodys(padding), owners[start:end]...)
}

// ruleRow draws one thin dim line across the frame, which is what separates the
// header and the input box from the transcript.
func (screen *Screen) ruleRow() string {
	line := row{}
	line.blanks(marginColumns)
	line.add(styleDim, strings.Repeat(string(ruleGlyph), screen.width-2*marginColumns))
	return line.render(screen.colors)
}
