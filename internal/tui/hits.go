package tui

// Where everything on the frame is. The rows between the two rules are
// composed here with a record of what each row belongs to, so that the frame
// and the map a click is looked up in come from one drawing and cannot
// disagree.

// composeMiddle draws the rows between the two rules for a frame of one shape:
// the transcript, with the palette under it, and says for every row which
// block it draws, or minus one for a row that is nobody's, such as the blank
// between two blocks, the welcome, or the palette. The pill at focused is drawn
// as the focused one.
func (screen *Screen) composeMiddle(shape frameShape, focused int) ([]string, []int) {
	rows, owners := screen.visibleTranscript(shape.transcriptHeight, focused)
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
