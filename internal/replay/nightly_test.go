package replay_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aNight is a little more than the day the nightly schedule waits, which is
// what a test moves the fake clock by to make the job fire.
const aNight = 24*time.Hour + time.Minute

// nightlyHarness is the nightly self-check with every dependency a fake.
type nightlyHarness struct {
	// nightly is the self-check under test.
	nightly *replay.Nightly
	// jobs is the fake job store the job is registered in.
	jobs *testkit.FakeJob
	// clock is the fake clock the schedule fires on.
	clock *testkit.FakeClock
	// channel is where the one line goes.
	channel *testkit.FakeChannel
	// skills are the skills whose dry runs are checked.
	skills *testkit.FakeSkill
	// dryRunsThatFail are the skills whose dry run says no.
	dryRunsThatFail map[string]bool
	// dryRuns is every skill whose dry run was asked for, in order.
	dryRuns []string
}

// newNightlyHarness builds the self-check over the fakes, with the memory given.
func newNightlyHarness(t *testing.T, remembering contract.Memory) *nightlyHarness {
	t.Helper()
	built := &nightlyHarness{
		clock:           testkit.NewFakeClock(theStartOfTime),
		channel:         testkit.NewFakeChannel("terminal"),
		skills:          testkit.NewFakeSkill(),
		dryRunsThatFail: map[string]bool{},
	}
	built.jobs = testkit.NewFakeJob(built.clock)
	made, err := replay.NewNightly(replay.NightlySettings{
		Jobs: built.jobs, Memory: remembering, Skills: built.skills,
		DryRun: built.dryRun, Send: built.channel.Send,
	})
	if err != nil {
		t.Fatalf("cannot build the nightly self-check: %v", err)
	}
	built.nightly = made
	return built
}

// dryRun stands in for a skill's dry run and says no to the ones the test named.
func (built *nightlyHarness) dryRun(_ context.Context, name string) (string, error) {
	built.dryRuns = append(built.dryRuns, name)
	if built.dryRunsThatFail[name] {
		return "", errors.New("the dry run of this skill did not reach its last step")
	}
	return "every step of the dry run worked", nil
}

// addSkill puts one skill in front of the self-check.
func (built *nightlyHarness) addSkill(name string) {
	built.skills.Add(contract.SkillSummary{Name: name, Description: "A skill the test wrote."},
		"# "+name+"\nA skill the test wrote.\n")
}

// fireTheSchedule moves the clock a night on and hands back the task the job
// store makes, which is what the running agent would pick up.
func (built *nightlyHarness) fireTheSchedule(t *testing.T) contract.TaskToRun {
	t.Helper()
	built.clock.Advance(aNight)
	task, due, err := built.jobs.NextTask(context.Background(), built.clock.Now())
	if err != nil {
		t.Fatalf("cannot ask the jobs what is due: %v", err)
	}
	if !due {
		t.Fatal("a night went by and the nightly job had nothing due")
	}
	return task
}

// twentyFacts is a memory holding twenty facts a search can find by their own
// words, which is a memory whose index is working.
func twentyFacts() *testkit.FakeMemory {
	facts := make([]contract.Fact, 0, replay.TwentyQuestions)
	for at := range replay.TwentyQuestions {
		facts = append(facts, contract.Fact{
			ID:       "m" + string(rune('a'+at)),
			Text:     "the user keeps their notes in folder number " + string(rune('a'+at)),
			Source:   "task 1",
			Recorded: theStartOfTime.Add(time.Duration(at) * time.Hour),
		})
	}
	return testkit.NewFakeMemory(facts...)
}

func TestTheNightlyJobFiresOnTheClockAndReportsOneLine(t *testing.T) {
	ctx := context.Background()
	built := newNightlyHarness(t, twentyFacts())
	built.addSkill("post")
	built.addSkill("shop")

	jobID, err := built.nightly.Register(ctx)
	if err != nil {
		t.Fatalf("cannot register the nightly job: %v", err)
	}
	if _, due, _ := built.jobs.NextTask(ctx, built.clock.Now()); due {
		t.Fatal("the nightly job was due the moment it was made, and it waits for the night")
	}

	task := built.fireTheSchedule(t)
	if task.JobID != jobID {
		t.Fatalf("the task that came due belongs to job %q and the nightly job is %q", task.JobID, jobID)
	}
	if !built.nightly.Handles(task) {
		t.Fatal("the nightly self-check does not recognise its own task")
	}
	if !task.Unattended {
		t.Error("the nightly task is attended, and nobody is awake at four in the morning")
	}

	report, err := built.nightly.Run(ctx, task)
	if err != nil {
		t.Fatalf("the nightly self-check did not finish: %v", err)
	}
	if report.Failed != 0 {
		t.Errorf("the self-check failed %d of its checks on a machine where everything works: %s", report.Failed, report.Line)
	}
	if report.Passed != replay.TwentyQuestions+2 {
		t.Errorf("the self-check ran %d checks and there are twenty questions and two skills", report.Passed)
	}
	if sent := built.channel.Sent(); len(sent) != 1 || sent[0] != report.Line {
		t.Fatalf("the user was sent %v and the self-check reported %q", sent, report.Line)
	}
	if !strings.Contains(report.Line, "22 passed") {
		t.Errorf("the one line is %q and it must say how many passed", report.Line)
	}
}

func TestTheNightlyJobIsRegisteredOnlyOnce(t *testing.T) {
	ctx := context.Background()
	built := newNightlyHarness(t, twentyFacts())

	first, err := built.nightly.Register(ctx)
	if err != nil {
		t.Fatalf("cannot register the nightly job: %v", err)
	}
	second, err := built.nightly.Register(ctx)
	if err != nil {
		t.Fatalf("cannot register the nightly job a second time: %v", err)
	}
	if first != second {
		t.Errorf("registering twice made jobs %q and %q, and there is only one nightly job", first, second)
	}
	listed, err := built.jobs.List(ctx)
	if err != nil {
		t.Fatalf("cannot list the jobs: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("the job store holds %d jobs and one was registered twice", len(listed))
	}
}

func TestTheNightlyCheckNamesWhatFailed(t *testing.T) {
	ctx := context.Background()
	built := newNightlyHarness(t, forgetfulMemory{fact: contract.Fact{
		ID: "m1", Text: "the user keeps their notes in the notes folder", Recorded: theStartOfTime,
	}})
	built.addSkill("post")
	built.dryRunsThatFail["post"] = true

	if _, err := built.nightly.Register(ctx); err != nil {
		t.Fatalf("cannot register the nightly job: %v", err)
	}
	report, err := built.nightly.Run(ctx, built.fireTheSchedule(t))
	if err != nil {
		t.Fatalf("the nightly self-check did not finish: %v", err)
	}

	if report.Failed != 2 {
		t.Fatalf("the self-check failed %d checks and both the question and the skill were meant to fail: %s", report.Failed, report.Line)
	}
	if !strings.Contains(report.Line, "m1") || !strings.Contains(report.Line, "post") {
		t.Errorf("the one line is %q and it must name the question and the skill that failed", report.Line)
	}
	if !strings.Contains(report.Line, "2 failed") {
		t.Errorf("the one line is %q and it must say how many failed", report.Line)
	}
}

// twelveNights is more than the ten failures in a row that switch a scheduled
// job off for good, which is what a self-check reporting a genuinely broken
// skill every night would otherwise reach.
const twelveNights = 12

func TestASkillThatIsBrokenEveryNightDoesNotSwitchTheSelfCheckOff(t *testing.T) {
	ctx := context.Background()
	built := newNightlyHarness(t, twentyFacts())
	built.addSkill("post")
	built.dryRunsThatFail["post"] = true
	if _, err := built.nightly.Register(ctx); err != nil {
		t.Fatalf("cannot register the nightly job: %v", err)
	}

	for night := 1; night <= twelveNights; night++ {
		report, err := built.nightly.Run(ctx, built.fireTheSchedule(t))
		if err != nil {
			t.Fatalf("night %d of the self-check did not finish: %v", night, err)
		}
		if !strings.Contains(report.Line, "post") {
			t.Fatalf("night %d did not name the skill that is broken: %q", night, report.Line)
		}
	}

	listed, err := built.jobs.List(ctx)
	if err != nil {
		t.Fatalf("cannot list the jobs: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("the job store holds %d jobs rather than the one nightly job", len(listed))
	}
	if listed[0].State != contract.JobRunning {
		t.Errorf("after %d nights of correctly reporting a broken skill the nightly job is %q, and a check that goes quiet is worse than no check at all",
			twelveNights, listed[0].State)
	}
	if sent := built.channel.Sent(); len(sent) != twelveNights {
		t.Errorf("the user heard on %d of the %d nights", len(sent), twelveNights)
	}
}

func TestTheNightlyCheckSaysSoWhenThereIsNothingToCheck(t *testing.T) {
	ctx := context.Background()
	built := newNightlyHarness(t, testkit.NewFakeMemory())

	if _, err := built.nightly.Register(ctx); err != nil {
		t.Fatalf("cannot register the nightly job: %v", err)
	}
	report, err := built.nightly.Run(ctx, built.fireTheSchedule(t))
	if err != nil {
		t.Fatalf("the nightly self-check did not finish: %v", err)
	}
	if report.Failed != 0 {
		t.Errorf("an empty machine failed %d checks: %s", report.Failed, report.Line)
	}
	if !strings.Contains(report.Line, "memory") {
		t.Errorf("the one line is %q and it must say the memory held nothing to ask about", report.Line)
	}
}

func TestTheNightlyCheckIgnoresATaskThatIsNotItsOwn(t *testing.T) {
	built := newNightlyHarness(t, twentyFacts())

	if built.nightly.Handles(contract.TaskToRun{JobID: "9", TaskID: "t1", Text: "post the anniversary tweet"}) {
		t.Error("the nightly self-check claimed somebody else's task")
	}
}

func TestNewNightlyRefusesSettingsItCannotRunOn(t *testing.T) {
	clock := testkit.NewFakeClock(theStartOfTime)
	whole := replay.NightlySettings{
		Jobs: testkit.NewFakeJob(clock),
		Send: func(context.Context, string) error { return nil },
	}
	missing := map[string]replay.NightlySettings{
		"the job store": {Send: whole.Send},
		"the channel":   {Jobs: whole.Jobs},
	}
	for what, settings := range missing {
		t.Run(what, func(t *testing.T) {
			if _, err := replay.NewNightly(settings); err == nil {
				t.Fatalf("a nightly self-check with no %s was built anyway", what)
			}
		})
	}
	if _, err := replay.NewNightly(whole); err != nil {
		t.Errorf("a nightly self-check with everything it needs was refused: %v", err)
	}
}

// forgetfulMemory is a memory that holds one fact and cannot find it again,
// which is what a broken index looks like from outside.
type forgetfulMemory struct {
	// fact is the one thing it holds.
	fact contract.Fact
}

// Search hands back the fact when nothing is asked for and nothing when
// something is, which is the failure the twenty questions are there to catch.
func (forgetful forgetfulMemory) Search(_ context.Context, query string, _ int) ([]contract.Fact, error) {
	if strings.TrimSpace(query) == "" {
		return []contract.Fact{forgetful.fact}, nil
	}
	return nil, nil
}

// Get hands back the one fact it holds.
func (forgetful forgetfulMemory) Get(_ context.Context, id string) (contract.Fact, error) {
	if id == forgetful.fact.ID {
		return forgetful.fact, nil
	}
	return contract.Fact{}, errors.New("this memory holds no fact with that id, so search for it instead")
}

// Save takes nothing, because the nightly self-check never writes to memory.
func (forgetful forgetfulMemory) Save(_ context.Context, _ []contract.Fact) error {
	return errors.New("the nightly self-check must never write to memory, so this memory refuses")
}

// Hint returns nothing, because the self-check asks no hints.
func (forgetful forgetfulMemory) Hint(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
