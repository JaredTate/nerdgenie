package tui

import (
	"strconv"
	"strings"
	"time"
)

// healthFreshFor is how recently the program must have answered its health check
// for the dot on the right of the header to be filled.
const healthFreshFor = 10 * time.Second

// headerRow draws the one row at the top: the wordmark in its two colours, the
// tagline, the model alias, the context meter, the task and its state, how
// long it has run and which round it is on, the cache share, what this
// session has cost so far, and the health dot on the right. Everything after
// the wordmark is dim except the task, which is bold while it runs, and the
// meters, which carry their own colours. On a terminal too narrow for all of
// it the tagline goes first, and then the pieces go whole from the right,
// least important last, until the row fits with the health mark and a gap in
// front of it; a piece is never cut in half while there is a whole piece to
// drop instead.
func (screen *Screen) headerRow() string {
	pieces := screen.headerPieces()
	line := screen.headerLine(true, pieces)
	if !screen.headerFits(line) {
		line = screen.headerLine(false, pieces)
	}
	for !screen.headerFits(line) && len(pieces) > 0 {
		pieces = pieces[:len(pieces)-1]
		line = screen.headerLine(false, pieces)
	}
	if screen.width >= narrowWidth {
		line.addRightPiece(screen.healthMark(), screen.width)
	}
	line.keepWithin(screen.width - marginColumns)
	return line.render(screen.colors)
}

// headerLine is the left-hand side of the header: the wordmark, the tagline
// when it is asked for, and the pieces given, each after a dim dot.
func (screen *Screen) headerLine(withTagline bool, pieces [][]span) row {
	line := row{}
	line.blanks(marginColumns)
	line.add(styleBold, wordmarkFirst)
	line.add(styleBrand, wordmarkSecond)
	if withTagline {
		line.add(styleDim, " · "+taglineText)
	}
	for _, piece := range pieces {
		line.add(styleDim, " · ")
		for _, part := range piece {
			line.addSpan(part)
		}
	}
	return line
}

// headerFits says whether a header line leaves room for the health mark and
// the gap in front of it, or, on a terminal too narrow to draw the mark at all,
// whether it fits inside the frame.
func (screen *Screen) headerFits(line row) bool {
	room := screen.width - marginColumns
	if screen.width >= narrowWidth {
		room -= displayWidth(screen.healthMark().text) + minimumGap
	}
	return line.width <= room
}

// headerPieces are the pieces between the wordmark and the health dot, most
// important first, because the header drops them from the right. A link that
// is not there is said in so many words first, and everything the program
// last reported is kept behind it, because a person whose link dropped still
// wants to know which model was running and what the session had cost. Each
// piece is one or more spans drawn together, so that the meter is dropped
// whole rather than cell by cell.
func (screen *Screen) headerPieces() [][]span {
	pieces := [][]span{}
	if !screen.attached {
		pieces = append(pieces, []span{screen.linkPiece()})
	}
	if screen.modelAlias != "" {
		pieces = append(pieces, []span{{style: styleDim, text: screen.modelWords()}})
	}
	if meter := screen.contextParts(); len(meter) > 0 {
		pieces = append(pieces, meter)
	}
	if task := screen.taskWords(); task != "" {
		pieces = append(pieces, []span{{style: screen.taskStyle(), text: task}})
	}
	if elapsed := screen.taskElapsedWords(); elapsed != "" {
		pieces = append(pieces, []span{{style: styleDim, text: elapsed}})
	}
	if round := screen.roundWords(); round != "" {
		pieces = append(pieces, []span{{style: styleDim, text: round}})
	}
	if cache := screen.cachePart(); cache.text != "" {
		pieces = append(pieces, []span{cache})
	}
	if cost := screen.costWords(); cost != "" {
		pieces = append(pieces, []span{{style: styleDim, text: cost}})
	}
	return pieces
}

// headerParts are the header's pieces as one flat run of spans, which is what
// a test reads them as.
func (screen *Screen) headerParts() []span {
	parts := []span{}
	for _, piece := range screen.headerPieces() {
		parts = append(parts, piece...)
	}
	return parts
}

// contextParts are the pieces that say how much of the model's context the
// last call used: a dim label, the ten-cell meter, and the share beside it,
// the filled cells and the share coloured green, amber or red as the context
// fills and the empty cells muted. They are drawn only when the program sent
// both numbers, because a share of a window nobody named is not a fact the
// screen has.
func (screen *Screen) contextParts() []span {
	share := contextShare(screen.contextTokens, screen.contextWindow)
	if share < 0 {
		return nil
	}
	parts := []span{{style: styleDim, text: "ctx "}}
	parts = append(parts, meterSpans(share, contextMeterCells, meterStyle(share))...)
	return append(parts, span{style: meterStyle(share), text: " " + strconv.Itoa(share) + "%"})
}

// cachePart says how much of the last call's input the provider reused, such
// as "cache 92%", coloured by how warm the cache is, or nothing when the
// share is unknown.
func (screen *Screen) cachePart() span {
	share := cacheShare(screen.cachedTokens, screen.contextTokens)
	if share < 0 {
		return span{}
	}
	return span{style: cacheStyle(share), text: "cache " + strconv.Itoa(share) + "%"}
}

// taskElapsedWords is how long the running task has run, such as "12m", or
// nothing when the program has not said when it began.
func (screen *Screen) taskElapsedWords() string {
	if screen.taskStarted.IsZero() {
		return ""
	}
	return elapsedWords(screen.now.Sub(screen.taskStarted))
}

// roundWords is the task's round the short way, such as "r27", or nothing
// when the program has not said or the task has not made a call yet.
func (screen *Screen) roundWords() string {
	if screen.round == "" || screen.round == "0" {
		return ""
	}
	return "r" + screen.round
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
	if count < 1000000 {
		return strconv.Itoa((count+500)/1000) + "k"
	}
	return strings.TrimSuffix(strconv.FormatFloat(float64(count)/1000000, 'f', 1, 64), ".0") + "M"
}

// linkPiece is what the header says about a link that is not there. A screen
// that has never reached the program is still connecting, which is ordinary and
// is drawn quietly; a link that was up and went away is a failure and is drawn
// loud, in bold white.
func (screen *Screen) linkPiece() span {
	if screen.everAttached {
		return span{style: styleBold, text: "disconnected"}
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

// taskStyle draws a running task in bold white and every other state
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
		words = countWords(screen.tokensIn) + " in " + countWords(screen.tokensOut) + " out"
	}
	money := moneyWords(screen.money)
	if money == "" {
		return words
	}
	if words == "" {
		return money
	}
	return words + " · " + money
}

// countWords draws a count the program sent as a bare number the way the
// context meter draws one, "5.9M" or "121k", and leaves a value already
// written for people, such as "6.1k", as it is.
func countWords(raw string) string {
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	return tokenWords(count)
}

// moneyWords draws a cost the program sent as a bare number with a dollar
// sign and two decimals, "$7.28", and leaves a value already written for
// people as it is. Nothing is nothing.
func moneyWords(raw string) string {
	raw = strings.TrimSpace(raw)
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	return "$" + strconv.FormatFloat(amount, 'f', 2, 64)
}

// healthMark is the dot on the right of the header and the word beside it:
// filled and accent when the program answered its health check in the last ten
// seconds, hollow and dim when it has not, and hollow in bold white when there
// is no link at all.
func (screen *Screen) healthMark() span {
	switch {
	case !screen.attached:
		return span{style: styleBold, text: string(hollowDotGlyph) + " offline"}
	case screen.now.Sub(screen.lastHealth) <= healthFreshFor:
		return span{style: styleAccent, text: string(filledDotGlyph) + " healthy"}
	default:
		return span{style: styleDim, text: string(hollowDotGlyph) + " quiet"}
	}
}

// modelWords is the alias and, when the program named it, the model file the
// daemon loaded without its ending, such as "local hauhau-Q4_K_P", so that a
// person sees which model is running without opening the panel.
func (screen *Screen) modelWords() string {
	if screen.modelFile == "" {
		return screen.modelAlias
	}
	file := screen.modelFile
	if at := strings.LastIndex(file, "."); at > 0 {
		file = file[:at]
	}
	return screen.modelAlias + " " + file
}
