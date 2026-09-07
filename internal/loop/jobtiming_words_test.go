package loop

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheTookLineNeedsBothMoments: a task's report says what the task took
// only when the store has both its moments; a job from a log written before
// the moments were kept, or a task not on the timing at all, adds nothing.
func TestTheTookLineNeedsBothMoments(t *testing.T) {
	start := time.Date(2026, 9, 7, 17, 0, 0, 0, time.UTC)
	timing := contract.JobTiming{Tasks: map[string]contract.TaskTiming{
		"t1": {Started: start, Finished: start.Add(3*time.Minute + 12*time.Second)},
		"t2": {Started: start},
	}}
	for _, shape := range []struct{ task, want string }{
		{"t1", " Task t1 took 3m 12s."},
		{"t2", ""},
		{"t3", ""},
	} {
		if got := theTookLine(timing, shape.task); got != shape.want {
			t.Errorf("the took line of %s reads %q, want %q", shape.task, got, shape.want)
		}
	}
}

// TestTheJobsSpanRunsFromItsStartToItsFinishOrToNow: the closing line's span
// is from the job's start to its finish, or to now when the store has not
// closed it yet, and nothing when the start is unknown.
func TestTheJobsSpanRunsFromItsStartToItsFinishOrToNow(t *testing.T) {
	start := time.Date(2026, 9, 7, 17, 0, 0, 0, time.UTC)
	now := start.Add(50 * time.Minute)
	for _, shape := range []struct {
		name   string
		timing contract.JobTiming
		want   string
	}{
		{"finished", contract.JobTiming{Started: start, Finished: start.Add(42*time.Minute + 3*time.Second)}, ", in 42m 3s"},
		{"still open", contract.JobTiming{Started: start}, ", in 50m"},
		{"unknown", contract.JobTiming{}, ""},
	} {
		if got := theJobsSpan(shape.timing, now); got != shape.want {
			t.Errorf("%s: the job's span reads %q, want %q", shape.name, got, shape.want)
		}
	}
}
