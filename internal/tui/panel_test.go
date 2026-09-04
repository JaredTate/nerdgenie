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

// aStatusWithNoJob is what the program says once the job's tasks are over: the
// job fields sent empty, and the count of jobs waiting.
func aStatusWithNoJob() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldJob:      "",
		contract.StatusFieldJobAsk:   "",
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
	for _, wanted := range []string{"opus", "ctx 12.4k / 262k", "5%", "6.1k in 0.4k out", "$0.04", "task 17 running", "3 jobs"} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not say %q:\n%s", wanted, panel)
		}
	}
}

func TestThePanelPutsACheckBesideEveryStepThatIsDone(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{
		string(doneStepGlyph) + " the product notes are",
		string(doneStepGlyph) + " a draft under 280",
		string(toDoStepGlyph) + " the tweet is posted",
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not draw %q:\n%s", wanted, panel)
		}
	}
	if strings.Count(panel, string(doneStepGlyph)) != 2 {
		t.Errorf("the panel draws %d checks for the two steps that are done:\n%s", strings.Count(panel, string(doneStepGlyph)), panel)
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
	for _, unwanted := range []string{"task", "job", "ctx"} {
		if strings.Contains(panel, unwanted) {
			t.Errorf("the panel says %q about a program that never said it:\n%s", unwanted, panel)
		}
	}
}

// TestThePanelShowsTheJobTheRunningTaskBelongsTo is the third group of the
// panel as the Tetris trial wanted it: the job's number and its ask, one line
// per task with its mark, its label and its title, the running task pointed
// at, and how far the job has got, in place of the count of jobs.
func TestThePanelShowsTheJobTheRunningTaskBelongsTo(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAJob())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{
		"job 4",
		"Run the DigiByte annivers",
		"  [x] t17 post the annive",
		string(currentTaskGlyph) + " [ ] t19 draft the blog",
		"  [ ] t22 post for day tw",
		"1 of 3 tasks done",
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not draw %q:\n%s", wanted, panel)
		}
	}
	if strings.Contains(panel, "3 jobs") {
		t.Errorf("the panel counts the jobs while it is showing one:\n%s", panel)
	}
	if strings.Count(panel, string(currentTaskGlyph)) != 1 {
		t.Errorf("the panel points at %d tasks, want the one that is running:\n%s", strings.Count(panel, string(currentTaskGlyph)), panel)
	}
}

// TestThePanelGoesBackToTheCountOfJobsWhenTheJobIsOver holds that the job
// gives way to today's line once the program says no job's task is running.
func TestThePanelGoesBackToTheCountOfJobsWhenTheJobIsOver(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAJob())
	send(screen, aStatusWithNoJob())

	panel := strings.Join(panelColumnOf(screen), "\n")
	if !strings.Contains(panel, "3 jobs") {
		t.Errorf("the panel does not count the jobs once the job is over:\n%s", panel)
	}
	for _, unwanted := range []string{"job 4", "t17", "tasks done"} {
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
	if drawn := strings.Count(panel, "] t"); drawn != maxJobTasks {
		t.Errorf("the panel draws %d tasks of a job of %d, want %d:\n%s", drawn, maxJobTasks+5, maxJobTasks, panel)
	}
	for _, wanted := range []string{"and 5 more tasks", "0 of " + strconv.Itoa(maxJobTasks+5) + " tasks done"} {
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
	send(inAJob, aStatusWithAJob())
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
