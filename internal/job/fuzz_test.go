package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/job"
)

// FuzzTheScheduleReader throws any schedule at the reader and the plain-words
// printer. A schedule comes from a model, so its cron expression and its time
// zone are outside text: whatever they say, the answer is an error or a
// sentence, and never a panic.
func FuzzTheScheduleReader(theFuzzer *testing.F) {
	theFuzzer.Add("cron", "0 7 * * 1-5", "America/New_York", int64(0), int64(0))
	theFuzzer.Add("every", "", "", int64(time.Hour), int64(0))
	theFuzzer.Add("at", "", "UTC", int64(0), int64(1_800_000_000))
	theFuzzer.Add("cron", "@every 1h", "", int64(0), int64(0))
	theFuzzer.Add("cron", "*/0 * * * *", "", int64(0), int64(0))
	theFuzzer.Add("", "-- -- -- -- --", "Mars/Olympus", int64(-1), int64(-1))

	theFuzzer.Fuzz(func(t *testing.T, kind string, expression string, timezone string, every int64, at int64) {
		schedule := contract.Schedule{
			Kind:     contract.ScheduleKind(kind),
			Cron:     expression,
			Timezone: timezone,
			Every:    time.Duration(every),
			At:       time.Unix(at, 0).UTC(),
		}
		said := job.InPlainWords(schedule)
		if said == "" {
			t.Errorf("the schedule %+v was put into no words at all", schedule)
		}
		if strings.Contains(said, "\n") {
			t.Errorf("the schedule %+v was put into more than one line: %q", schedule, said)
		}
	})
}
