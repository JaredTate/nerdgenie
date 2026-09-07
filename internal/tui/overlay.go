package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The record overlay. A row of the side panel names a task or a job, and
// Enter on it when it is focused, or a click on it, asks the program for that
// record; the answer is drawn over the transcript, between the two rules and
// left of the panel, with a title above it, until Esc closes it. The input
// box works the whole time, so a person can read the record and type the next
// thing at once, and the panel goes on drawing beside it.

// The two kinds of panel target, written before the colon: "task:t3" for a
// task's record and "job:4" for a job's.
const (
	targetTask = "task"
	targetJob  = "job"
)

// openTarget asks the program for the record a panel target names, and does
// nothing for a target it does not know or one that names nothing.
func (screen *Screen) openTarget(target string) {
	kind, name, found := strings.Cut(target, ":")
	if !found || name == "" {
		return
	}
	switch kind {
	case targetTask:
		fields := map[string]string{showFieldTask: name}
		// A task named the job's way, t2, is found through its job, because
		// the program numbers tasks its own way.
		if isAJobTaskName(name) && screen.job != "" {
			fields[showFieldJob] = screen.job
		}
		screen.tell(contract.SocketEnvelope{Type: contract.SocketShow, Fields: fields})
	case targetJob:
		screen.tell(contract.SocketEnvelope{Type: contract.SocketShow, Fields: map[string]string{showFieldJob: name}})
	}
}

// isAJobTaskName says whether a name is a task named the job's way: the letter
// t and a number.
func isAJobTaskName(name string) bool {
	if len(name) < 2 || name[0] != 't' {
		return false
	}
	for _, letter := range name[1:] {
		if letter < '0' || letter > '9' {
			return false
		}
	}
	return true
}

// overlayOpen says whether a record is drawn over the transcript.
func (screen *Screen) overlayOpen() bool {
	return screen.overlayTitle != ""
}

// closeOverlay takes the record off the transcript.
func (screen *Screen) closeOverlay() {
	screen.overlayTitle, screen.overlayText, screen.overlayScroll = "", "", 0
}

// recordArrived takes the program's answer to a show for a task's or a job's
// record and opens the overlay on it, from the top. An answer that names
// neither is nothing this screen asked for and is left alone. A focus that
// was on a pill is let go of first, because the overlay covers the pills and
// the focus is an index into the list of what can be seen: left in place it
// would slide onto the panel's first row, and the Esc meant for the overlay
// would be spent on that instead.
func (screen *Screen) recordArrived(envelope contract.SocketEnvelope) {
	title := screen.overlayTitleFor(envelope.Fields)
	if title == "" {
		return
	}
	text := envelope.Text
	if text == "" {
		text = "(the program sent nothing back for this record)"
	}
	if screen.focused().block >= 0 {
		screen.clearFocus()
	}
	screen.overlayTitle, screen.overlayText, screen.overlayScroll = title, keepTail(text), 0
}

// overlayTitleFor is the title above a record: "job 4 · Tater Tots Tetris"
// for a job, with the name when it is the job the screen knows the name of,
// and "task 17" for a task; or nothing when the fields name neither.
func (screen *Screen) overlayTitleFor(fields map[string]string) string {
	switch {
	case fields[showFieldJob] != "" && fields[showFieldTask] != "":
		return "task " + fields[showFieldTask] + " · job " + fields[showFieldJob]
	case fields[showFieldJob] != "":
		title := "job " + fields[showFieldJob]
		if fields[showFieldJob] == screen.job && screen.jobName != "" {
			title += " · " + screen.jobName
		}
		return title
	case fields[showFieldTask] != "":
		return "task " + taskNumber(fields[showFieldTask])
	default:
		return ""
	}
}

// overlayRows draws the record in a number of rows: the title on the first,
// then the text wrapped to the width between the margins, from however far
// down it has been scrolled, which is pulled back here so that the last line
// never scrolls above the bottom row.
func (screen *Screen) overlayRows(height int) []string {
	if height < 1 {
		return []string{}
	}
	width := max(screen.transcriptColumns()-2*marginColumns, 1)
	title := row{}
	title.blanks(marginColumns)
	title.add(styleBold, cutWithEllipsis(screen.overlayTitle, width))
	drawn := []string{title.render(screen.colors)}

	lines := wrapText(screen.overlayText, width)
	room := height - 1
	screen.overlayScroll = min(screen.overlayScroll, max(len(lines)-room, 0))
	for _, text := range lines[screen.overlayScroll:min(screen.overlayScroll+room, len(lines))] {
		line := row{}
		line.blanks(marginColumns)
		line.add(styleNormal, text)
		drawn = append(drawn, line.render(screen.colors))
	}
	for len(drawn) < height {
		drawn = append(drawn, "")
	}
	return drawn
}

// scrolledTheOverlay holds the keys that scroll the record while it is open,
// and says whether the key was one of them: Up and Down by a row, Page Up and
// Page Down by a page.
func (screen *Screen) scrolledTheOverlay(key tea.KeyPressMsg) bool {
	switch key.Code {
	case tea.KeyUp:
		screen.scrollOverlayBy(-1)
	case tea.KeyDown:
		screen.scrollOverlayBy(1)
	case tea.KeyPgUp:
		screen.scrollOverlayBy(-screen.pageRows())
	case tea.KeyPgDown:
		screen.scrollOverlayBy(screen.pageRows())
	default:
		return false
	}
	return true
}

// scrollOverlayBy moves the record up or down by a number of rows. It never
// goes above the top here, and how far down it may go is found when the frame
// is next drawn, which is the one place that knows how many rows there are.
func (screen *Screen) scrollOverlayBy(rows int) {
	screen.overlayScroll = max(screen.overlayScroll+rows, 0)
}
