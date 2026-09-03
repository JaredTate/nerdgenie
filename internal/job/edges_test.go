package job

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestADayOfTheMonthIsWrittenTheWayAPersonWritesIt(t *testing.T) {
	for _, shape := range []struct {
		day  int
		want string
	}{
		{1, "1st"}, {2, "2nd"}, {3, "3rd"}, {4, "4th"}, {11, "11th"}, {12, "12th"},
		{13, "13th"}, {21, "21st"}, {22, "22nd"}, {23, "23rd"}, {30, "30th"},
	} {
		if written := ordinal(shape.day); written != shape.want {
			t.Errorf("day %d is written %q, want %q", shape.day, written, shape.want)
		}
	}
}

func TestEveryStateOfAJobHasAStatusItsRecordCanPrint(t *testing.T) {
	for _, shape := range []struct {
		state contract.JobState
		want  contract.RecordStatus
	}{
		{contract.JobRunning, contract.StatusRunning},
		{contract.JobPaused, contract.StatusWaiting},
		{contract.JobOff, contract.StatusStopped},
		{contract.JobDone, contract.StatusDone},
		{contract.JobState("something else"), contract.StatusRunning},
	} {
		if written := recordStatusOfJob(shape.state); written != shape.want {
			t.Errorf("a job that is %q writes the status %q, want %q", shape.state, written, shape.want)
		}
	}
}

func TestAMoreUnusualCronExpressionStillPrintsSomethingTrue(t *testing.T) {
	for _, shape := range []struct {
		expression string
		want       string
	}{
		{"0 9 * * sun,mon", "every Sunday and Monday at 9 in the morning"},
		{"0 9 * * mon-fri", "every weekday at 9 in the morning"},
		{"0 9 * * sat,sun", "every Saturday and Sunday at 9 in the morning"},
		{"0 9 * * ?", "every day at 9 in the morning"},
		{"@weekly", "every Sunday at midnight"},
		{"0 9 * * 1-3", `on the cron schedule "0 9 * * 1-3"`},
		{"0 9 1-7 * 1", `on the cron schedule "0 9 1-7 * 1"`},
		{"0 9 3 6 *", `on the cron schedule "0 9 3 6 *"`},
		{"0 9 * * *  and more", `on the cron schedule "0 9 * * *  and more"`},
		{"*/0 * * * *", `on the cron schedule "*/0 * * * *"`},
		{"*/5 3 * * *", `on the cron schedule "*/5 3 * * *"`},
		{"* */2 * * *", `on the cron schedule "* */2 * * *"`},
		{"0 9 * * 1,notaday", `on the cron schedule "0 9 * * 1,notaday"`},
	} {
		if said := cronInWords(shape.expression); said != shape.want {
			t.Errorf("the expression %q reads as %q, want %q", shape.expression, said, shape.want)
		}
	}
}

func TestATimeOfDayThatIsNoTimeOfDayIsPrintedAsTheNumbersItIs(t *testing.T) {
	if said := timeOfDayInWords(25, 0); said != "25:00" {
		t.Errorf("an hour of 25 reads as %q, want the numbers themselves", said)
	}
	if said := timeOfDayInWords(9, 99); said != "09:99" {
		t.Errorf("a minute of 99 reads as %q, want the numbers themselves", said)
	}
}

func TestAnEmptyListOfNamesReadsAsNothing(t *testing.T) {
	if said := listInWords(nil); said != "" {
		t.Errorf("no names at all read as %q", said)
	}
	if said := listInWords([]string{"Monday", "Wednesday", "Friday"}); said != "Monday, Wednesday and Friday" {
		t.Errorf("three names read as %q", said)
	}
}

func TestADateWithNoMomentBehindItIsNoDateAtAll(t *testing.T) {
	if said := dueInPlainWords(time.Time{}); said != "" {
		t.Errorf("a task with no date carries the date %q", said)
	}
	written := dueInPlainWords(time.Date(2026, 9, 3, 14, 0, 0, 0, time.UTC))
	if strings.Contains(written, ", ") {
		t.Errorf("a date reads as %q, and a comma is where a task's line splits it off", written)
	}
}

func TestTwoFailuresWithTheSameLongBeginningAreOneIncident(t *testing.T) {
	first := strings.Repeat("the same beginning ", 20) + "and one ending"
	second := strings.Repeat("the same beginning ", 20) + "and another"
	now := time.Unix(0, 0).UTC()

	counted, isNew := noteIncident(nil, first, now)
	if !isNew || len(counted) != 1 {
		t.Fatalf("the first failure made %d incidents (new %v)", len(counted), isNew)
	}
	counted, isNew = noteIncident(counted, second, now.Add(time.Hour))
	if isNew || len(counted) != 1 || counted[0].Count != 2 {
		t.Errorf("two failures with the same long beginning made %+v (new %v)", counted, isNew)
	}
}

func TestANoteLongerThanTheWholeNotepadIsCutToFitIt(t *testing.T) {
	kept := appendToNotepad("", strings.Repeat("x", NotepadBytes*2))

	if len(kept) != NotepadBytes {
		t.Errorf("one very long note left %d bytes on the notepad, and the cap is %d", len(kept), NotepadBytes)
	}
}

func TestACommandThatIsOnlyAWordInFrontOfOneIsNoCommand(t *testing.T) {
	for _, harmless := range []string{"sudo", "  ", ";;;", "env NAME=value"} {
		if err := checkItCannotRestartTheAgent(harmless); err != nil {
			t.Errorf("the text %q was refused as work that restarts the agent: %v", harmless, err)
		}
	}
}

func TestAReportThatTheJobNeverWroteProvesNothing(t *testing.T) {
	if recordHoldsReport(&heldJob{}, "") {
		t.Error("an empty report identifier was taken as one the job wrote")
	}
}
