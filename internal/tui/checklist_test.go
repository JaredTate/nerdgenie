package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

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
