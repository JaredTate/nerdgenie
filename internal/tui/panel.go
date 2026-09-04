// The side panel is opencode's sidebar, whose drawing was read at
// ~/Code/opencode/packages/tui/src/routes/session/sidebar.tsx: a column of a
// fixed width down the right-hand side of the conversation, holding short quiet
// lines in groups with a blank line between them, the loudest line at the top of
// each group. Nothing was copied: that file is TypeScript drawing through a
// layout engine, and this is Go writing rows of its own.

package tui

import (
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// panelColumns is how wide the side panel is, its edge and its blanks and
	// all.
	panelColumns = 28
	// panelFrom is the width of terminal at which the panel appears. Below it
	// there is no room for a panel and a transcript wide enough to read, so the
	// panel is dropped and nothing else changes.
	panelFrom = 100
	// panelTextColumns is how many columns the panel's words are drawn in: its
	// width, less the edge, the blank after it, and the blank down its right.
	panelTextColumns = panelColumns - 3
	// maxPlanSteps is how many steps of a plan the panel draws before it says
	// how many more there are, so that a plan of fifty steps is not the whole
	// screen.
	maxPlanSteps = 8
	// maxJobTasks is how many tasks of a job the panel draws before it says
	// how many more there are, so that a job of two hundred tasks is not the
	// whole screen. Twelve is the job in section 4 of NERDGENIE.md drawn whole.
	maxJobTasks = 12
	// stepIndent is how far the running task's plan is drawn in under it, so
	// that the steps read as part of that task and of no other.
	stepIndent = 2
)

const (
	// panelEdgeGlyph is the thin line down the left of the panel, which is what
	// separates it from the transcript when there is no colour at all.
	panelEdgeGlyph = '│'
	// doneGlyph is the check beside a task or a step that is finished.
	doneGlyph = '✓'
	// runningGlyph is the pointer beside the task that is running now, and
	// beside the step of its plan the work is at.
	runningGlyph = '▶'
	// plannedGlyph is the ring beside a task or a step still to come.
	plannedGlyph = '○'
	// ellipsisGlyph ends a row that was cut to fit the panel.
	ellipsisGlyph = '…'
)

// checkMark is the state of one item on the checklist. One glyph carries it on
// its own, so that a terminal with no colour still reads the list.
type checkMark int

const (
	// markPlanned is an item still to come.
	markPlanned checkMark = iota
	// markRunning is the item the work is at.
	markRunning
	// markDone is an item that is finished.
	markDone
)

// span is the glyph of a mark and the blank after it, in the colour the mark
// is drawn in: green for a check, DigiByte blue for the pointer, dim for a
// ring.
func (mark checkMark) span() span {
	switch mark {
	case markDone:
		return span{style: styleDone, text: string(doneGlyph) + " "}
	case markRunning:
		return span{style: styleBrand, text: string(runningGlyph) + " "}
	default:
		return span{style: styleDim, text: string(plannedGlyph) + " "}
	}
}

// wordsStyle is how the words after a mark are drawn: white on the item the
// work is at, so the eye lands there, and dim on every other.
func (mark checkMark) wordsStyle() style {
	if mark == markRunning {
		return styleNormal
	}
	return styleDim
}

// showsPanel says whether the terminal is wide enough for the side panel.
func (screen *Screen) showsPanel() bool {
	return screen.width >= panelFrom
}

// transcriptColumns is how many columns the transcript and everything in it is
// drawn in: the whole frame, less the side panel when there is one.
func (screen *Screen) transcriptColumns() int {
	if screen.showsPanel() {
		return screen.width - panelColumns
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

// panelLines is what the panel says, in three groups with a blank line between
// them: the model and what it has cost, the checklist of what is being worked
// on, and how many jobs are waiting. A group the program has said nothing
// about is left out altogether, blank line and all.
func (screen *Screen) panelLines() []row {
	lines := []row{}
	for _, group := range [][]row{screen.modelPanelLines(), screen.checklistLines(), screen.waitingPanelLines()} {
		if len(group) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, row{})
		}
		lines = append(lines, group...)
	}
	return lines
}

// modelPanelLines are the model in use, how full its context is, and what this
// session has cost so far, kept quiet: one dim line each, with no box around
// them. The context share keeps the header's own colour, so it turns the accent
// and then bold white at the same places.
func (screen *Screen) modelPanelLines() []row {
	lines := appendPanelWords(nil, styleDim, screen.modelAlias)
	if parts := screen.contextParts(); len(parts) > 0 {
		measure := row{}
		measure.add(styleDim, cutTo(parts[0].text, panelTextColumns-3-displayWidth(parts[1].text)))
		measure.add(styleDim, " · ")
		measure.addSpan(parts[1])
		lines = append(lines, measure)
	}
	return appendPanelWords(lines, styleDim, screen.costWords())
}

// checklistLines is the checklist: a header naming what is being worked on, a
// rule, one row per item, a rule, and how far the work has got. A job's items
// are its tasks with the running task's plan under it; a plain task's items
// are its plan steps. A header with no items under it stands alone, without
// rules, and no header at all is drawn while nothing is running.
func (screen *Screen) checklistLines() []row {
	lines := screen.checklistHeader()
	if len(lines) == 0 {
		return nil
	}
	items, total, done := screen.checklistItems()
	if len(items) == 0 {
		return lines
	}
	lines = append(lines, panelRule())
	lines = append(lines, items...)
	lines = append(lines, panelRule())
	return appendPanelWords(lines, styleAccent, strconv.Itoa(done)+" of "+strconv.Itoa(total)+" done")
}

// checklistHeader names what is being worked on: "JOB 4 · Tater Tots Tetris"
// while a job's task is running, with the job's ask on a dim line under it when
// the job has no name, so an older job still says what it is; "TASK 17" while a
// plain task runs; and nothing at all when nothing is running.
func (screen *Screen) checklistHeader() []row {
	switch {
	case screen.job != "":
		lines := []row{checklistTitle("JOB", screen.job, screen.jobName)}
		if screen.jobName == "" {
			lines = appendPanelWords(lines, styleDim, screen.jobAsk)
		}
		return lines
	case screen.taskID != "":
		return []row{checklistTitle("TASK", taskNumber(screen.taskID), "")}
	default:
		return nil
	}
}

// checklistTitle draws the header: the word in bold white, the number in the
// accent, and the name, when there is one, in bold white after a dim dot.
func checklistTitle(word string, number string, name string) row {
	line := row{}
	line.add(styleBold, word)
	line.add(styleAccent, " "+number)
	if name != "" {
		line.add(styleDim, " · ")
		line.add(styleBold, cutWithEllipsis(name, panelTextColumns-line.width))
	}
	line.keepWithin(panelTextColumns)
	return line
}

// taskNumber is a task's number without the word in front of it, because the
// program sends "17" from one place and "task 17" from another, and the header
// writes the word itself.
func taskNumber(id string) string {
	return strings.TrimSpace(strings.TrimPrefix(id, "task"))
}

// checklistItems are the rows between the two rules, with how many items there
// are in all and how many are done. For a job they are one row per task, at
// most maxJobTasks of them, with the running task's plan indented under it and
// under no other; for a plain task they are its plan steps.
func (screen *Screen) checklistItems() ([]row, int, int) {
	if screen.job == "" {
		steps := planSteps(screen.plan)
		return planStepLines(steps, 0), len(steps), stepsDone(steps)
	}
	tasks := contract.ParseJobTaskLines(screen.jobTasks)
	shown := tasks
	if len(shown) > maxJobTasks {
		shown = shown[:maxJobTasks]
	}
	lines := []row{}
	for _, task := range shown {
		running := task.TaskID != "" && task.TaskID == screen.jobTask
		lines = append(lines, jobTaskLine(task, running))
		if running {
			lines = append(lines, planStepLines(planSteps(screen.plan), stepIndent)...)
		}
	}
	if len(tasks) > len(shown) {
		lines = appendPanelWords(lines, styleDim, "and "+strconv.Itoa(len(tasks)-len(shown))+" more tasks")
	}
	done := 0
	for _, task := range tasks {
		if task.Done {
			done++
		}
	}
	return lines, len(tasks), done
}

// jobTaskLine draws one task of the job on one row: the glyph of its state,
// its label dim, and its words, white on the running task and dim on the rest,
// cut with an ellipsis to the panel, because a checklist is one row per item.
func jobTaskLine(task contract.JobTask, running bool) row {
	mark := markPlanned
	switch {
	case task.Done:
		mark = markDone
	case running:
		mark = markRunning
	}
	line := row{}
	line.addSpan(mark.span())
	if task.TaskID != "" {
		line.add(styleDim, task.TaskID+" ")
	}
	line.add(mark.wordsStyle(), cutWithEllipsis(task.Text, panelTextColumns-line.width))
	return line
}

// panelRule is the thin line above and below the checklist's rows, in
// DigiByte's own blue.
func panelRule() row {
	line := row{}
	line.add(styleBrand, strings.Repeat(string(ruleGlyph), panelTextColumns))
	return line
}

// jobWords says how many jobs are waiting in the words a person would use. A
// count that is empty or zero is nothing at all, because "0 jobs" is noise the
// program did not mean to say.
func jobWords(count string) string {
	switch count {
	case "", "0":
		return ""
	case "1":
		return "1 job waiting"
	default:
		return count + " jobs waiting"
	}
}

// waitingPanelLines is how many jobs are waiting, dim, while no job is on the
// checklist, or nothing at all when the program has not said.
func (screen *Screen) waitingPanelLines() []row {
	if screen.job != "" {
		return nil
	}
	return appendPanelWords(nil, styleDim, jobWords(screen.jobs))
}

// appendPanelWords puts one line of words in the panel, cut to it with an
// ellipsis, and puts nothing there when there are no words, because the panel
// says only what the program said.
func appendPanelWords(lines []row, chosen style, words string) []row {
	if words == "" {
		return lines
	}
	line := row{}
	line.add(chosen, cutWithEllipsis(words, panelTextColumns))
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

// planStep is one step of the running task's plan: its words, and whether it is
// done.
type planStep struct {
	// done is whether the step is finished.
	done bool
	// text is the step in the record's own words.
	text string
}

// planSteps reads the plan the program sent: one step per line, each beginning
// with "[x] " when the step is done and "[ ] " when it is not. A line in any
// other shape is read as a step that is not done, because a plan the screen
// cannot quite parse is still a plan worth showing.
func planSteps(plan string) []planStep {
	steps := []planStep{}
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rest, marked := strings.CutPrefix(line, "[x] "); marked {
			steps = append(steps, planStep{done: true, text: rest})
			continue
		}
		steps = append(steps, planStep{text: strings.TrimPrefix(line, "[ ] ")})
	}
	return steps
}

// stepsDone counts the steps of a plan that are finished.
func stepsDone(steps []planStep) int {
	done := 0
	for _, step := range steps {
		if step.done {
			done++
		}
	}
	return done
}

// planStepLines draws the steps of a plan, each on one row indented by a number
// of columns: a check on every finished step, the pointer on the first that is
// not, because that is where the work stands, and a ring on the rest. At most
// maxPlanSteps are drawn, and a plan with more says how many more there are.
func planStepLines(steps []planStep, indent int) []row {
	shown := steps
	if len(shown) > maxPlanSteps {
		shown = shown[:maxPlanSteps]
	}
	lines := []row{}
	pointed := false
	for _, step := range shown {
		mark := markPlanned
		switch {
		case step.done:
			mark = markDone
		case !pointed:
			mark, pointed = markRunning, true
		}
		line := row{}
		line.blanks(indent)
		line.addSpan(mark.span())
		line.add(mark.wordsStyle(), cutWithEllipsis(step.text, panelTextColumns-line.width))
		lines = append(lines, line)
	}
	if len(steps) > len(shown) {
		line := row{}
		line.blanks(indent)
		line.add(styleDim, cutWithEllipsis("and "+strconv.Itoa(len(steps)-len(shown))+" more steps", panelTextColumns-indent))
		lines = append(lines, line)
	}
	return lines
}
