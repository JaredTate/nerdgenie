package tui

import (
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
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

// checklistItems is the checklist with its click targets: a header naming
// what is being worked on, a rule, one row per item, a rule, and how far the
// work has got. A job's items are its tasks with the running task's plan
// under it; a plain task's items are its plan steps. A header with no items
// under it stands alone, without rules, and no header at all is drawn while
// nothing is running.
func (screen *Screen) checklistItems() []panelLine {
	items := screen.checklistHeader()
	if len(items) == 0 {
		return nil
	}
	rows, total, done := screen.checklistRows()
	if len(rows) == 0 {
		return items
	}
	width := screen.panelTextWidth()
	items = append(items, panelLine{drawn: panelRule(width)})
	items = append(items, rows...)
	items = append(items, panelLine{drawn: panelRule(width)})
	progress := appendPanelWords(nil, styleAccent, strconv.Itoa(done)+" of "+strconv.Itoa(total)+" done", width)
	return append(items, panelLine{drawn: progress[0]})
}

// checklistHeader names what is being worked on: "JOB 4 · Tater Tots Tetris"
// while a job's task is running, with the job's ask on a dim line under it when
// the job has no name, so an older job still says what it is; "TASK 17 · Post
// a tweet a…" while a plain task runs, its ask cut with an ellipsis, or "TASK
// 17" alone until the ask has reached the screen; and nothing at all when
// nothing is running. The header is the click target for the job or the task
// it names.
func (screen *Screen) checklistHeader() []panelLine {
	width := screen.panelTextWidth()
	switch {
	case screen.job != "":
		items := []panelLine{{drawn: checklistTitle("JOB", screen.job, screen.jobName, width), target: "job:" + screen.job}}
		for _, line := range appendPanelWords(nil, styleDim, screen.jobAskWhenUnnamed(), width) {
			items = append(items, panelLine{drawn: line})
		}
		return items
	case screen.taskID != "":
		number := taskNumber(screen.taskID)
		return []panelLine{{drawn: checklistTitle("TASK", number, screen.taskAsk, width), target: "task:" + number}}
	default:
		return nil
	}
}

// jobAskWhenUnnamed is the job's ask, drawn under its header only when the
// job has no name, and nothing otherwise.
func (screen *Screen) jobAskWhenUnnamed() string {
	if screen.jobName != "" {
		return ""
	}
	return screen.jobAsk
}

// checklistTitle draws the header: the word in bold white, the number in the
// accent, and the name, when there is one, in bold white after a dim dot.
func checklistTitle(word string, number string, name string, width int) row {
	line := row{}
	line.add(styleBold, word)
	line.add(styleAccent, " "+number)
	if name != "" {
		line.add(styleDim, " · ")
		line.add(styleBold, cutWithEllipsis(name, width-line.width))
	}
	line.keepWithin(width)
	return line
}

// taskNumber is a task's number without the word in front of it, because the
// program sends "17" from one place and "task 17" from another, and the header
// writes the word itself.
func taskNumber(id string) string {
	return strings.TrimSpace(strings.TrimPrefix(id, "task"))
}

// checklistRows are the rows between the two rules, with how many items there
// are in all and how many are done. For a job they are one row per task, at
// most maxJobTasks of them, each the click target for its task, with the
// running task's plan indented under it and under no other; for a plain task
// they are its plan steps.
func (screen *Screen) checklistRows() ([]panelLine, int, int) {
	width := screen.panelTextWidth()
	if screen.job == "" {
		steps := planSteps(screen.plan)
		return withoutTargets(planStepLines(steps, 0, width)), len(steps), stepsDone(steps)
	}
	tasks := contract.ParseJobTaskLines(screen.jobTasks)
	shown := tasks
	if len(shown) > maxJobTasks {
		shown = shown[:maxJobTasks]
	}
	rows := []panelLine{}
	for _, task := range shown {
		running := task.TaskID != "" && task.TaskID == screen.jobTask
		rows = append(rows, panelLine{drawn: jobTaskLine(task, running, width), target: taskTarget(task.TaskID)})
		if running {
			rows = append(rows, withoutTargets(planStepLines(planSteps(screen.plan), stepIndent, width))...)
		}
	}
	if len(tasks) > len(shown) {
		more := appendPanelWords(nil, styleDim, "and "+strconv.Itoa(len(tasks)-len(shown))+" more tasks", width)
		rows = append(rows, panelLine{drawn: more[0]})
	}
	done := 0
	for _, task := range tasks {
		if task.Done {
			done++
		}
	}
	return rows, len(tasks), done
}

// taskTarget is the click target of a job's task row, or nothing for a task
// the program gave no id.
func taskTarget(id string) string {
	if id == "" {
		return ""
	}
	return "task:" + id
}

// withoutTargets wraps rows that are nothing to click as panel lines.
func withoutTargets(lines []row) []panelLine {
	items := make([]panelLine, len(lines))
	for at, line := range lines {
		items[at] = panelLine{drawn: line}
	}
	return items
}

// jobTaskLine draws one task of the job on one row: the glyph of its state,
// its label dim, and its words, white on the running task and dim on the rest,
// cut with an ellipsis to the panel, because a checklist is one row per item.
func jobTaskLine(task contract.JobTask, running bool, width int) row {
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
	line.add(mark.wordsStyle(), cutWithEllipsis(task.Text, width-line.width))
	return line
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
func planStepLines(steps []planStep, indent int, width int) []row {
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
		line.add(mark.wordsStyle(), cutWithEllipsis(step.text, width-line.width))
		lines = append(lines, line)
	}
	if len(steps) > len(shown) {
		line := row{}
		line.blanks(indent)
		line.add(styleDim, cutWithEllipsis("and "+strconv.Itoa(len(steps)-len(shown))+" more steps", width-indent))
		lines = append(lines, line)
	}
	return lines
}
