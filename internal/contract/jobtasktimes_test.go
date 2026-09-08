package contract_test

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheTimingStatusFieldsAreNamed pins the two fields that carry when a job
// and its tasks began and ended, so that the program and the screen spell
// them the same.
func TestTheTimingStatusFieldsAreNamed(t *testing.T) {
	for field, want := range map[string]string{
		contract.StatusFieldJobStarted:   "jobStarted",
		contract.StatusFieldJobTaskTimes: "jobTaskTimes",
	} {
		if field != want {
			t.Errorf("status field is %q, want %q", field, want)
		}
	}
}

// theCampaignTimes are the moments of the campaign's tasks: two finished, one
// running, and one that has not started and so has no line.
func theCampaignTimes() []contract.JobTaskTime {
	start := time.Date(2026, 9, 7, 23, 26, 10, 0, time.UTC)
	return []contract.JobTaskTime{
		{TaskID: "t17", Started: start, Finished: start.Add(3*time.Minute + 30*time.Second)},
		{TaskID: "t19", Started: start.Add(4 * time.Minute), Finished: start.Add(9 * time.Minute)},
		{TaskID: "t31", Started: start.Add(10 * time.Minute)},
		{TaskID: "t40"},
	}
}

func TestAJobsTaskTimesAreWrittenOneTaskPerLineWithADashForNoEnd(t *testing.T) {
	written := contract.JobTaskTimeLines(theCampaignTimes())

	want := "t17 2026-09-07T23:26:10Z 2026-09-07T23:29:40Z\n" +
		"t19 2026-09-07T23:30:10Z 2026-09-07T23:35:10Z\n" +
		"t31 2026-09-07T23:36:10Z -"
	if written != want {
		t.Errorf("the task times are written as:\n%s\nwant:\n%s", written, want)
	}
	if contract.JobTaskTimeLines(nil) != "" {
		t.Errorf("no times are written as %q, want nothing", contract.JobTaskTimeLines(nil))
	}
}

func TestAJobsTaskTimesReadBackWhatWasWritten(t *testing.T) {
	read := contract.ParseJobTaskTimeLines(contract.JobTaskTimeLines(theCampaignTimes()))

	want := theCampaignTimes()[:3]
	if len(read) != len(want) {
		t.Fatalf("read %d task times back, want %d: %+v", len(read), len(want), read)
	}
	for at := range want {
		if read[at].TaskID != want[at].TaskID || !read[at].Started.Equal(want[at].Started) || !read[at].Finished.Equal(want[at].Finished) {
			t.Errorf("task time %d reads %+v, want %+v", at, read[at], want[at])
		}
	}
}

func TestATaskTimeLineInAnotherShapeIsSkipped(t *testing.T) {
	read := contract.ParseJobTaskTimeLines("\nt17 2026-09-07T23:26:10Z\nt19 not-a-moment -\n  t31 2026-09-07T23:36:10Z -  \nwords words words words\n")
	if len(read) != 1 || read[0].TaskID != "t31" || read[0].Started.IsZero() || !read[0].Finished.IsZero() {
		t.Errorf("the lines read as %+v, want only t31, started and not finished", read)
	}
	if len(contract.ParseJobTaskTimeLines("")) != 0 {
		t.Error("an empty field read as task times")
	}
}

func FuzzParseJobTaskTimeLines(f *testing.F) {
	f.Add("t17 2026-09-07T23:26:10Z 2026-09-07T23:29:40Z\nt31 2026-09-07T23:36:10Z -")
	f.Add("")
	f.Add("t1 - -\n\n\n")
	f.Fuzz(func(t *testing.T, text string) {
		for _, one := range contract.ParseJobTaskTimeLines(text) {
			if one.TaskID == "" || one.Started.IsZero() {
				t.Fatalf("a task time with no label or no start was read out of %q: %+v", text, one)
			}
		}
	})
}

// TestATaskTimeKeepsItsFractionOfASecond: on run 22 the panel said a task
// took 3m 5s while its report said 3m 4s, because the moments crossed the
// socket to the whole second and the two ends rounded the span differently.
// The lines carry the fraction, so the screen and the report agree.
func TestATaskTimeKeepsItsFractionOfASecond(t *testing.T) {
	started := time.Date(2026, 9, 7, 18, 7, 8, 633_823_409, time.UTC)
	finished := time.Date(2026, 9, 7, 18, 10, 13, 506_682_852, time.UTC)
	read := contract.ParseJobTaskTimeLines(contract.JobTaskTimeLines([]contract.JobTaskTime{{TaskID: "t1", Started: started, Finished: finished}}))
	if len(read) != 1 || !read[0].Started.Equal(started) || !read[0].Finished.Equal(finished) {
		t.Errorf("the task time reads %+v after the round trip, want the fractions kept", read)
	}
	if got := read[0].Finished.Sub(read[0].Started); got != finished.Sub(started) {
		t.Errorf("the span reads %v after the round trip, want %v", got, finished.Sub(started))
	}
}
