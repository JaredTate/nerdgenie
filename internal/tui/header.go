package tui

import "time"

// healthFreshFor is how recently the program must have answered its health check
// for the dot on the right of the header to be filled.
const healthFreshFor = 10 * time.Second

// headerRow draws the one row at the top: the wordmark, the model alias, the
// task state, what this session has cost so far, and the health dot on the
// right. Everything is dim except the task state, which is accent while a task
// runs.
func (screen *Screen) headerRow() string {
	line := row{}
	line.blanks(marginColumns)
	line.add(styleDim, "coeus")
	for _, piece := range screen.headerParts() {
		line.add(styleDim, " · ")
		line.addSpan(piece)
	}
	if screen.width >= narrowWidth {
		mark := screen.healthMark()
		line.padTo(screen.width - marginColumns - displayWidth(mark.text))
		line.addSpan(mark)
	}
	line.keepWithin(screen.width - marginColumns)
	return line.render(screen.colors)
}

// headerParts are the pieces between the wordmark and the health dot. Before the
// link is made there is only one of them, and it says so.
func (screen *Screen) headerParts() []span {
	if !screen.attached {
		return []span{{style: styleDim, text: "connecting"}}
	}
	parts := []span{}
	if screen.modelAlias != "" {
		parts = append(parts, span{style: styleDim, text: screen.modelAlias})
	}
	if task := screen.taskWords(); task != "" {
		parts = append(parts, span{style: screen.taskStyle(), text: task})
	}
	if cost := screen.costWords(); cost != "" {
		parts = append(parts, span{style: styleDim, text: cost})
	}
	return parts
}

// taskWords is the task and its state, such as "task 17 running", or empty when
// no task is running.
func (screen *Screen) taskWords() string {
	switch {
	case screen.taskID == "" && screen.taskState == "":
		return ""
	case screen.taskID == "":
		return screen.taskState
	case screen.taskState == "":
		return screen.taskID
	default:
		return screen.taskID + " " + screen.taskState
	}
}

// taskStyle draws a running task in the accent colour and every other state
// plainly, so that the eye finds the one thing that is happening.
func (screen *Screen) taskStyle() style {
	if screen.taskState == "running" {
		return styleAccent
	}
	return styleNormal
}

// costWords is what this session has cost so far: the tokens in and out, and the
// money when the provider reports it.
func (screen *Screen) costWords() string {
	words := ""
	if screen.tokensIn != "" || screen.tokensOut != "" {
		words = screen.tokensIn + " in " + screen.tokensOut + " out"
	}
	if screen.money == "" {
		return words
	}
	if words == "" {
		return screen.money
	}
	return words + " · " + screen.money
}

// healthMark is the dot on the right of the header and the word beside it:
// filled and accent when the program answered its health check in the last ten
// seconds, hollow and dim when it has not, and error-coloured when there is no
// link at all.
func (screen *Screen) healthMark() span {
	switch {
	case !screen.attached:
		return span{style: styleError, text: string(hollowDotGlyph) + " offline"}
	case screen.now.Sub(screen.lastHealth) <= healthFreshFor:
		return span{style: styleAccent, text: string(filledDotGlyph) + " healthy"}
	default:
		return span{style: styleDim, text: string(hollowDotGlyph) + " quiet"}
	}
}
