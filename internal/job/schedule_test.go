package job_test

import (
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/job"
)

// aScheduledJob creates a job with a schedule and a template, and fails the test
// when the schedule is refused.
func (holding *opened) aScheduledJob(t *testing.T, ask string, schedule contract.Schedule) string {
	t.Helper()
	jobID, err := holding.jobs.Create(t.Context(), contract.NewJob{
		Ask: ask, Schedule: &schedule, TaskTemplate: "write and post today's message",
	})
	if err != nil {
		t.Fatalf("cannot create the scheduled job %q: %v", ask, err)
	}
	return jobID
}

func TestEachKindOfScheduleMakesOneTaskPerTick(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("this machine has no time zone database, so a named time zone cannot be tested: %v", err)
	}
	morning := time.Date(2026, 9, 2, 6, 0, 0, 0, newYork)

	for _, shape := range []struct {
		name     string
		schedule contract.Schedule
		firstAt  time.Time
		secondAt time.Time
		ticks    int
	}{
		{
			name:     "every",
			schedule: contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
			firstAt:  morning.Add(time.Hour), secondAt: morning.Add(2 * time.Hour), ticks: 2,
		},
		{
			name:     "cron",
			schedule: contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 7 * * 1-5", Timezone: "America/New_York"},
			firstAt:  morning.Add(2 * time.Hour), secondAt: morning.Add(26 * time.Hour), ticks: 2,
		},
		{
			name:     "at",
			schedule: contract.Schedule{Kind: contract.ScheduleAt, At: morning.Add(3 * time.Hour)},
			firstAt:  morning.Add(3 * time.Hour), secondAt: morning.Add(48 * time.Hour), ticks: 1,
		},
	} {
		t.Run(shape.name, func(t *testing.T) {
			holding := newJobs(t)
			holding.clock.Advance(morning.Sub(theEpoch()))
			jobID := holding.aScheduledJob(t, "Post on the schedule.", shape.schedule)

			if _, due := holding.nextTask(t, morning); due {
				t.Fatal("a task was made before the first tick came round")
			}
			first, due := holding.nextTask(t, shape.firstAt)
			if !due || !first.Unattended || first.Text != "write and post today's message" {
				t.Fatalf("the first tick made %+v (due %v), want the template's text, unattended", first, due)
			}
			if _, due := holding.nextTask(t, shape.firstAt); due {
				t.Error("one tick made two tasks, and a tick makes one")
			}
			holding.finish(t, jobID, first.TaskID, "posted", false)

			if _, due := holding.nextTask(t, shape.secondAt); due != (shape.ticks > 1) {
				t.Errorf("the second tick was due=%v, want %v", due, shape.ticks > 1)
			}
			if summary := holding.summaryOf(t, jobID); shape.ticks > 1 && summary.NextRun.IsZero() {
				t.Error("a schedule that fires again has no next run, and the cron listing needs one")
			}
		})
	}
}

func TestATickThatWasMissedWhileTheMachineSleptMakesOneTaskAndNotAHundred(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})

	if _, due := holding.nextTask(t, theEpoch().Add(100*time.Hour)); !due {
		t.Fatal("a tick a hundred hours late made no task at all")
	}

	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Tasks) != 1 {
		t.Errorf("a hundred missed hours made %d tasks, want the one the design asks for", len(held.Work.Tasks))
	}
}

func TestATickBehindATaskDatedNextWeekStillRuns(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	holding.aTask(t, jobID, "the task for next week", theEpoch().Add(7*24*time.Hour))

	next, due := holding.nextTask(t, theEpoch().Add(90*time.Minute))

	if !due || next.Text != "write and post today's message" || !next.Unattended {
		t.Errorf("the task handed out is %+v (due %v), want the tick's own, unattended", next, due)
	}
}

func TestASchedulePushesTheNextTickBackAfterAFailure(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	next, due := holding.nextTask(t, theEpoch().Add(time.Hour))
	if !due {
		t.Fatal("the first tick made no task")
	}

	holding.clock.Advance(time.Hour)
	holding.finish(t, jobID, next.TaskID, "the site was down", true)

	first := holding.summaryOf(t, jobID).NextRun
	if wanted := theEpoch().Add(2 * time.Hour); !first.Equal(wanted) {
		t.Errorf("after one failure the next tick is %s, want %s, because the schedule's own moment is later than the backoff", first, wanted)
	}
	for round := 2; round <= 5; round++ {
		next, due = holding.nextTask(t, first.Add(time.Duration(round)*time.Hour))
		if !due {
			t.Fatalf("tick %d made no task", round)
		}
		holding.finish(t, jobID, next.TaskID, "the site was down", true)
	}
	if backoff := holding.summaryOf(t, jobID).FailuresInARow; backoff != 5 {
		t.Errorf("five failed ticks counted %d failures in a row", backoff)
	}
}

func TestASchedulePlainlyRefusesWhatItCannotRead(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()

	for _, wrong := range []struct {
		name     string
		schedule contract.Schedule
	}{
		{"a cron expression that is not one", contract.Schedule{Kind: contract.ScheduleCron, Cron: "not a cron expression"}},
		{"a time zone this machine does not know", contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 7 * * *", Timezone: "Mars/Olympus"}},
		{"an interval closer together than a task takes", contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Second}},
		{"a one-off with no moment to run at", contract.Schedule{Kind: contract.ScheduleAt}},
	} {
		t.Run(wrong.name, func(t *testing.T) {
			_, err := holding.jobs.Create(ctx, contract.NewJob{
				Ask: "Post on the schedule.", Schedule: &wrong.schedule, TaskTemplate: "post",
			})
			if err == nil {
				t.Errorf("a job was created with %s", wrong.name)
			}
		})
	}
}

func TestAJobWithNoScheduleHasNoUseForATemplate(t *testing.T) {
	holding := newJobs(t)

	_, err := holding.jobs.Create(t.Context(), contract.NewJob{Ask: "Do a long thing.", TaskTemplate: "post"})

	if err == nil {
		t.Error("a job with no schedule took a template, and nothing would ever make a task from it")
	}
}

func TestEverySchedulePrintsInPlainWords(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, shape := range []struct {
		schedule contract.Schedule
		want     string
	}{
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 7 * * 1-5"}, "every weekday at 7 in the morning"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "30 14 * * *"}, "every day at 2:30 in the afternoon"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 0 * * *"}, "every day at midnight"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 12 * * 1,4"}, "every Monday and Thursday at noon"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 20 * * 0,6"}, "every Saturday and Sunday at 8 in the evening"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "*/15 * * * *"}, "every 15 minutes"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 */2 * * *"}, "every 2 hours"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 9 3 * *"}, "every month on the 3rd at 9 in the morning"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "@daily"}, "every day at midnight"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "@every 90m"}, "every 90m"},
		{contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 9 1-7 * 1"}, `on the cron schedule "0 9 1-7 * 1"`},
		{contract.Schedule{Kind: contract.ScheduleEvery, Every: 2 * time.Hour}, "every 2 hours"},
		{contract.Schedule{Kind: contract.ScheduleEvery, Every: 24 * time.Hour}, "every day"},
		{contract.Schedule{Kind: contract.ScheduleEvery, Every: 90 * time.Minute}, "every 90 minutes"},
		{contract.Schedule{Kind: contract.ScheduleEvery, Every: 0}, "every no time at all"},
		{contract.Schedule{Kind: contract.ScheduleAt, At: at}, "once on 2026-10-01 at 09:00"},
		{contract.Schedule{Kind: contract.ScheduleAt, At: at, Timezone: "America/New_York"}, "once on 2026-10-01 at 09:00 in America/New_York"},
		{contract.Schedule{Kind: "sometimes"}, `on a schedule of the unknown kind "sometimes"`},
	} {
		t.Run(shape.want, func(t *testing.T) {
			if said := job.InPlainWords(shape.schedule); said != shape.want {
				t.Errorf("the schedule %+v reads as %q, want %q", shape.schedule, said, shape.want)
			}
		})
	}
}
