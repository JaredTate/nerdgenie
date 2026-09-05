package contract_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestEveryStateOfAJobHasAStatusItsRecordCanPrint pins the one reading of a
// job's state as a record status, which the real job store and the fake both
// print, so that neither can drift from the other.
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
		if written := contract.RecordStatusOfJob(shape.state); written != shape.want {
			t.Errorf("a job that is %q writes the status %q, want %q", shape.state, written, shape.want)
		}
	}
}

// TestARunsNumberIsReadAsTheNumberItIsAndNothingElseIsTheOldest pins how the
// run a put-down mark carries is compared: by the number it is, with a mark
// that carries no number at all reading as older than any that does.
func TestARunsNumberIsReadAsTheNumberItIsAndNothingElseIsTheOldest(t *testing.T) {
	for _, shape := range []struct {
		run  string
		want int
	}{
		{"7", 7}, {"12", 12}, {"", 0}, {"seven", 0}, {"-3", -3},
	} {
		if read := contract.RunNumberOf(shape.run); read != shape.want {
			t.Errorf("the run %q reads as %d, want %d", shape.run, read, shape.want)
		}
	}
}
