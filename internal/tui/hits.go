package tui

import tea "charm.land/bubbletea/v2"

// Where everything on the frame is. The rows between the two rules are
// composed here with a record of what each row belongs to, so that the frame
// and the map a click is looked up in come from one drawing and cannot
// disagree.

// hitMap is what every row of the frame holds under the mouse: for the rows
// between the two rules, the block each one draws, and beside them, from the
// column the panel begins in, the target of each panel line.
type hitMap struct {
	// firstRow is the row of the frame the rows between the rules begin on,
	// which is the row under the header and its rule.
	firstRow int
	// owners is the block each row draws, or minus one, in the order the rows
	// are drawn.
	owners []int
	// targets is what the panel names for each of its lines, in order, as
	// panelTargets gives them.
	targets []string
	// panelFrom is the column the panel begins in, or minus one when there is
	// no panel on the frame.
	panelFrom int
}

// hit is what a click landed on: a block of the transcript, a panel row by its
// target, or nothing at all, which is a block of minus one and no target.
type hit struct {
	// block is the index of the block clicked, or minus one.
	block int
	// target is the panel target clicked, or empty.
	target string
}

// nothingHit is the hit for a click that landed on nothing.
func nothingHit() hit {
	return hit{block: -1}
}

// at says what a click at a column and a row of the frame lands on: a panel
// line's target from the panel's first column on, a block on any row it draws
// left of the panel, and nothing anywhere else.
func (hits hitMap) at(column int, rowOnScreen int) hit {
	at := rowOnScreen - hits.firstRow
	if at < 0 || at >= len(hits.owners) {
		return nothingHit()
	}
	if hits.panelFrom >= 0 && column >= hits.panelFrom {
		if at < len(hits.targets) && hits.targets[at] != "" {
			return hit{block: -1, target: hits.targets[at]}
		}
		return nothingHit()
	}
	if hits.owners[at] >= 0 {
		return hit{block: hits.owners[at]}
	}
	return nothingHit()
}

// hits builds the map for the next frame from the same rows the frame draws.
// It is built when a click arrives rather than kept from the last frame,
// because the two are drawn from the same state and the map would otherwise
// be one more thing to keep true.
func (screen *Screen) hits() hitMap {
	_, owners := screen.composeMiddle(screen.shape(), noFocus)
	found := hitMap{firstRow: 2, owners: owners, panelFrom: -1}
	if screen.showsPanel() {
		found.panelFrom = screen.transcriptColumns()
		found.targets = screen.panelTargets()
	}
	return found
}

// clicked takes one press of a mouse button on the frame. Only the left button
// does anything, and only on a pill or a panel row; a click anywhere else is
// nothing, and a click is activity, so it disarms a half-pressed quit.
func (screen *Screen) clicked(press tea.MouseClickMsg) {
	if press.Button != tea.MouseLeft {
		return
	}
	screen.quitArmed = false
	screen.clickedOn(screen.hits().at(press.X, press.Y))
}

// clickedOn does what a click on one thing does: a pill is toggled, and a
// panel row's record is asked for.
func (screen *Screen) clickedOn(found hit) {
	switch {
	case found.block >= 0:
		screen.togglePillAt(found.block)
	case found.target != "":
		screen.openTarget(found.target)
	}
}

// composeMiddle draws the rows between the two rules for a frame of one shape:
// the transcript, or the record overlay in its place while one is open, with
// the palette under it, and says for every row which block it draws, or minus
// one for a row that is nobody's, such as the blank between two blocks, the
// welcome, the overlay, or the palette. The pill at focused is drawn as the
// focused one.
func (screen *Screen) composeMiddle(shape frameShape, focused int) ([]string, []int) {
	rows, owners := screen.visibleTranscript(shape.transcriptHeight, focused)
	if screen.overlayOpen() {
		rows, owners = screen.overlayRows(shape.transcriptHeight), nobodys(shape.transcriptHeight)
	}
	rows = append(rows, shape.palette...)
	owners = append(owners, nobodys(len(shape.palette))...)
	return rows, owners
}

// nobodys is a run of owners that are nobody's.
func nobodys(count int) []int {
	owners := make([]int, max(count, 0))
	for at := range owners {
		owners[at] = -1
	}
	return owners
}

// belongingTo is a run of owners that are all one block's.
func belongingTo(block int, count int) []int {
	owners := make([]int, max(count, 0))
	for at := range owners {
		owners[at] = block
	}
	return owners
}
