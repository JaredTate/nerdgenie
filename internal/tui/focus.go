package tui

import tea "charm.land/bubbletea/v2"

// The keyboard focus. Tab moves it to the next thing on the frame a person can
// open and Shift+Tab to the one before: the tool pills on the visible
// transcript from the oldest to the newest, then the rows of the side panel
// that name a job or a task. The focus is a place in that list, counted again
// on every frame from what is on the frame, so it never points at a row that
// has scrolled away; it takes no letters, because letters belong to the input
// box, and Esc lets go of it.

// noFocus is the value of focusAt while nothing is focused.
const noFocus = -1

// focusable is one thing the focus can land on: a tool pill, found by the
// block that draws it, or a row of the side panel, found by its line in the
// panel and the target the panel names for it. A pill has a line of minus one
// and a panel row a block of minus one.
type focusable struct {
	// block is the index of the pill's block in the transcript.
	block int
	// line is the row of the panel, counted from the panel's first line.
	line int
	// target is what the panel names for that row, such as "job:4".
	target string
}

// interactionHints are the words for the keys this side of the screen owns,
// which the status strip's footer reads: one short pair per key.
func interactionHints() []string {
	return []string{"tab focus", "↵ expand", "^B panel", "⇞⇟ scroll"}
}

// focusables is everything the focus can land on right now, in Tab order.
func (screen *Screen) focusables() []focusable {
	return screen.focusablesIn(screen.shape())
}

// focusablesIn lists the focusable things on a frame of one shape: the pills
// on the visible rows, then the panel rows that name a target, as far down as
// the panel is drawn.
func (screen *Screen) focusablesIn(shape frameShape) []focusable {
	_, owners := screen.composeMiddle(shape, noFocus)
	targets := []string{}
	if screen.showsPanel() {
		targets = screen.panelTargets()
	}
	return screen.focusablesFrom(owners, targets, len(owners))
}

// focusablesFrom builds the focus list from the owner of every row between the
// two rules and the target of every panel line: each pill that carries a
// result id once, in the order its rows come, then each panel line with a
// target, up to the number of rows the panel is drawn on.
func (screen *Screen) focusablesFrom(owners []int, targets []string, panelRows int) []focusable {
	listed := []focusable{}
	last := -1
	for _, owner := range owners {
		if owner < 0 || owner == last || owner >= len(screen.blocks) {
			continue
		}
		last = owner
		if screen.blocks[owner].kind == blockTool && resultIDOf(screen.blocks[owner].text) != "" {
			listed = append(listed, focusable{block: owner, line: -1})
		}
	}
	for at, target := range targets {
		if at >= panelRows {
			break
		}
		if target != "" {
			listed = append(listed, focusable{block: -1, line: at, target: target})
		}
	}
	return listed
}

// nothingFocused is the focusable that stands for no focus at all.
func nothingFocused() focusable {
	return focusable{block: -1, line: -1}
}

// focusIn is the focused thing on a frame of one shape, or nothingFocused when
// the focus is off or points past the end of a list that has shrunk.
func (screen *Screen) focusIn(shape frameShape) focusable {
	if screen.focusAt < 0 {
		return nothingFocused()
	}
	items := screen.focusablesIn(shape)
	if screen.focusAt >= len(items) {
		return nothingFocused()
	}
	return items[screen.focusAt]
}

// focused is the focused thing on the next frame.
func (screen *Screen) focused() focusable {
	return screen.focusIn(screen.shape())
}

// focusedBlock is the block of the focused pill, or minus one when the focus
// is off or on a panel row.
func (screen *Screen) focusedBlock() int {
	return screen.focused().block
}

// moveFocus moves the focus one place forward or back through the list, and
// round the end of it, and turns it off when there is nothing to land on.
func (screen *Screen) moveFocus(step int) {
	items := screen.focusables()
	if len(items) == 0 {
		screen.focusAt = noFocus
		return
	}
	if screen.focusAt < 0 || screen.focusAt >= len(items) {
		if step > 0 {
			screen.focusAt = 0
		} else {
			screen.focusAt = len(items) - 1
		}
		return
	}
	screen.focusAt = ((screen.focusAt+step)%len(items) + len(items)) % len(items)
}

// clearFocus turns the focus off.
func (screen *Screen) clearFocus() {
	screen.focusAt = noFocus
}

// pressedOnTheFocus holds the keys the focus takes: Tab and Shift+Tab move it,
// and Esc lets go of it. It says whether the key was one of them; every other
// key goes on to whoever holds the keys.
func (screen *Screen) pressedOnTheFocus(key tea.KeyPressMsg) bool {
	switch {
	case key.Code == tea.KeyTab && key.Mod == tea.ModShift:
		screen.moveFocus(-1)
	case key.Code == tea.KeyTab && key.Mod == 0:
		screen.moveFocus(1)
	case key.Code == tea.KeyEsc && screen.focusAt >= 0:
		screen.clearFocus()
	default:
		return false
	}
	return true
}

// focusedPanelRow draws one line of the panel as the focused one: the panel's
// edge, then the line as panelLines built it with its first glyph in reverse
// video, which reads on a terminal with no colour at all. The edge is drawn
// here again because the panel's own drawing has no hook for a focused row.
func (screen *Screen) focusedPanelRow(at int) string {
	lines := screen.panelLines()
	if at < 0 || at >= len(lines) {
		return ""
	}
	drawn := row{}
	drawn.add(styleDim, string(panelEdgeGlyph)+" ")
	for number, piece := range lines[at].spans {
		if number == 0 {
			piece.style = styleReverse
		}
		drawn.addSpan(piece)
	}
	return drawn.render(screen.colors)
}

// drawTheFocusedPanelRow puts the focused panel row onto the frame in place of
// the plain one, beside the transcript row it shares, when the focus is on a
// panel row that is drawn.
func (screen *Screen) drawTheFocusedPanelRow(rows []string, middle []string, focus focusable) {
	at := focus.line
	if at < 0 || at >= len(middle) || !screen.showsPanel() {
		return
	}
	rows[2+at] = screen.paintTo(middle[at], screen.transcriptColumns()) + screen.focusedPanelRow(at)
}
