package tui

import tea "charm.land/bubbletea/v2"

// How the transcript scrolls: by the mouse wheel, which the screen asks the
// terminal to report, and by the keys that do the same. The view is measured in
// rows up from the newest row, so that a view left alone rests on the newest
// content, and the arithmetic that holds the view still while new output
// arrives under it is here too.

// wheelRows is how far one notch of the mouse wheel, Shift+Up, or Shift+Down
// moves the transcript.
const wheelRows = 3

// pageOverlap is how many rows of one screenful Page Up and Page Down keep on
// the frame from the last, so that the eye has a row it has already read to
// find its place by.
const pageOverlap = 2

// pageRows is how far one Page Up or Page Down moves the transcript, or the
// overlay when one is open: the rows between the two rules less the overlap,
// and never less than one.
func (screen *Screen) pageRows() int {
	return max(screen.shape().transcriptHeight-pageOverlap, 1)
}

// wheeled takes one turn of the mouse wheel: up shows older rows, down shows
// newer ones, and the wheel pushed sideways does nothing.
func (screen *Screen) wheeled(turn tea.MouseWheelMsg) {
	switch turn.Button {
	case tea.MouseWheelUp:
		screen.scrollBy(wheelRows)
	case tea.MouseWheelDown:
		screen.scrollBy(-wheelRows)
	}
}

// scrolledWithTheKey holds the keys that scroll the transcript, and says
// whether the key was one of them. They belong to the screen as a whole rather
// than to whatever holds the other keys, so that a person can read back while a
// card waits for an answer or the masked prompt is open. End is one of them
// only while the view is scrolled up; at the bottom it belongs to the input
// box, where it moves the cursor to the end of the line.
func (screen *Screen) scrolledWithTheKey(key tea.KeyPressMsg) bool {
	switch {
	case key.Code == tea.KeyPgUp:
		screen.scrollBy(screen.pageRows())
	case key.Code == tea.KeyPgDown:
		screen.scrollBy(-screen.pageRows())
	case key.Code == tea.KeyUp && key.Mod == tea.ModShift:
		screen.scrollBy(wheelRows)
	case key.Code == tea.KeyDown && key.Mod == tea.ModShift:
		screen.scrollBy(-wheelRows)
	case key.Code == tea.KeyEnd && key.Mod == 0 && screen.scrolledUp():
		screen.showTheNewest()
	default:
		return false
	}
	return true
}

// scrollBy moves the view up or down inside the transcript. It never goes below
// the newest row here, and how far up it may go is found when the frame is next
// drawn, which is the one place that knows how many rows there really are.
func (screen *Screen) scrollBy(rows int) {
	screen.scrollBack = max(screen.scrollBack+rows, 0)
}

// scrolledUp says whether the person has scrolled away from the newest content,
// which is what puts the older mark in the status strip and what holds the view
// still while new output arrives.
func (screen *Screen) scrolledUp() bool {
	return screen.scrollBack > 0
}

// showTheNewest brings the view back to the newest row, which is where it goes
// when the person sends a message and when a card arrives that needs them.
func (screen *Screen) showTheNewest() {
	screen.scrollBack = 0
}

// rowsAddedBy is how many rows one more block puts on the end of the
// transcript, counting the blank line drawn before it.
func (screen *Screen) rowsAddedBy(added block) int {
	rows := len(screen.blockLines(added))
	if last := len(screen.blocks) - 1; last >= 0 && blankBetween(screen.blocks[last].kind, added.kind) {
		rows++
	}
	return rows
}

// setText changes the text of a block already on the screen. When the person
// has scrolled up, the view is held still by moving it as many rows from the
// newest row as the change added or took away; the rows are counted only then,
// because drawing a block twice on every heartbeat is a cost the ordinary case
// need not pay.
func (screen *Screen) setText(item *block, text string) {
	if !screen.scrolledUp() {
		item.text = keepTail(text)
		return
	}
	before := len(screen.blockLines(*item))
	item.text = keepTail(text)
	screen.scrollBack = max(screen.scrollBack+len(screen.blockLines(*item))-before, 0)
}
