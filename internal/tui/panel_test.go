package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aStatusWithAPlan is what the program says about itself while a task is
// running: everything the header already reads, and the plan and the job count
// the side panel reads beside it.
func aStatusWithAPlan() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:         "opus",
		contract.StatusFieldTask:          "17",
		contract.StatusFieldTaskState:     "running",
		contract.StatusFieldTokensIn:      "6.1k",
		contract.StatusFieldTokensOut:     "0.4k",
		contract.StatusFieldCost:          "$0.04",
		contract.StatusFieldBudget:        "86 rounds, 51 min left",
		contract.StatusFieldContextTokens: "12400",
		contract.StatusFieldContextWindow: "262144",
		contract.StatusFieldState:         contract.StateThinking,
		contract.StatusFieldPlan: "[x] the product notes are read\n" +
			"[x] a draft under 280 characters is written\n" +
			"[ ] the tweet is posted",
		contract.StatusFieldJobs: "3",
	}}
}

// aStatusWithAJob is what the program says while the running task belongs to a
// job: the same status, with the job's number, its ask, which of its tasks is
// running, and its task list one task per line with a mark on every task that
// is done.
func aStatusWithAJob() contract.SocketEnvelope {
	status := aStatusWithAPlan()
	status.Fields[contract.StatusFieldJob] = "4"
	status.Fields[contract.StatusFieldJobAsk] = "Run the DigiByte anniversary campaign this month. One post a day on X, one blog piece, and a summary for me at the end."
	status.Fields[contract.StatusFieldJobTask] = "t19"
	status.Fields[contract.StatusFieldJobTasks] = "[x] t17 post the anniversary tweet\n" +
		"[ ] t19 draft the blog piece\n" +
		"[ ] t22 post for day two"
	return status
}

// aStatusWithANamedJob is the same job once the model has given it the short
// name every job now carries, which is what the panel's header shows.
func aStatusWithANamedJob() contract.SocketEnvelope {
	status := aStatusWithAJob()
	status.Fields[contract.StatusFieldJobName] = "Tater Tots Tetris"
	return status
}

// aStatusWithNoJob is what the program says once the job's tasks are over: the
// job fields sent empty, and the count of jobs waiting.
func aStatusWithNoJob() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldJob:      "",
		contract.StatusFieldJobAsk:   "",
		contract.StatusFieldJobName:  "",
		contract.StatusFieldJobTask:  "",
		contract.StatusFieldJobTasks: "",
		contract.StatusFieldJobs:     "3",
	}}
}

// panelColumnOf is the rows of the side panel alone, cut out of the frame at the
// column the panel begins in. A row of the frame that is not the panel's, such
// as the header or a rule, is left out, because the panel's rows are the ones
// carrying its edge.
func panelColumnOf(screen *Screen) []string {
	drawn := []string{}
	for _, line := range strings.Split(plainText(screen.frame()), "\n") {
		letters := []rune(line)
		if len(letters) < screen.transcriptColumns() {
			continue
		}
		beside := strings.TrimRight(string(letters[screen.transcriptColumns():]), " ")
		if !strings.HasPrefix(beside, string(panelEdgeGlyph)) {
			continue
		}
		drawn = append(drawn, beside)
	}
	return drawn
}

// checklistRowsOf is the panel's rows with the edge and the blank after it
// taken off, so that a test reads the checklist the way a person does, one row
// at a time from the top.
func checklistRowsOf(screen *Screen) []string {
	rows := []string{}
	for _, line := range panelColumnOf(screen) {
		rows = append(rows, strings.TrimPrefix(strings.TrimPrefix(line, string(panelEdgeGlyph)), " "))
	}
	return rows
}

// rowStarting is the position of the first row that begins with a piece of
// text, or minus one when there is none.
func rowStarting(rows []string, prefix string) int {
	for at, line := range rows {
		if strings.HasPrefix(line, prefix) {
			return at
		}
	}
	return -1
}

func TestTheSidePanelIsDrawnFromAHundredColumnsAndNotBelowThem(t *testing.T) {
	for _, size := range []struct {
		width  int
		panel  bool
		saying string
	}{{99, false, "ninety-nine columns"}, {100, true, "a hundred columns"}, {120, true, "a hundred and twenty columns"}} {
		screen, _ := newTestScreen(size.width, 36)
		screen.Update(linkMessage{up: true})
		send(screen, aStatusWithAPlan())

		frame := plainText(screen.frame())
		if shown := strings.Contains(frame, "the tweet is posted"); shown != size.panel {
			t.Errorf("at %s the panel is drawn: %v, and it should be: %v\n%s", size.saying, shown, size.panel, frame)
		}
	}
}

func TestThePanelIsTwentyEightColumnsWideAndLeavesTheRestToTheTranscript(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())
	typeAndSend(screen, theHaikuMessage)

	if columns := screen.width - screen.transcriptColumns(); columns != panelColumns {
		t.Errorf("the panel takes %d columns, and the design gives it %d", columns, panelColumns)
	}
	for _, line := range strings.Split(plainText(screen.frame()), "\n") {
		if !strings.Contains(line, string(personBarGlyph)) {
			continue
		}
		beside := string([]rune(line)[:screen.transcriptColumns()])
		if edge := displayWidth(strings.TrimRight(beside, " ")); edge >= screen.transcriptColumns() {
			t.Errorf("the person's bubble reaches column %d, and the transcript ends at %d: %q",
				edge, screen.transcriptColumns(), line)
		}
	}
}

func TestThePanelSaysTheModelTheContextTheCostTheTaskAndTheJobs(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{"opus", "ctx 12.4k / 262k", "5%", "6.1k in 0.4k out", "$0.04", "TASK 17", "3 jobs waiting"} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not say %q:\n%s", wanted, panel)
		}
	}
}

// TestThePanelPutsACheckBesideEveryStepThatIsDone holds that a finished step
// gets the check, and the first step that is not finished gets the pointer,
// because that is where the work stands inside the task.
func TestThePanelPutsACheckBesideEveryStepThatIsDone(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{
		string(doneGlyph) + " the product notes are",
		string(doneGlyph) + " a draft under 280",
		string(runningGlyph) + " the tweet is posted",
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not draw %q:\n%s", wanted, panel)
		}
	}
	if strings.Count(panel, string(doneGlyph)) != 2 {
		t.Errorf("the panel draws %d checks for the two steps that are done:\n%s", strings.Count(panel, string(doneGlyph)), panel)
	}
}

func TestThePanelSaysNothingTheProgramHasNotSaid(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel: "local",
		contract.StatusFieldState: contract.StateIdle,
	}})

	panel := strings.Join(panelColumnOf(screen), "\n")
	if !strings.Contains(panel, "local") {
		t.Errorf("the panel does not name the model, which is the one thing the program said:\n%s", panel)
	}
	for _, unwanted := range []string{"task", "TASK", "job", "JOB", "ctx", "done", string(ruleGlyph)} {
		if strings.Contains(panel, unwanted) {
			t.Errorf("the panel says %q about a program that never said it:\n%s", unwanted, panel)
		}
	}
}

// TestThePanelShowsTheJobTheRunningTaskBelongsTo is the checklist as the
// Tetris trial wanted it: the job's number, one row per task with the glyph of
// its state, its label and its words, the running task pointed at, and how far
// the job has got, in place of the count of jobs. A job made without a name
// still says what it is, by its ask on the line under its number.
func TestThePanelShowsTheJobTheRunningTaskBelongsTo(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAJob())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{
		"JOB 4",
		"Run the DigiByte anniver" + string(ellipsisGlyph),
		string(doneGlyph) + " t17 post the",
		string(runningGlyph) + " t19 draft the blog",
		string(plannedGlyph) + " t22 post for day two",
		"1 of 3 done",
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not draw %q:\n%s", wanted, panel)
		}
	}
	if strings.Contains(panel, "3 jobs") {
		t.Errorf("the panel counts the jobs while it is showing one:\n%s", panel)
	}
	pointed := 0
	for _, line := range checklistRowsOf(screen) {
		if strings.HasPrefix(line, string(runningGlyph)+" t") {
			pointed++
		}
	}
	if pointed != 1 {
		t.Errorf("the panel points at %d tasks, want the one that is running:\n%s", pointed, panel)
	}
}

// TestThePanelGoesBackToTheCountOfJobsWhenTheJobIsOver holds that the job
// gives way to the waiting line once the program says no job's task is running.
func TestThePanelGoesBackToTheCountOfJobsWhenTheJobIsOver(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithANamedJob())
	send(screen, aStatusWithNoJob())

	panel := strings.Join(panelColumnOf(screen), "\n")
	if !strings.Contains(panel, "3 jobs waiting") {
		t.Errorf("the panel does not count the jobs once the job is over:\n%s", panel)
	}
	for _, unwanted := range []string{"JOB 4", "Tater", "t17", "1 of 3 done"} {
		if strings.Contains(panel, unwanted) {
			t.Errorf("the panel still says %q about a job that is over:\n%s", unwanted, panel)
		}
	}
}

// TestThePanelDrawsOnlySoManyTasksOfALongJob bounds the list: a job of two
// hundred tasks is not the whole screen.
func TestThePanelDrawsOnlySoManyTasksOfALongJob(t *testing.T) {
	status := aStatusWithAJob()
	listed := []string{}
	for at := range maxJobTasks + 5 {
		listed = append(listed, "[ ] t"+strconv.Itoa(at+1)+" task number "+strconv.Itoa(at+1))
	}
	status.Fields[contract.StatusFieldJobTasks] = strings.Join(listed, "\n")
	status.Fields[contract.StatusFieldJobTask] = "t1"
	screen, _ := newTestScreen(120, 60)
	screen.Update(linkMessage{up: true})
	send(screen, status)

	panel := strings.Join(panelColumnOf(screen), "\n")
	if drawn := strings.Count(panel, "task number "); drawn != maxJobTasks {
		t.Errorf("the panel draws %d tasks of a job of %d, want %d:\n%s", drawn, maxJobTasks+5, maxJobTasks, panel)
	}
	for _, wanted := range []string{"and 5 more tasks", "0 of " + strconv.Itoa(maxJobTasks+5) + " done"} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not say %q:\n%s", wanted, panel)
		}
	}
}

func TestThePanelIsDrawnAsTheGoldenFilesHaveIt(t *testing.T) {
	quiet, _ := newTestScreen(120, 36)
	quiet.link = &recordingLink{}
	quiet.Update(linkMessage{up: true})
	send(quiet, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel: "opus",
		contract.StatusFieldState: contract.StateIdle,
	}})
	testkit.Golden(t, "panel-idle-120x36.txt", []byte(quiet.frame()))

	working, _ := newTestScreen(120, 36)
	working.link = &recordingLink{}
	working.Update(linkMessage{up: true})
	send(working, aStatusWithAPlan())
	typeAndSend(working, "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters.")
	send(working, contract.SocketEnvelope{Type: contract.SocketReply, Text: "Where I stand: the notes are read, drafting next."})
	send(working, aToolLine("▸ read memory/product.md · 2,100 characters · r3"))
	testkit.Golden(t, "panel-task-120x36.txt", []byte(working.frame()))

	inAJob, _ := newTestScreen(120, 36)
	inAJob.link = &recordingLink{}
	inAJob.Update(linkMessage{up: true})
	send(inAJob, aStatusWithANamedJob())
	send(inAJob, contract.SocketEnvelope{Type: contract.SocketReply, Text: "Where I stand: the tweet is up, drafting the blog piece next."})
	testkit.Golden(t, "panel-job-120x36.txt", []byte(inAJob.frame()))
}

// TestThePanelDrawsOnlySoManyStepsOfALongPlan bounds the plan the way the job
// list is bounded: a plan of many steps draws maxPlanSteps of them and then says
// how many more there are, so a fifty-step plan is not the whole panel.
func TestThePanelDrawsOnlySoManyStepsOfALongPlan(t *testing.T) {
	status := aStatusWithAPlan()
	listed := []string{}
	for at := range maxPlanSteps + 5 {
		listed = append(listed, "[ ] plan step "+strconv.Itoa(at+1))
	}
	status.Fields[contract.StatusFieldPlan] = strings.Join(listed, "\n")
	screen, _ := newTestScreen(120, 40)
	screen.Update(linkMessage{up: true})
	send(screen, status)

	panel := strings.Join(panelColumnOf(screen), "\n")
	if drawn := strings.Count(panel, "plan step "); drawn != maxPlanSteps {
		t.Errorf("the panel draws %d steps of a plan of %d, want %d:\n%s", drawn, maxPlanSteps+5, maxPlanSteps, panel)
	}
	if !strings.Contains(panel, "and 5 more steps") {
		t.Errorf("the panel does not say %q:\n%s", "and 5 more steps", panel)
	}
}

// TestThePanelSaysNothingWhenNoJobsAreWaiting holds that a count of zero draws no
// jobs line, the same as an empty field, because "0 jobs" is noise the program
// did not mean to say.
func TestThePanelSaysNothingWhenNoJobsAreWaiting(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel: "opus",
		contract.StatusFieldState: contract.StateIdle,
		contract.StatusFieldJobs:  "0",
	}})

	panel := strings.Join(panelColumnOf(screen), "\n")
	if strings.Contains(panel, "job") {
		t.Errorf("the panel counts zero jobs as waiting, want nothing:\n%s", panel)
	}
}

// TestThePanelDrawsNothingForEmptyPlanAndJobsFields holds the panel's promise to
// say only what the program said: a plan and a jobs count sent empty leave the
// frame exactly as it is without them.
func TestThePanelDrawsNothingForEmptyPlanAndJobsFields(t *testing.T) {
	base := map[string]string{
		contract.StatusFieldModel:     "opus",
		contract.StatusFieldTask:      "17",
		contract.StatusFieldTaskState: "running",
		contract.StatusFieldState:     contract.StateThinking,
	}
	without, _ := newTestScreen(120, 36)
	without.Update(linkMessage{up: true})
	send(without, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: base})

	empty := map[string]string{contract.StatusFieldPlan: "", contract.StatusFieldJobs: ""}
	for name, value := range base {
		empty[name] = value
	}
	withEmpty, _ := newTestScreen(120, 36)
	withEmpty.Update(linkMessage{up: true})
	send(withEmpty, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: empty})

	if without.frame() != withEmpty.frame() {
		t.Errorf("empty plan and jobs fields changed the frame:\nwithout them:\n%s\nwith them empty:\n%s",
			plainText(without.frame()), plainText(withEmpty.frame()))
	}
}

// TestThePanelShowsTheJobsNameInsteadOfItsAsk holds that a job made with a
// short name is headed "JOB N · Name" with its task list under it, and its long
// ask is not spelled out, which is what the job should look like on the side.
func TestThePanelShowsTheJobsNameInsteadOfItsAsk(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithANamedJob())

	panel := strings.Join(panelColumnOf(screen), "\n")
	if !strings.Contains(panel, "JOB 4 · Tater Tots Tetris") {
		t.Errorf("the panel does not name the job:\n%s", panel)
	}
	if strings.Contains(panel, "Run the DigiByte") {
		t.Errorf("the panel spells out the ask even though the job has a name:\n%s", panel)
	}
	for _, wanted := range []string{string(doneGlyph) + " t17 post the", "1 of 3 done"} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the named job does not draw its task list %q:\n%s", wanted, panel)
		}
	}
}

// TestTheChecklistMarksEachTaskWithTheGlyphOfItsState holds that the glyph
// carries the state on its own, so a terminal with no colour still reads the
// checklist: a check on a finished task, a pointer on the running one, a ring
// on one still planned. On a terminal with colour the check is green, the
// pointer DigiByte's own blue, and the ring dim.
func TestTheChecklistMarksEachTaskWithTheGlyphOfItsState(t *testing.T) {
	plain, _ := newTestScreen(120, 36)
	plain.Update(linkMessage{up: true})
	send(plain, aStatusWithANamedJob())
	rows := checklistRowsOf(plain)
	for _, wanted := range []string{string(doneGlyph) + " t17 ", string(runningGlyph) + " t19 ", string(plannedGlyph) + " t22 "} {
		if rowStarting(rows, wanted) < 0 {
			t.Errorf("no row of the checklist begins %q:\n%s", wanted, strings.Join(rows, "\n"))
		}
	}

	themed := newThemedScreen(120, 36)
	themed.Update(linkMessage{up: true})
	send(themed, aStatusWithANamedJob())
	frame := themed.frame()
	for _, one := range []struct {
		drawn  string
		saying string
	}{
		{themed.colors.wrap(styleDone, string(doneGlyph)+" "), "a finished task's check is green"},
		{themed.colors.wrap(styleBrand, string(runningGlyph)+" "), "the running task's pointer is DigiByte's own blue"},
		{themed.colors.wrap(styleDim, string(plannedGlyph)+" "), "a planned task's ring is dim"},
	} {
		if !strings.Contains(frame, one.drawn) {
			t.Errorf("the frame does not hold %q, and %s", one.drawn, one.saying)
		}
	}
}

// TestTheRunningTasksPlanIsIndentedUnderItAndUnderNoOtherTask holds that the
// plan steps sit two columns in under the row of the task that is running, with
// their own glyphs, and that no other task has anything under it, so a person
// sees the job's whole checklist and where inside the current task the work
// stands.
func TestTheRunningTasksPlanIsIndentedUnderItAndUnderNoOtherTask(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithANamedJob())
	rows := checklistRowsOf(screen)

	running := rowStarting(rows, string(runningGlyph)+" t19")
	if running < 0 {
		t.Fatalf("the running task is not on the checklist:\n%s", strings.Join(rows, "\n"))
	}
	under := []string{
		"  " + string(doneGlyph) + " the product notes ar",
		"  " + string(doneGlyph) + " a draft under 280",
		"  " + string(runningGlyph) + " the tweet is posted",
	}
	for at, wanted := range under {
		if running+1+at >= len(rows) || !strings.HasPrefix(rows[running+1+at], wanted) {
			t.Errorf("row %d under the running task is %q, want it to begin %q", at+1, rows[running+1+at], wanted)
		}
	}
	if next := running + 1 + len(under); next >= len(rows) || !strings.HasPrefix(rows[next], string(plannedGlyph)+" t22") {
		t.Errorf("the row after the plan is %q, and the next task follows the plan straight away", rows[next])
	}
	indented := 0
	for _, line := range rows {
		if strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
			indented++
		}
	}
	if indented != len(under) {
		t.Errorf("%d rows are indented, and only the running task's %d steps are:\n%s", indented, len(under), strings.Join(rows, "\n"))
	}

	status := aStatusWithANamedJob()
	status.Fields[contract.StatusFieldJobTask] = "t22"
	send(screen, status)
	rows = checklistRowsOf(screen)
	if at := rowStarting(rows, string(runningGlyph)+" t22"); at < 0 || !strings.HasPrefix(rows[at+1], "  "+string(doneGlyph)) {
		t.Errorf("with t22 running its plan is not under it:\n%s", strings.Join(rows, "\n"))
	}
	if at := rowStarting(rows, string(plannedGlyph)+" t19"); at < 0 || strings.HasPrefix(rows[at+1], "  ") {
		t.Errorf("t19 is no longer running and still has steps under it:\n%s", strings.Join(rows, "\n"))
	}
}

// TestAPlainTaskShowsItsPlanUnderItsHeaderWithItsProgress holds that a task
// belonging to no job is a checklist of its own: its header, a rule, its plan
// steps with their glyphs, a rule, how many steps are done, and then, after a
// blank, how many jobs are waiting.
func TestAPlainTaskShowsItsPlanUnderItsHeaderWithItsProgress(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())
	rows := checklistRowsOf(screen)

	start := rowStarting(rows, "TASK 17")
	if start < 0 {
		t.Fatalf("the task is not headed on the checklist:\n%s", strings.Join(rows, "\n"))
	}
	rule := strings.Repeat(string(ruleGlyph), panelTextColumns)
	for at, wanted := range []string{
		"TASK 17",
		rule,
		string(doneGlyph) + " the product notes are",
		string(doneGlyph) + " a draft under 280 char",
		string(runningGlyph) + " the tweet is posted",
		rule,
		"2 of 3 done",
		"",
		"3 jobs waiting",
	} {
		if start+at >= len(rows) || !strings.HasPrefix(rows[start+at], wanted) {
			t.Errorf("row %d of the checklist is %q, want it to begin %q", at+1, rows[start+at], wanted)
		}
	}
}

// TestTheChecklistCutsALongLineWithAnEllipsisAndNeverPastThePanel holds that a
// row that will not fit is cut to the panel with an ellipsis on the end, and
// that a row which fits is left whole.
func TestTheChecklistCutsALongLineWithAnEllipsisAndNeverPastThePanel(t *testing.T) {
	status := aStatusWithANamedJob()
	status.Fields[contract.StatusFieldJobTasks] = "[ ] t19 a task whose words run on far past the edge of the panel\n[ ] t22 short"
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, status)

	rows := checklistRowsOf(screen)
	long := rowStarting(rows, string(runningGlyph)+" t19")
	if long < 0 || !strings.HasSuffix(rows[long], string(ellipsisGlyph)) {
		t.Errorf("the long task row is %q, and it ends with an ellipsis", rows[long])
	}
	short := rowStarting(rows, string(plannedGlyph)+" t22")
	if short < 0 || rows[short] != string(plannedGlyph)+" t22 short" {
		t.Errorf("the short task row is %q, and a row that fits is left whole", rows[short])
	}
	for _, line := range rows {
		if displayWidth(line) > panelTextColumns {
			t.Errorf("the row %q is %d columns, and the panel's words are %d wide", line, displayWidth(line), panelTextColumns)
		}
	}
}

// TestTheChecklistHeaderAndRulesAreDrawnInThePaletteTheBriefGives pins the
// colours of the checklist's frame: the word JOB in bold white with the number
// in the accent text, the rules in DigiByte's own blue, and the progress in the
// accent text.
func TestTheChecklistHeaderAndRulesAreDrawnInThePaletteTheBriefGives(t *testing.T) {
	screen := newThemedScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithANamedJob())
	frame := screen.frame()

	colors := screen.colors
	for _, one := range []struct {
		drawn  string
		saying string
	}{
		{colors.wrap(styleBold, "JOB") + colors.wrap(styleAccent, " 4") + colors.wrap(styleDim, " · ") + colors.wrap(styleBold, "Tater Tots Tetris"), "the header is bold white with the number in the accent"},
		{colors.wrap(styleBrand, strings.Repeat(string(ruleGlyph), panelTextColumns)), "the rules are DigiByte's own blue"},
		{colors.wrap(styleAccent, "1 of 3 done"), "the progress is in the accent"},
	} {
		if !strings.Contains(frame, one.drawn) {
			t.Errorf("the frame does not hold %q, and %s", one.drawn, one.saying)
		}
	}
}
