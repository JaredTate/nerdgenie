package replay_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/replay"
)

// The bounds this package promises, written out as the numbers they are. A
// bound asserted against the constant that holds it is not pinned at all,
// because the constant and the test move together and nothing fails. These are
// the literals, so changing one of them fails here and the change is deliberate.
func TestEveryBoundOfAReplayIsTheNumberItIsMeantToBe(t *testing.T) {
	bounds := []struct {
		name string
		held int
		want int
	}{
		{"the questions one night asks the memory", replay.TwentyQuestions, 20},
		{"how far down a search a fact may be and still count as found", replay.AnswersLookedAt, 3},
		{"the skills one night dry runs", replay.MaxSkillsChecked, 200},
		{"how much of a fact is used as the question about it", replay.MaxQuestionRunes, 200},
		{"the failures the one line names before it counts the rest", replay.MaxFailuresNamed, 5},
		{"the notes about what could not be checked the one line carries", replay.MaxNotesKept, 2},
		{"the model replies one recording may hold", replay.MaxRoundsRead, 2000},
		{"the tool calls one recording may hold", replay.MaxCallsRead, 20000},
		{"the window the replayed model reports", replay.RecordedContextLength, 200000},
	}
	for _, bound := range bounds {
		if bound.held != bound.want {
			t.Errorf("%s is %d, want %d; if the change is meant, change this test with it",
				bound.name, bound.held, bound.want)
		}
	}
}

// TestTheNightlySelfCheckRunsAnHourAfterTheBackupTimer holds the schedule to the
// reason it was chosen rather than to its own spelling: four in the morning is
// four because the backup runs at three, and the two must never run over each
// other. If either hour moves, this is where it is noticed.
func TestTheNightlySelfCheckRunsAnHourAfterTheBackupTimer(t *testing.T) {
	if replay.NightlyCron != "0 4 * * *" {
		t.Errorf("the nightly self-check is scheduled %q, want \"0 4 * * *\"; if the change is meant, change this test with it", replay.NightlyCron)
	}
	fields := strings.Fields(replay.NightlyCron)
	if len(fields) != 5 {
		t.Fatalf("the nightly schedule %q is not five cron fields, so nobody can say what hour it runs at", replay.NightlyCron)
	}
	checkHour, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("the hour of the nightly schedule %q cannot be read: %v", replay.NightlyCron, err)
	}

	backupHour := hourOfTheBackupTimer(t)

	if checkHour != backupHour+1 {
		t.Errorf("the self-check runs at %02d:00 and the backup timer at %02d:00, and the self-check runs an hour after the backup so that the two never run over each other",
			checkHour, backupHour)
	}
}

// hourOfTheBackupTimer is the hour the installed backup timer fires at, read out
// of the unit "nerdgenie install" writes.
func hourOfTheBackupTimer(t *testing.T) int {
	t.Helper()
	const marker = "OnCalendar="
	for _, line := range strings.Split(command.BackupTimerText(contract.NewHome("/home/somebody/.nerdgenie")), "\n") {
		if !strings.HasPrefix(line, marker) {
			continue
		}
		when := strings.Fields(strings.TrimPrefix(line, marker))
		if len(when) != 2 {
			t.Fatalf("the backup timer says %q, which is not a date and a time", line)
		}
		hour, err := strconv.Atoi(strings.Split(when[1], ":")[0])
		if err != nil {
			t.Fatalf("the hour of the backup timer %q cannot be read: %v", line, err)
		}
		return hour
	}
	t.Fatalf("the backup timer names no time at all, so there is nothing for the self-check to run an hour after")
	return 0
}
