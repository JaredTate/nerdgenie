package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// threeFixtureJobs makes the three jobs the golden files are written from: a
// plain job part way through its list, a scheduled job that has run once, and a
// job that was paused.
func threeFixtureJobs(t *testing.T) *opened {
	t.Helper()
	ctx := t.Context()
	holding := newJobs(t)

	campaign := holding.aJob(t, "Run the DigiByte anniversary campaign this month.")
	first := holding.aTask(t, campaign, "post the anniversary tweet", time.Time{})
	holding.aTask(t, campaign, "draft the blog piece", time.Time{})
	holding.aTask(t, campaign, "post for day three", time.Date(2026, 9, 3, 14, 0, 0, 0, time.UTC))
	holding.finish(t, campaign, first, "posted, 236 characters, link saved", false)

	morning := holding.aScheduledJob(t, "Post the morning update every weekday.",
		contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 7 * * 1-5", Timezone: "America/New_York"})
	// The campaign is put down while the morning job ticks, so that the tick's
	// own task is the one handed out rather than the campaign's next.
	if err := holding.jobs.Pause(ctx, campaign); err != nil {
		t.Fatalf("cannot pause the campaign job: %v", err)
	}
	ticked, due := holding.nextTask(t, time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	if !due || ticked.JobID != morning {
		t.Fatalf("the morning job's first tick made %+v (due %v)", ticked, due)
	}
	holding.finish(t, morning, ticked.TaskID, "posted the morning update", false)
	if err := holding.jobs.Resume(ctx, campaign); err != nil {
		t.Fatalf("cannot set the campaign job running again: %v", err)
	}

	tidying := holding.aJob(t, "Tidy the blog folder.")
	holding.aTask(t, tidying, "list what is in the folder", time.Time{})
	if err := holding.jobs.Pause(ctx, tidying); err != nil {
		t.Fatalf("cannot pause the tidying job: %v", err)
	}
	return holding
}

// runCommand runs one of the two commands and fails the test on an error.
func runCommand(t *testing.T, command contract.Command, arguments string) string {
	t.Helper()
	answer, err := command.Run(t.Context(), arguments, contract.CommandContext{})
	if err != nil {
		t.Fatalf("running /%s %q failed: %v", command.Name, arguments, err)
	}
	return answer
}

func TestTheJobsAndCronListingsReadAsTheGoldenFilesSay(t *testing.T) {
	holding := threeFixtureJobs(t)
	listings := holding.jobs.JobsCommand()
	scheduled := holding.jobs.CronCommand()

	testkit.Golden(t, "jobs-list.golden", []byte(runCommand(t, listings, "")))
	testkit.Golden(t, "jobs-one.golden", []byte(runCommand(t, listings, "1")))
	testkit.Golden(t, "cron-list.golden", []byte(runCommand(t, scheduled, "")))
	testkit.Golden(t, "cron-one.golden", []byte(runCommand(t, scheduled, "2")))
}

func TestTheTwoCommandsSayWhatTheyAreAndRefuseWhatIsNotThere(t *testing.T) {
	holding := threeFixtureJobs(t)
	ctx := t.Context()
	listings := holding.jobs.JobsCommand()
	scheduled := holding.jobs.CronCommand()

	if listings.Name != "jobs" || listings.Help == "" || listings.TerminalOnly {
		t.Errorf("the jobs command is %+v, want one named jobs with a help line, allowed on any channel", listings)
	}
	if scheduled.Name != "cron" || scheduled.Help == "" {
		t.Errorf("the cron command is %+v, want one named cron with a help line", scheduled)
	}
	if _, err := listings.Run(ctx, "99", contract.CommandContext{}); err == nil {
		t.Error("/jobs 99 printed a job that is not there")
	}
	if _, err := scheduled.Run(ctx, "99", contract.CommandContext{}); err == nil {
		t.Error("/cron 99 printed a job that is not there")
	}
	if _, err := scheduled.Run(ctx, "1", contract.CommandContext{}); err == nil {
		t.Error("/cron 1 printed a job that has no schedule")
	}
	if _, err := scheduled.Run(ctx, "run 99", contract.CommandContext{}); err == nil {
		t.Error("/cron run 99 ran a job that is not there")
	}
	if _, err := scheduled.Run(ctx, "off 99", contract.CommandContext{}); err == nil {
		t.Error("/cron off 99 switched off a job that is not there")
	}
}

func TestCronRunsAndSwitchesOffOneJob(t *testing.T) {
	holding := threeFixtureJobs(t)
	ctx := t.Context()
	scheduled := holding.jobs.CronCommand()

	if said := runCommand(t, scheduled, "run 2"); !strings.Contains(said, "runs its next task now") {
		t.Errorf("/cron run 2 said %q", said)
	}
	if said := runCommand(t, scheduled, "off 2"); !strings.Contains(said, "switched off") {
		t.Errorf("/cron off 2 said %q", said)
	}
	if state := holding.summaryOf(t, "2").State; state != contract.JobOff {
		t.Errorf("after /cron off 2 the job is %q, want %q", state, contract.JobOff)
	}
	if _, err := scheduled.Run(ctx, "run 2", contract.CommandContext{}); err != nil {
		t.Errorf("/cron run 2 could not start the job again: %v", err)
	}
	if state := holding.summaryOf(t, "2").State; state != contract.JobRunning {
		t.Errorf("after /cron run 2 the job is %q, want %q", state, contract.JobRunning)
	}
}

func TestTheListingsSayWhenThereIsNothingToList(t *testing.T) {
	holding := newJobs(t)

	if said := runCommand(t, holding.jobs.JobsCommand(), ""); !strings.Contains(said, "no jobs") {
		t.Errorf("/jobs on an agent with no jobs said %q", said)
	}
	if said := runCommand(t, holding.jobs.CronCommand(), ""); !strings.Contains(said, "No job has a schedule") {
		t.Errorf("/cron on an agent with no scheduled jobs said %q", said)
	}
	holding.aJob(t, "Do a long thing.")
	if said := runCommand(t, holding.jobs.CronCommand(), ""); !strings.Contains(said, "No job has a schedule") {
		t.Errorf("/cron with one plain job said %q", said)
	}
}

func TestALongAskIsCutToFitOneLine(t *testing.T) {
	holding := newJobs(t)
	holding.aJob(t, strings.Repeat("a very long ask ", 20))

	said := runCommand(t, holding.jobs.JobsCommand(), "")

	for _, line := range strings.Split(said, "\n") {
		if len(line) > 160 {
			t.Errorf("a line of the listing is %d characters long: %q", len(line), line)
		}
	}
	if !strings.Contains(said, "...") {
		t.Error("a long ask was printed whole rather than cut to fit the column")
	}
}
