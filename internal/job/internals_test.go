package job

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// FuzzTheRestartGuard throws any text at the guard that refuses work which would
// restart the agent. A task's text is written by a model, so it is outside text:
// whatever it says, the guard answers with a yes or a no and never a panic, and
// a command that begins with the word reboot is always refused.
func FuzzTheRestartGuard(theFuzzer *testing.F) {
	theFuzzer.Add("post the anniversary tweet")
	theFuzzer.Add("sudo systemctl restart coeus")
	theFuzzer.Add("tidy the logs; reboot")
	theFuzzer.Add(`run ["killall", "coeus"]`)
	theFuzzer.Add("|;&&\\\n\x00")

	theFuzzer.Fuzz(func(t *testing.T, text string) {
		err := checkItCannotRestartTheAgent(text)
		leading := strings.ToLower(strings.TrimSpace(text))
		if err == nil && (leading == "reboot" || strings.HasPrefix(leading, "reboot ")) {
			t.Errorf("a command that begins with the word reboot was accepted: %q", text)
		}
	})
}

// FuzzTheJobLogKeyReader throws any key at the reader that decides which of a
// log's events belong to a job. The key comes out of the database, so it is
// outside text too.
func FuzzTheJobLogKeyReader(theFuzzer *testing.F) {
	theFuzzer.Add("j4")
	theFuzzer.Add("17")
	theFuzzer.Add("j")
	theFuzzer.Add("j0")
	theFuzzer.Add("j-1")
	theFuzzer.Add("j99999999999999999999999")

	theFuzzer.Fuzz(func(t *testing.T, logKey string) {
		jobID, itIsAJob := jobIDOfLogKey(logKey)
		if !itIsAJob {
			return
		}
		if contract.RecordLogKey(contract.RecordJob, jobID) != logKey {
			t.Errorf("the key %q was read as job %q, which is logged under %q instead",
				logKey, jobID, contract.RecordLogKey(contract.RecordJob, jobID))
		}
	})
}

func TestABackoffClimbsThroughTheTableAndStaysAtTheTop(t *testing.T) {
	if waiting := nextBackoff(0); waiting != 0 {
		t.Errorf("a job with no failures waits %s, want none at all", waiting)
	}
	if waiting := nextBackoff(1); waiting != 30*time.Second {
		t.Errorf("the first failure waits %s, want thirty seconds", waiting)
	}
	if waiting := nextBackoff(len(backoffSteps)); waiting != time.Hour {
		t.Errorf("the last step of the table waits %s, want an hour", waiting)
	}
	if waiting := nextBackoff(500); waiting != time.Hour {
		t.Errorf("five hundred failures wait %s, want the hour the table stops at", waiting)
	}
}

func TestAOneOffScheduleThatHasRunHasNoMomentLeft(t *testing.T) {
	once := contract.Schedule{Kind: contract.ScheduleAt, At: time.Unix(1000, 0).UTC()}

	moment, err := nextRun(once, time.Unix(2000, 0).UTC())

	if err != nil {
		t.Fatalf("reading a one-off schedule that has run failed: %v", err)
	}
	if !moment.IsZero() {
		t.Errorf("a one-off schedule that has run says it runs again at %s", moment)
	}
	pushed, waited, err := afterAFailedTick(once, time.Unix(2000, 0).UTC(), 1)
	if err != nil {
		t.Fatalf("pushing a one-off schedule back after a failure failed: %v", err)
	}
	if !pushed.IsZero() || waited != 30*time.Second {
		t.Errorf("a one-off schedule that has run was pushed back to %s after waiting %s", pushed, waited)
	}
}

func TestReadingASchedulePlainlyRefusesWhatItCannotWork(t *testing.T) {
	for _, wrong := range []contract.Schedule{
		{Kind: "sometimes"},
		{Kind: contract.ScheduleEvery, Every: time.Second},
		{Kind: contract.ScheduleCron, Cron: "not a cron expression"},
		{Kind: contract.ScheduleCron, Cron: "0 7 * * *", Timezone: "Mars/Olympus"},
	} {
		if _, err := nextRun(wrong, time.Unix(0, 0).UTC()); err == nil {
			t.Errorf("the schedule %+v was read as one that fires", wrong)
		}
	}
}

func TestAStampComparesAsTextTheWayTheMomentsCompare(t *testing.T) {
	earlier := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	later := earlier.Add(500 * time.Millisecond)

	if stamp(earlier) >= stamp(later) {
		t.Errorf("the stamp %q does not sort before %q, and the claims table compares them as text",
			stamp(earlier), stamp(later))
	}
	if len(stamp(earlier)) != len(stamp(later)) {
		t.Errorf("the stamps %q and %q are different lengths, so text will not compare them",
			stamp(earlier), stamp(later))
	}
}

func TestAJobIdentifierIsAWholeNumberAndNothingElse(t *testing.T) {
	for _, wrong := range []string{"", "0", "-1", "01", "4.2", "four", " 4"} {
		if _, valid := jobNumber(wrong); valid {
			t.Errorf("%q was read as a job number", wrong)
		}
	}
	if number, valid := jobNumber("42"); !valid || number != 42 {
		t.Errorf("the job number 42 was read as %d (valid %v)", number, valid)
	}
}

func TestOneLineAndSafeLineTakeOutWhatARecordsOwnLinesAreMadeOf(t *testing.T) {
	folded := safeLine("the first thing -> the second\nwith a break. Cause: not really")

	if strings.ContainsAny(folded, "\n") {
		t.Errorf("a report was folded to %q, and it still holds a line break", folded)
	}
	for _, mark := range []string{" -> ", ". Cause: "} {
		if strings.Contains(folded, mark) {
			t.Errorf("a report was folded to %q, and it still holds %q", folded, mark)
		}
	}
}
