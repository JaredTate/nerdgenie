// The side panel is opencode's sidebar, whose drawing was read at
// ~/Code/opencode/packages/tui/src/routes/session/sidebar.tsx: a column down
// the right-hand side of the conversation, holding short quiet lines in
// groups with a blank line between them, each group under a small label, the
// loudest line at the top of each. Nothing was copied: that file is
// TypeScript drawing through a layout engine, and this is Go writing rows of
// its own.

package tui

import "strings"

const (
	// panelColumns is how wide the side panel is on a screen under
	// wideScreenFrom columns, its edge and its blanks and all.
	panelColumns = 28
	// panelFrom is the width of terminal at which the panel appears. Below it
	// there is no room for a panel and a transcript wide enough to read, so the
	// panel is dropped and nothing else changes.
	panelFrom = 100
	// panelTextColumns is how many columns the panel's words are drawn in at
	// its narrow width: its width, less the edge, the blank after it, and the
	// blank down its right.
	panelTextColumns = panelColumns - panelFrameColumns
	// panelFrameColumns are the columns of the panel that are not its words:
	// the edge, the blank after it, and the blank down its right.
	panelFrameColumns = 3
	// wideScreenFrom is the width of terminal from which the panel takes a
	// third of the screen rather than its narrow width, because a wide screen
	// has room for the record's state beside the talk.
	wideScreenFrom = 140
	// widestPanelColumns is the widest the panel ever grows, so that a very
	// wide screen keeps its transcript the loudest thing on it.
	widestPanelColumns = 56
)

const (
	// panelEdgeGlyph is the thin line down the left of the panel, which is what
	// separates it from the transcript when there is no colour at all.
	panelEdgeGlyph = '│'
	// doneGlyph is the check beside a task, a step or a call that is finished.
	doneGlyph = '✓'
	// runningGlyph is the pointer beside the task that is running now, beside
	// the step of its plan the work is at, and in front of a call in flight.
	runningGlyph = '▶'
	// plannedGlyph is the ring beside a task or a step still to come.
	plannedGlyph = '○'
	// ellipsisGlyph ends a row that was cut to fit the panel.
	ellipsisGlyph = '…'
)

// panelLine is one line of the panel and what a click on it lands on: "job:4"
// on a job's header, "task:t3" on one of its tasks or a plain task's header,
// and "" on a line that is nothing to click. The two are built together, so a
// row on the screen maps to the thing under it and the two cannot drift.
type panelLine struct {
	// drawn is the line as it is drawn.
	drawn row
	// target names what the line stands for, or is empty.
	target string
}

// showsPanel says whether the terminal is wide enough for the side panel and
// the person has not put it away with its key.
func (screen *Screen) showsPanel() bool {
	return screen.width >= panelFrom && !screen.panelHidden
}

// panelWidth is how wide the side panel is: its narrow width on a screen
// under wideScreenFrom columns, and a third of the screen from there, never
// more than widestPanelColumns.
func (screen *Screen) panelWidth() int {
	if screen.width < wideScreenFrom {
		return panelColumns
	}
	return min(screen.width/3, widestPanelColumns)
}

// panelTextWidth is how many columns the panel's words are drawn in.
func (screen *Screen) panelTextWidth() int {
	return screen.panelWidth() - panelFrameColumns
}

// transcriptColumns is how many columns the transcript and everything in it is
// drawn in: the whole frame, less the side panel when there is one.
func (screen *Screen) transcriptColumns() int {
	if screen.showsPanel() {
		return screen.width - screen.panelWidth()
	}
	return screen.width
}

// besideThePanel puts the side panel down the right-hand side of the rows
// between the two rules, and hands the rows back unchanged when the terminal is
// too narrow for one.
func (screen *Screen) besideThePanel(body []string) []string {
	if !screen.showsPanel() {
		return body
	}
	panel := screen.panelRows(len(body))
	beside := make([]string, len(body))
	for at, drawn := range body {
		beside[at] = screen.paintTo(drawn, screen.transcriptColumns()) + panel[at]
	}
	return beside
}

// panelRows draws the panel as many rows tall as the transcript beside it: what
// it has to say at the top, and the edge alone under it.
func (screen *Screen) panelRows(height int) []string {
	lines := screen.panelLines()
	drawn := make([]string, height)
	for at := range drawn {
		line := row{}
		line.add(styleDim, string(panelEdgeGlyph)+" ")
		if at < len(lines) {
			for _, piece := range lines[at].spans {
				line.addSpan(piece)
			}
		}
		drawn[at] = line.render(screen.colors)
	}
	return drawn
}

// panelLines is what the panel says, as rows.
func (screen *Screen) panelLines() []row {
	items := screen.panelItems()
	lines := make([]row, len(items))
	for at, item := range items {
		lines[at] = item.drawn
	}
	return lines
}

// panelTargets names what a click on each line of the panel lands on, one
// entry per line panelLines draws, out of the same walk.
func (screen *Screen) panelTargets() []string {
	items := screen.panelItems()
	targets := make([]string, len(items))
	for at, item := range items {
		targets[at] = item.target
	}
	return targets
}

// panelItems is the one walk the panel's lines and their targets come from:
// the groups in order, the model, what is happening now, the checklist, the
// record's state, its failures, the round, and how many jobs are waiting, with
// a blank line between them. A group the program has said nothing about is
// left out altogether, blank line and all.
func (screen *Screen) panelItems() []panelLine {
	items := []panelLine{}
	for _, group := range [][]panelLine{
		screen.labelled("MODEL", screen.modelPanelLines()),
		screen.labelled("NOW", screen.nowPanelLines()),
		screen.checklistItems(),
		screen.labelled("STATE", screen.statePanelLines()),
		screen.labelled("FAILURES", screen.failuresPanelLines()),
		screen.labelled("ROUND", screen.roundPanelLines()),
		screen.labelled("", screen.waitingPanelLines()),
	} {
		if len(group) == 0 {
			continue
		}
		if len(items) > 0 {
			items = append(items, panelLine{})
		}
		items = append(items, group...)
	}
	return items
}

// labelled puts a group's label above its lines: the label dim and
// upper-case, and the checklist's rule after it out to the panel's edge, so
// that a terminal with no colour still sees where one group ends and the
// next begins. A group with no lines is nothing, label and all, and a group
// with no label is its lines alone.
func (screen *Screen) labelled(label string, lines []row) []panelLine {
	if len(lines) == 0 {
		return nil
	}
	items := []panelLine{}
	if label != "" {
		items = append(items, panelLine{drawn: panelLabel(label, screen.panelTextWidth())})
	}
	for _, line := range lines {
		items = append(items, panelLine{drawn: line})
	}
	return items
}

// panelLabel draws a group's label with the rule after it, cut to the width.
func panelLabel(label string, width int) row {
	line := row{}
	line.add(styleDim, label+" ")
	if rule := width - line.width; rule > 0 {
		line.add(styleBrand, strings.Repeat(string(ruleGlyph), rule))
	}
	line.keepWithin(width)
	return line
}

// panelRule is the thin line above and below the checklist's rows, in
// DigiByte's own blue.
func panelRule(width int) row {
	line := row{}
	line.add(styleBrand, strings.Repeat(string(ruleGlyph), width))
	return line
}

// appendPanelWords puts one line of words in the panel, cut to it with an
// ellipsis, and puts nothing there when there are no words, because the panel
// says only what the program said.
func appendPanelWords(lines []row, chosen style, words string, width int) []row {
	if words == "" {
		return lines
	}
	line := row{}
	line.add(chosen, cutWithEllipsis(words, width))
	return append(lines, line)
}

// cutWithEllipsis shortens text to a number of columns with an ellipsis on the
// end, and leaves text that already fits alone.
func cutWithEllipsis(text string, width int) string {
	if width < 1 {
		return ""
	}
	if displayWidth(text) <= width {
		return text
	}
	return strings.TrimRight(cutTo(text, width-1), " ") + string(ellipsisGlyph)
}

// cutRowWithEllipsis shortens a row of several styles to a number of columns
// with an ellipsis on the end, in the style of the last piece kept, and
// leaves a row that already fits alone.
func cutRowWithEllipsis(line row, width int) row {
	if line.width <= width {
		return line
	}
	line.keepWithin(max(width-1, 0))
	ending := styleDim
	if len(line.spans) > 0 {
		last := &line.spans[len(line.spans)-1]
		ending = last.style
		trimmed := strings.TrimRight(last.text, " ")
		line.width -= displayWidth(last.text) - displayWidth(trimmed)
		last.text = trimmed
	}
	line.add(ending, string(ellipsisGlyph))
	return line
}
