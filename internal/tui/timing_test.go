package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The person watching a job wants to know how long it has run, how long the
// task it is on has run, and what each finished task took, without doing the
// arithmetic from the transcript. The program sends the moments; the screen
// draws the times beside the job and beside every task that has started.

// aStatusWithAJobsTimes is the named job with its moments: the job begun
// seventeen minutes before the test's clock, t17 finished in three minutes and
// twelve seconds, t19 running for four minutes, and t22 not started.
func aStatusWithAJobsTimes() contract.SocketEnvelope {
	status := aStatusWithANamedJob()
	status.Fields[contract.StatusFieldJobStarted] = startOfTest.Add(-17 * time.Minute).Format(time.RFC3339)
	status.Fields[contract.StatusFieldJobTaskTimes] = contract.JobTaskTimeLines([]contract.JobTaskTime{
		{TaskID: "t17", Started: startOfTest.Add(-16 * time.Minute), Finished: startOfTest.Add(-16*time.Minute + 3*time.Minute + 12*time.Second)},
		{TaskID: "t19", Started: startOfTest.Add(-4 * time.Minute)},
	})
	return status
}

// aScreenShowing is a linked screen that has read one status, wide enough
// that the panel takes a third of it and a whole task row with its time fits.
func aScreenShowing(status contract.SocketEnvelope) *Screen {
	screen, _ := newTestScreen(200, 40)
	screen.Update(linkMessage{up: true})
	send(screen, status)
	return screen
}

// TestTheJobRowSaysHowLongTheJobHasRun holds that the JOB row ends with the
// job's running time, live from its start to the screen's clock, and with
// nothing when the program has not said when the job began.
func TestTheJobRowSaysHowLongTheJobHasRun(t *testing.T) {
	rows := checklistRowsOf(aScreenShowing(aStatusWithAJobsTimes()))
	if at := rowStarting(rows, "JOB 4"); at < 0 || rows[at] != "JOB 4 · Tater Tots Tetris · 17m" {
		t.Errorf("the job row reads %q, want the name and then the job's running time", rows[at])
	}

	rows = checklistRowsOf(aScreenShowing(aStatusWithANamedJob()))
	if at := rowStarting(rows, "JOB 4"); at < 0 || rows[at] != "JOB 4 · Tater Tots Tetris" {
		t.Errorf("the job row reads %q with no start sent, want the name alone", rows[at])
	}
}

// TestEachTaskRowSaysWhatItTookOrHowLongItHasRun holds the three states of a
// task row: a finished task ends with what it took, to the second; the running
// task ends with how long it has run, live; a task that has not started ends
// with nothing.
func TestEachTaskRowSaysWhatItTookOrHowLongItHasRun(t *testing.T) {
	rows := checklistRowsOf(aScreenShowing(aStatusWithAJobsTimes()))
	for _, want := range []string{
		string(doneGlyph) + " t17 post the anniversary tweet · 3m 12s",
		string(runningGlyph) + " t19 draft the blog piece · 4m",
		string(plannedGlyph) + " t22 post for day two",
	} {
		found := false
		for _, line := range rows {
			if line == want {
				found = true
			}
		}
		if !found {
			t.Errorf("no row of the checklist reads %q:\n%s", want, strings.Join(rows, "\n"))
		}
	}
}

// TestTheTimeOnATaskRowSurvivesWhenTheWordsAreCut holds that a long task's
// words are what the ellipsis cuts, and the time stays whole on the row's end
// inside the panel's width.
func TestTheTimeOnATaskRowSurvivesWhenTheWordsAreCut(t *testing.T) {
	status := aStatusWithAJobsTimes()
	status.Fields[contract.StatusFieldJobTasks] = "[x] t17 a task whose words run on far past the edge of the panel\n" +
		"[ ] t19 draft the blog piece\n[ ] t22 short"
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, status)
	rows := checklistRowsOf(screen)
	long := rowStarting(rows, string(doneGlyph)+" t17")
	if long < 0 || !strings.HasSuffix(rows[long], string(ellipsisGlyph)+" · 3m 12s") {
		t.Errorf("the long finished task's row is %q, want its words cut with an ellipsis and the time whole after them", rows[long])
	}
	if job := rowStarting(rows, "JOB 4"); job < 0 || !strings.HasSuffix(rows[job], string(ellipsisGlyph)+" · 17m") {
		t.Errorf("the job row is %q on the narrow panel, want its name cut and the time whole", rows[job])
	}
	for _, line := range rows {
		if displayWidth(line) > panelTextColumns {
			t.Errorf("the row %q is %d columns, and the panel's words are %d wide", line, displayWidth(line), panelTextColumns)
		}
	}
}

// TestTheJobTimesAreReadClearedAndLeftAlone holds the three ways the two
// fields arrive: sent, they are read; not sent, the screen keeps what it has;
// sent empty, the screen forgets them.
func TestTheJobTimesAreReadClearedAndLeftAlone(t *testing.T) {
	screen := aScreenShowing(aStatusWithAJobsTimes())
	if !screen.jobStarted.Equal(startOfTest.Add(-17*time.Minute)) || len(screen.jobTaskTimes) != 2 {
		t.Fatalf("after the status the screen holds the job's start %v and %d task times, want the start sent and two", screen.jobStarted, len(screen.jobTaskTimes))
	}

	send(screen, aStatusWithANamedJob())
	if screen.jobStarted.IsZero() || len(screen.jobTaskTimes) != 2 {
		t.Errorf("a status without the fields changed them: start %v, %d task times", screen.jobStarted, len(screen.jobTaskTimes))
	}

	cleared := aStatusWithANamedJob()
	cleared.Fields[contract.StatusFieldJobStarted] = ""
	cleared.Fields[contract.StatusFieldJobTaskTimes] = ""
	send(screen, cleared)
	if !screen.jobStarted.IsZero() || len(screen.jobTaskTimes) != 0 {
		t.Errorf("a status with the fields empty left them: start %v, %d task times", screen.jobStarted, len(screen.jobTaskTimes))
	}
}
