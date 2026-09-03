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

	"github.com/JaredTate/coeus/internal/contract"
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
	// whole screen. Twelve is the job in section 4 of COEUS.md drawn whole.
	maxJobTasks = 12
	// jobTaskMarkColumns is what a task line spends before its label: the
	// pointer or its blank, and the mark with the blank after it.
	jobTaskMarkColumns = 6
)

const (
	// panelEdgeGlyph is the thin line down the left of the panel, which is what
	// separates it from the transcript when there is no colour at all.
	panelEdgeGlyph = '│'
	// doneStepGlyph is the check beside a step of the plan that is finished.
	doneStepGlyph = '✓'
	// toDoStepGlyph is the mark beside a step that is not finished yet.
	toDoStepGlyph = '·'
	// currentTaskGlyph is the pointer beside the task of the job that is
	// running now.
	currentTaskGlyph = '▸'
)

// The two things the panel draws that internal/contract does not name yet. The
// job the running task belongs to is not one of them: the program sends it in
// the four StatusFieldJob fields the contract names. The lines asked for are:
//
//	// StatusFieldPlan is the running task's plan: one done-when step per line,
//	// each beginning with "[x] " when the step is done and "[ ] " when it is
//	// not, so a screen can draw a check beside every step that is finished.
//	StatusFieldPlan = "plan"
//	// StatusFieldJobs is how many jobs are waiting, written as a number.
//	StatusFieldJobs = "jobs"
//
// The spellings here are the ones the program will send, so the panel fills in
// the moment those lines are in contract and the program sends them.
const (
	statusFieldPlan = "plan"
	statusFieldJobs = "jobs"
)

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
// them: which model is answering and what it has cost, the running task and how
// far through its plan it is, and the job the task belongs to or else how many
// jobs are waiting. A group the program has said nothing about is left out
// altogether, blank line and all.
func (screen *Screen) panelLines() []row {
	lines := []row{}
	for _, group := range [][]row{screen.modelPanelLines(), screen.taskPanelLines(), screen.jobPanelLines()} {
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
// session has cost so far.
func (screen *Screen) modelPanelLines() []row {
	lines := []row{}
	lines = appendPanelWords(lines, styleBold, screen.modelAlias)
	if parts := screen.contextParts(); len(parts) > 0 {
		measure := row{}
		measure.add(parts[0].style, cutTo(parts[0].text, panelTextColumns-3-displayWidth(parts[1].text)))
		measure.add(styleDim, " · ")
		measure.addSpan(parts[1])
		lines = append(lines, measure)
	}
	return appendPanelWords(lines, styleDim, screen.costWords())
}

// taskPanelLines are the running task and its plan, a check beside every step
// that is done.
func (screen *Screen) taskPanelLines() []row {
	lines := appendPanelWords(nil, screen.taskStyle(), screen.taskWords())
	steps := planSteps(screen.plan)
	shown := steps
	if len(shown) > maxPlanSteps {
		shown = shown[:maxPlanSteps]
	}
	for _, step := range shown {
		lines = append(lines, planStepLines(step)...)
	}
	if len(steps) > len(shown) {
		lines = appendPanelWords(lines, styleDim, "and "+strconv.Itoa(len(steps)-len(shown))+" more steps")
	}
	return lines
}

// jobPanelLines is the job the running task belongs to: its number, its ask,
// one line per task with a mark on every task that is done and a pointer at
// the one running now, and how far the job has got. While no job's task is
// running it is how many jobs are waiting, or nothing at all when the program
// has not said.
func (screen *Screen) jobPanelLines() []row {
	if screen.job == "" {
		return appendPanelWords(nil, styleDim, jobWords(screen.jobs))
	}
	lines := appendPanelWords(nil, styleBold, "job "+screen.job)
	lines = appendPanelWords(lines, styleDim, screen.jobAsk)
	tasks := contract.ParseJobTaskLines(screen.jobTasks)
	shown := tasks
	if len(shown) > maxJobTasks {
		shown = shown[:maxJobTasks]
	}
	for _, task := range shown {
		lines = append(lines, jobTaskLine(task, task.TaskID != "" && task.TaskID == screen.jobTask))
	}
	if len(tasks) > len(shown) {
		lines = appendPanelWords(lines, styleDim, "and "+strconv.Itoa(len(tasks)-len(shown))+" more tasks")
	}
	if len(tasks) > 0 {
		lines = appendPanelWords(lines, styleDim, tasksDoneWords(tasks))
	}
	return lines
}

// jobTaskLine draws one task of the job on one line: the pointer when it is
// the task running now, its mark, and its label and title cut to the room that
// is left, because a task list is one line per task.
func jobTaskLine(task contract.JobTask, current bool) row {
	line := row{}
	if current {
		line.add(styleAccent, string(currentTaskGlyph)+" ")
	} else {
		line.blanks(2)
	}
	words, mark := styleNormal, "[ ] "
	if task.Done {
		words, mark = styleDim, "[x] "
	}
	line.add(words, mark)
	line.add(words, cutTo(strings.TrimSpace(task.TaskID+" "+task.Text), panelTextColumns-jobTaskMarkColumns))
	return line
}

// tasksDoneWords is the job's progress in the words the design uses, "3 of 12
// tasks done", counted from the list the program sent.
func tasksDoneWords(tasks []contract.JobTask) string {
	done := 0
	for _, task := range tasks {
		if task.Done {
			done++
		}
	}
	return strconv.Itoa(done) + " of " + strconv.Itoa(len(tasks)) + " tasks done"
}

// jobWords says how many jobs are waiting in the words a person would use.
func jobWords(count string) string {
	switch count {
	case "":
		return ""
	case "1":
		return "1 job"
	default:
		return count + " jobs"
	}
}

// appendPanelWords puts one line of words in the panel, and puts nothing there
// when there are no words, because the panel says only what the program said.
func appendPanelWords(lines []row, chosen style, words string) []row {
	if words == "" {
		return lines
	}
	line := row{}
	line.add(chosen, cutTo(words, panelTextColumns))
	return append(lines, line)
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

// planStepLines draws one step of the plan: its mark, and its words wrapped
// under themselves rather than under the mark, so that the marks read as a
// column of their own.
func planStepLines(step planStep) []row {
	mark, marked := toDoStepGlyph, styleDim
	if step.done {
		mark, marked = doneStepGlyph, styleAccent
	}
	words := styleNormal
	if step.done {
		words = styleDim
	}
	lines := []row{}
	for at, wrapped := range wrapText(step.text, panelTextColumns-2) {
		line := row{}
		if at == 0 {
			line.add(marked, string(mark)+" ")
		} else {
			line.blanks(2)
		}
		line.add(words, wrapped)
		lines = append(lines, line)
	}
	return lines
}
