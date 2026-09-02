package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// threeJobsOneAlreadyPaused makes two running jobs and one the user had already
// paused, which is what proves "/resume" starts only what "/pause" stopped.
func threeJobsOneAlreadyPaused(t *testing.T) (*testkit.FakeJob, string) {
	t.Helper()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime))
	for _, ask := range []string{"run the anniversary campaign", "post the weekly update"} {
		if _, err := jobs.Create(context.Background(), contract.NewJob{Ask: ask, Why: "because the user asked"}); err != nil {
			t.Fatalf("creating the job %q failed: %v", ask, err)
		}
	}
	alreadyPaused, err := jobs.Create(context.Background(), contract.NewJob{Ask: "tidy the memory folder", Why: "because the user asked"})
	if err != nil {
		t.Fatalf("creating the third job failed: %v", err)
	}
	if err := jobs.Pause(context.Background(), alreadyPaused); err != nil {
		t.Fatalf("pausing the third job failed: %v", err)
	}
	return jobs, alreadyPaused
}

// jobDeps wires a job store into the commands, with a resume that starts one
// job running again, which contract.Job itself has no way to do.
func jobDeps(jobs *testkit.FakeJob, resumed *[]string) command.Deps {
	return command.Deps{
		Jobs: jobs,
		ResumeJob: func(ctx context.Context, jobID string) error {
			*resumed = append(*resumed, jobID)
			return jobs.RunNow(ctx, jobID)
		},
	}
}

// runTwo registers the core commands once and runs two lines through them in
// order, which is how the pause and resume pair is tested on one set of state.
func runTwo(t *testing.T, deps command.Deps, first string, second string) (string, string) {
	t.Helper()
	registry := command.NewRegistry()
	for _, one := range command.New(registry, deps).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}
	where := contract.CommandContext{Channel: terminalChannel()}

	firstAnswer, err := registry.Run(context.Background(), first, where)
	if err != nil {
		t.Fatalf("running %q failed: %v", first, err)
	}
	secondAnswer, err := registry.Run(context.Background(), second, where)
	if err != nil {
		t.Fatalf("running %q failed: %v", second, err)
	}
	return firstAnswer, secondAnswer
}

func TestPauseStopsEveryRunningJobAndResumeStartsExactlyThose(t *testing.T) {
	jobs, alreadyPaused := threeJobsOneAlreadyPaused(t)
	resumed := []string{}

	pausedAnswer, resumedAnswer := runTwo(t, jobDeps(jobs, &resumed), "/pause", "/resume")

	for _, name := range []string{"1", "2"} {
		if !strings.Contains(pausedAnswer, name) {
			t.Errorf("the pause command does not name the job %s it paused: %q", name, pausedAnswer)
		}
		if !strings.Contains(resumedAnswer, name) {
			t.Errorf("the resume command does not name the job %s it started again: %q", name, resumedAnswer)
		}
	}
	if strings.Join(resumed, " ") != "1 2" {
		t.Errorf("the resume command started %v rather than exactly the two the pause command stopped", resumed)
	}

	listed, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	for _, job := range listed {
		wanted := contract.JobRunning
		if job.ID == alreadyPaused {
			wanted = contract.JobPaused
		}
		if job.State != wanted {
			t.Errorf("the job %s came out %s rather than %s", job.ID, job.State, wanted)
		}
	}
}

func TestPauseSaysWhenNothingIsRunning(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime))
	resumed := []string{}

	answer := runOne(t, jobDeps(jobs, &resumed), "/pause")

	if !strings.Contains(answer, "nothing to pause") {
		t.Errorf("the pause command does not say that nothing was running: %q", answer)
	}
}

func TestResumeSaysWhenPauseStoppedNothing(t *testing.T) {
	jobs, _ := threeJobsOneAlreadyPaused(t)
	resumed := []string{}

	answer := runOne(t, jobDeps(jobs, &resumed), "/resume")

	if len(resumed) != 0 {
		t.Fatalf("the resume command started %v, which /pause never stopped", resumed)
	}
	if !strings.Contains(answer, "nothing to resume") {
		t.Errorf("the resume command does not say that it had stopped nothing: %q", answer)
	}
}

func TestPauseSaysWhenThereIsNoJobStore(t *testing.T) {
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/pause", contract.CommandContext{Channel: terminalChannel()}); err == nil {
		t.Fatalf("the pause command claimed to pause the jobs with no job store at all")
	}
}
