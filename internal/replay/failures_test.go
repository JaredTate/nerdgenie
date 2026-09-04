package replay

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// errBrokenOnPurpose is what every deliberately broken dependency in this file
// says when it is asked to do its job.
var errBrokenOnPurpose = errors.New("this part of the agent is broken on purpose, so that the code around it can be tested")

// brokenJobs is a job store where the three things the nightly self-check asks
// of it can each be made to fail.
type brokenJobs struct {
	contract.Job
	// listFails makes listing the jobs fail.
	listFails bool
	// createFails makes making the nightly job fail.
	createFails bool
	// finishFails makes writing the report back into the job fail.
	finishFails bool
}

// List hands back nothing, or fails when the test asked it to.
func (broken brokenJobs) List(context.Context) ([]contract.JobSummary, error) {
	if broken.listFails {
		return nil, errBrokenOnPurpose
	}
	return nil, nil
}

// Create makes the job, or fails when the test asked it to.
func (broken brokenJobs) Create(context.Context, contract.NewJob) (string, error) {
	if broken.createFails {
		return "", errBrokenOnPurpose
	}
	return "1", nil
}

// FinishTask writes the report back, or fails when the test asked it to.
func (broken brokenJobs) FinishTask(context.Context, string, string, string, bool) (string, error) {
	if broken.finishFails {
		return "", errBrokenOnPurpose
	}
	return "j1.1", nil
}

// brokenMemory is a memory that cannot be searched.
type brokenMemory struct {
	contract.Memory
	// afterTheFirst lets the first search work and fails the ones after it,
	// which is a memory that can list its facts and cannot look one up.
	afterTheFirst bool
	// searches counts the searches made so far.
	searches *int
}

// Search fails, or fails only after the first, depending on what the test asked
// for.
func (broken brokenMemory) Search(_ context.Context, _ string, _ int) ([]contract.Fact, error) {
	*broken.searches++
	if broken.afterTheFirst && *broken.searches == 1 {
		return []contract.Fact{{ID: "m1", Text: "the user keeps their notes in the notes folder"}}, nil
	}
	return nil, errBrokenOnPurpose
}

// brokenSkills is a skill store that cannot be listed.
type brokenSkills struct {
	contract.Skill
}

// List fails, which is what a skills folder nobody can read looks like.
func (broken brokenSkills) List(context.Context) ([]contract.SkillSummary, error) {
	return nil, errBrokenOnPurpose
}

// brokenStore is an event log that cannot be read by task.
type brokenStore struct {
	contract.Store
}

// ByTask fails, which is what a damaged database looks like from outside.
func (broken brokenStore) ByTask(context.Context, string) ([]contract.Event, error) {
	return nil, errBrokenOnPurpose
}

// aNightlyOver builds the self-check over the parts given, with the rest of its
// settings standing in for a machine where nothing else is wrong.
func aNightlyOver(t *testing.T, settings NightlySettings) *Nightly {
	t.Helper()
	if settings.Send == nil {
		settings.Send = func(context.Context, string) error { return nil }
	}
	made, err := NewNightly(settings)
	if err != nil {
		t.Fatalf("cannot build the nightly self-check: %v", err)
	}
	return made
}

func TestTheNightlyJobSaysWhichPartOfTheJobStoreFailed(t *testing.T) {
	ctx := context.Background()
	failing := map[string]brokenJobs{
		"listing the jobs": {listFails: true},
		"making the job":   {createFails: true},
	}
	for what, jobs := range failing {
		t.Run(what, func(t *testing.T) {
			_, err := aNightlyOver(t, NightlySettings{Jobs: jobs}).Register(ctx)
			if err == nil {
				t.Fatalf("%s failed and the nightly job was registered anyway", what)
			}
		})
	}
}

func TestTheNightlyCheckSaysSoWhenItsLineCannotBeSent(t *testing.T) {
	nightly := aNightlyOver(t, NightlySettings{
		Jobs: brokenJobs{},
		Send: func(context.Context, string) error { return errBrokenOnPurpose },
	})

	report, err := nightly.Run(context.Background(), contract.TaskToRun{})
	if err == nil {
		t.Fatal("the one line never reached the user and the self-check said nothing about it")
	}
	if report.Line == "" {
		t.Error("the self-check made its checks and gave back no line at all")
	}
}

func TestTheNightlyCheckSaysSoWhenItsReportCannotReachTheJob(t *testing.T) {
	nightly := aNightlyOver(t, NightlySettings{Jobs: brokenJobs{finishFails: true}})

	if _, err := nightly.Run(context.Background(), contract.TaskToRun{JobID: "1", TaskID: "t1"}); err == nil {
		t.Fatal("the report never reached the job and the self-check said nothing about it")
	}
}

func TestTheNightlyCheckSaysSoWhenTheMemoryCannotBeSearched(t *testing.T) {
	searches := 0
	nightly := aNightlyOver(t, NightlySettings{
		Jobs: brokenJobs{}, Memory: brokenMemory{searches: &searches},
	})

	report, err := nightly.Run(context.Background(), contract.TaskToRun{})
	if err != nil {
		t.Fatalf("the nightly self-check did not finish: %v", err)
	}
	if report.Failed != 1 || !strings.Contains(report.Line, theMemoryItself) {
		t.Errorf("the one line is %q and the memory could not be searched at all", report.Line)
	}
}

func TestTheNightlyCheckSaysSoWhenOneQuestionCannotBeAsked(t *testing.T) {
	searches := 0
	nightly := aNightlyOver(t, NightlySettings{
		Jobs: brokenJobs{}, Memory: brokenMemory{afterTheFirst: true, searches: &searches},
	})

	report, err := nightly.Run(context.Background(), contract.TaskToRun{})
	if err != nil {
		t.Fatalf("the nightly self-check did not finish: %v", err)
	}
	if report.Failed != 1 || !strings.Contains(report.Line, "m1") {
		t.Errorf("the one line is %q and the one question could not be asked", report.Line)
	}
}

func TestTheNightlyCheckSaysSoWhenTheSkillsCannotBeListed(t *testing.T) {
	nightly := aNightlyOver(t, NightlySettings{
		Jobs:   brokenJobs{},
		Skills: brokenSkills{},
		DryRun: func(context.Context, string) (string, error) { return "", nil },
	})

	report, err := nightly.Run(context.Background(), contract.TaskToRun{})
	if err != nil {
		t.Fatalf("the nightly self-check did not finish: %v", err)
	}
	if report.Failed != 1 || !strings.Contains(report.Line, "the skills") {
		t.Errorf("the one line is %q and the skills could not be listed", report.Line)
	}
}

func TestAReplayOfARecordingThatMakesNoRecordSaysSo(t *testing.T) {
	result, err := RunRecording(context.Background(), Options{
		Into:       testkit.NewFakeStore(),
		Context:    aWorkingContextForTheseTests(t),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(theStartOfTheseTests),
	}, Recording{TaskID: "7", Ask: "what is two and two", Answer: "four", Final: aSmallRecord("7", 99)})
	if err != nil {
		t.Fatalf("cannot replay a recording with no rounds in it: %v", err)
	}

	if result.Passed {
		t.Error("a replay that wrote no record was said to have reproduced one")
	}
	if !strings.Contains(result.Difference, "no record at all") {
		t.Errorf("the difference is %q and the replay made no record", result.Difference)
	}
}

func TestReadingARecordingOutOfALogThatWillNotBeRead(t *testing.T) {
	if _, err := Read(context.Background(), brokenStore{}, "17"); err == nil {
		t.Fatal("a recording was read out of a log that cannot be read")
	}
}
