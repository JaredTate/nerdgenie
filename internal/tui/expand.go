package tui

import (
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Opening a tool pill. A pill is one line about a call, and the full text of
// the call and what came back of it lives in the program; Enter on a focused
// pill, or a click on it, asks the program for that text once, over the
// socket, and draws it under the pill until the pill is folded up again.

const (
	// maxShownTexts is how many result texts the screen keeps. Past it the
	// text whose pill is oldest on the transcript is let go, because a
	// day-long task fetches more texts than a screen should hold.
	maxShownTexts = 200
	// maxExpandedLines is the most lines an open pill draws under itself,
	// the last of them saying how many were left off, because a result of
	// ten thousand lines belongs in the log and not on the frame.
	maxExpandedLines = 60
	// pillTextIndent is the column the text under an open pill starts in:
	// two columns past the pill's own left edge.
	pillTextIndent = marginColumns + gutterColumns + 2
)

// The keys a show carries in its fields, which the doc comments on
// contract.SocketShow and contract.SocketShown name: the result's id, the task
// it belongs to, and the job whose record is asked for.
const (
	showFieldID   = "id"
	showFieldTask = "task"
	showFieldJob  = "job"
)

// The two labels the program writes at the head of a result's text, drawn dim
// so that the words after them are the loud part, and the one this screen
// writes when the program answered with an error instead.
var textLabels = []string{"call:", "result:", "error:"}

// togglePillAt opens the pill at one place in the transcript when it is
// folded and folds it when it is open, and does nothing to a block that is not
// a pill or to a pill whose call has not come back yet. A view scrolled up is
// held still across the rows the text adds or takes away, the way it is held
// while a reply streams.
func (screen *Screen) togglePillAt(at int) {
	if at < 0 || at >= len(screen.blocks) || screen.blocks[at].kind != blockTool {
		return
	}
	id := resultIDOf(screen.blocks[at].text)
	if id == "" {
		return
	}
	before := len(screen.blockLines(screen.blocks[at]))
	if screen.expanded[id] {
		delete(screen.expanded, id)
	} else {
		screen.openPill(id)
	}
	if screen.scrolledUp() {
		screen.scrollBack = max(screen.scrollBack+len(screen.blockLines(screen.blocks[at]))-before, 0)
	}
}

// openPill marks one pill open and asks for its text when the screen does not
// have it. The open pills are capped like the texts, letting the oldest on the
// transcript go, so that the map cannot grow past what a person could open.
func (screen *Screen) openPill(id string) {
	if screen.expanded == nil {
		screen.expanded = map[string]bool{}
	}
	if len(screen.expanded) >= maxShownTexts {
		delete(screen.expanded, screen.oldestPillID(func(held string) bool { return screen.expanded[held] }))
	}
	screen.expanded[id] = true
	screen.askToShow(id)
}

// askToShow sends one show for a result the screen has no text for, and
// writes an empty text under its id at once, which is what stops the same id
// being asked for twice while the answer is on its way. A show that cannot be
// sent folds the pill back up and says why in an error card, because a pill
// that said fetching for ever would be a lie.
func (screen *Screen) askToShow(id string) {
	if _, asked := screen.shown[id]; asked {
		return
	}
	screen.rememberShown(id, "")
	fields := map[string]string{showFieldID: id}
	if screen.taskID != "" {
		fields[showFieldTask] = screen.taskID
	}
	if err := screen.link.Send(contract.SocketEnvelope{Type: contract.SocketShow, Fields: fields}); err != nil {
		delete(screen.shown, id)
		delete(screen.expanded, id)
		screen.showTrouble(err.Error())
	}
}

// rememberShown keeps the text the program sent for one result, cut to what a
// block may hold, letting the text whose pill is oldest on the transcript go
// when the screen already holds as many as it keeps.
func (screen *Screen) rememberShown(id string, text string) {
	if screen.shown == nil {
		screen.shown = map[string]string{}
	}
	if _, held := screen.shown[id]; !held && len(screen.shown) >= maxShownTexts {
		delete(screen.shown, screen.oldestPillID(func(held string) bool { _, kept := screen.shown[held]; return kept }))
	}
	screen.shown[id] = keepTail(text)
}

// oldestPillID is the id, among those the test given holds, whose pill is
// oldest on the transcript, or, when none of them is on the transcript any
// more, the first that the maps come to, because a text whose pill has gone
// is one nobody can open. The walk is over the transcript's blocks, which are
// capped, and over one map, which is capped too.
func (screen *Screen) oldestPillID(holds func(id string) bool) string {
	for _, item := range screen.blocks {
		if item.kind != blockTool {
			continue
		}
		if id := resultIDOf(item.text); id != "" && holds(id) {
			return id
		}
	}
	for id := range screen.shown {
		if holds(id) {
			return id
		}
	}
	for id := range screen.expanded {
		if holds(id) {
			return id
		}
	}
	return ""
}

// collapseEveryPill folds every open pill.
func (screen *Screen) collapseEveryPill() {
	screen.expanded = nil
}

// forgetTheOpenPills is what a clear does to this side of the screen: the
// transcript is empty, so nothing is open and nothing is focused.
func (screen *Screen) forgetTheOpenPills() {
	screen.collapseEveryPill()
	screen.clearFocus()
}

// shownArrived takes the program's answer to a show for a result: the text
// goes under the result's id, in place of the empty text that marked it as
// asked for, and the pill draws it on the next frame. An answer with no text
// at all still says so, so the pill never says fetching for ever.
func (screen *Screen) shownArrived(id string, text string) {
	if text == "" {
		text = "(the program sent nothing back for " + id + ")"
	}
	screen.rememberShown(id, text)
}

// errorAnswersAShow takes an error the program sent in place of a result's
// text, which is an error naming an id this screen asked for, and draws its
// words under the pill rather than as a card. It says whether the error was
// one of those; any other error is a card.
func (screen *Screen) errorAnswersAShow(envelope contract.SocketEnvelope) bool {
	id := envelope.Fields[showFieldID]
	if id == "" {
		return false
	}
	if _, asked := screen.shown[id]; !asked {
		return false
	}
	screen.rememberShown(id, "error: "+troubleWords(envelope))
	return true
}

// expansionLines is what hangs under a pill: nothing while it is folded, one
// dim line saying the text is on its way while the program has not answered,
// and the text itself once it has.
func (screen *Screen) expansionLines(item block) []string {
	id := resultIDOf(item.text)
	if id == "" || !screen.expanded[id] {
		return nil
	}
	text, answered := screen.shown[id]
	if !answered || text == "" {
		return []string{screen.lineUnderThePill(styleDim, "fetching "+id+string(ellipsisGlyph))}
	}
	return screen.textUnderThePill(text)
}

// textUnderThePill draws a result's text wrapped to the transcript's width,
// less the two columns it is drawn in past the pill, at most maxExpandedLines
// of it with the last line saying how many more there are.
func (screen *Screen) textUnderThePill(text string) []string {
	wrapped := wrapText(text, max(screen.transcriptWidth()-2, 1))
	if len(wrapped) > maxExpandedLines {
		more := len(wrapped) - (maxExpandedLines - 1)
		wrapped = append(wrapped[:maxExpandedLines-1], string(ellipsisGlyph)+" and "+strconv.Itoa(more)+" more lines")
	}
	drawn := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		drawn = append(drawn, screen.labelledLine(line))
	}
	return drawn
}

// labelledLine draws one line of a result's text in the column under the pill:
// a line that begins with one of the labels has the label dim and the words
// after it plain, and any other line is plain from end to end.
func (screen *Screen) labelledLine(text string) string {
	line := row{}
	line.blanks(pillTextIndent)
	for _, label := range textLabels {
		if rest, labelled := strings.CutPrefix(text, label); labelled {
			line.add(styleDim, label)
			line.add(styleNormal, rest)
			return line.render(screen.colors)
		}
	}
	line.add(styleNormal, text)
	return line.render(screen.colors)
}

// lineUnderThePill draws one line in one style in the column under the pill.
func (screen *Screen) lineUnderThePill(chosen style, text string) string {
	line := row{}
	line.blanks(pillTextIndent)
	line.add(chosen, text)
	return line.render(screen.colors)
}
