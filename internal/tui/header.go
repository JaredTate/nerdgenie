package tui

import (
	"strconv"
	"strings"
	"time"
)

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
	line.add(styleDim, "nerdgenie")
	for _, piece := range screen.headerParts() {
		line.add(styleDim, " · ")
		line.addSpan(piece)
	}
	if screen.width >= narrowWidth {
		line.addRightPiece(screen.healthMark(), screen.width)
	}
	line.keepWithin(screen.width - marginColumns)
	return line.render(screen.colors)
}

// headerParts are the pieces between the wordmark and the health dot. A link
// that is not there is said in so many words first, and everything the program
// last reported is kept behind it, because a person whose link dropped still
// wants to know which model was running and what the session had cost.
func (screen *Screen) headerParts() []span {
	parts := []span{}
	if !screen.attached {
		parts = append(parts, screen.linkPiece())
	}
	if screen.modelAlias != "" {
		parts = append(parts, span{style: styleDim, text: screen.modelAlias})
	}
	parts = append(parts, screen.contextParts()...)
	if task := screen.taskWords(); task != "" {
		parts = append(parts, span{style: screen.taskStyle(), text: task})
	}
	if cost := screen.costWords(); cost != "" {
		parts = append(parts, span{style: styleDim, text: cost})
	}
	return parts
}

// The two shares of the model's context at which the header stops being quiet
// about how full it is, because a person who cannot see the context filling up
// finds out when the model forgets something.
const (
	// contextShareWarn is the share at which the measure turns the warning gold.
	contextShareWarn = 80
	// contextShareTrouble is the share at which it turns the error colour.
	contextShareTrouble = 95
)

// contextParts are the two pieces that say how much of the model's context the
// last call used: the measure itself, always quiet, and the share, which is the
// piece that turns gold and then red as the context fills. They are drawn only
// when the program sent both numbers, because a share of a window nobody named
// is not a fact the screen has.
func (screen *Screen) contextParts() []span {
	if screen.contextWindow <= 0 || screen.contextTokens <= 0 {
		return nil
	}
	share := (screen.contextTokens*100 + screen.contextWindow/2) / screen.contextWindow
	measure := "ctx " + tokenWords(screen.contextTokens) + " / " + tokenWords(screen.contextWindow)
	return []span{
		{style: styleDim, text: measure},
		{style: shareStyle(share), text: strconv.Itoa(share) + "%"},
	}
}

// shareStyle draws a context with room to spare quietly, one that is filling up
// in the warning gold, and one that is nearly full in the error colour.
func shareStyle(share int) style {
	switch {
	case share >= contextShareTrouble:
		return styleError
	case share >= contextShareWarn:
		return styleWarn
	default:
		return styleDim
	}
}

// tokenWords writes a count of tokens the short way a status line reads it:
// plainly below a thousand, then in thousands with one decimal place, and in
// whole thousands once the decimal place says nothing worth reading.
func tokenWords(count int) string {
	if count < 1000 {
		return strconv.Itoa(count)
	}
	if count < 100000 {
		return strings.TrimSuffix(strconv.FormatFloat(float64(count)/1000, 'f', 1, 64), ".0") + "k"
	}
	return strconv.Itoa((count+500)/1000) + "k"
}

// linkPiece is what the header says about a link that is not there. A screen
// that has never reached the program is still connecting, which is ordinary and
// is drawn quietly; a link that was up and went away is a failure and is drawn
// in the error colour.
func (screen *Screen) linkPiece() span {
	if screen.everAttached {
		return span{style: styleError, text: "disconnected"}
	}
	return span{style: styleDim, text: "connecting"}
}

// taskWords is the task and its state in the words docs/TUI_DESIGN.md writes
// them in, such as "task 17 running", or empty when the program has named no
// task. The state is drawn only when the program said what it is; the number
// alone is drawn when it did not, because a bare number in the middle of the
// header says nothing at all.
func (screen *Screen) taskWords() string {
	switch {
	case screen.taskID == "" && screen.taskState == "":
		return ""
	case screen.taskID == "":
		return screen.taskState
	case screen.taskState == "":
		return taskNamed(screen.taskID)
	default:
		return taskNamed(screen.taskID) + " " + screen.taskState
	}
}

// taskNamed puts the word "task" in front of a task's number, because the
// program sends the number on its own. A program that already says the word is
// not made to say it twice.
func taskNamed(id string) string {
	if strings.HasPrefix(id, "task") {
		return id
	}
	return "task " + id
}

// taskStyle draws a running task in the accent colour and every other state
// plainly, so that the eye finds the one thing that is happening.
func (screen *Screen) taskStyle() style {
	if screen.taskRunning() {
		return styleBold
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
