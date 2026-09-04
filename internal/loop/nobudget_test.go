package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The tests in this file hold the user's rule in the loop: Nerd Genie has no cap on
// its own work unless the user sets one. On the defaults a task runs past a
// hundred rounds and past an hour and is never stopped for either, its record
// says it has no budget, and a stop the person asks for costs no model call. A
// budget the user does set still stops the task with the final answer.

// theLongTask is how many rounds the unbudgeted task below runs: past the
// hundred the old default would have stopped it at.
const theLongTask = 120

// aTaskOfRounds is a script that reads a different file every round and then
// answers, so that the identical-call detector has nothing to refuse.
func aTaskOfRounds(count int) []testkit.Step {
	steps := []testkit.Step{}
	for number := 1; number <= count; number++ {
		steps = append(steps, callStep(fmt.Sprintf("I will read part %d.", number),
			callFor(fmt.Sprintf("c%d", number), "read", fmt.Sprintf(`{"path":"part-%d.md"}`, number))))
	}
	return append(steps,
		answerStep("I read every part. Nothing is left."),
		answerStep("The review says what to keep."))
}

// tickingTool answers every read with a different line and moves the fake clock
// on by a fixed step each time, so that a task spends more than an hour without
// any test waiting for one.
type tickingTool struct {
	clock *testkit.FakeClock
	step  time.Duration
	reads int
}

// Spec is what the model is told about the ticking tool.
func (tool *tickingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        "read",
		Description: "A read that takes a fixed slice of the clock, so that a task's time can be seen passing.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run moves the clock on and answers with the number of the read.
func (tool *tickingTool) Run(context.Context, json.RawMessage) (contract.ToolOutput, error) {
	tool.reads++
	if tool.clock != nil {
		tool.clock.Advance(tool.step)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("the notes, part %d", tool.reads)}, nil
}

// loopWithCaps rebuilds the harness's loop over the caps the test chooses,
// which is how a budget set in config.toml reaches the loop.
func loopWithCaps(t *testing.T, built *harness, caps contract.Caps) {
	t.Helper()
	options := built.options()
	options.Caps = caps
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build the loop over the test's caps: %v", err)
	}
	built.loop = made
}

// TestATaskWithNoBudgetRunsPastAHundredRoundsAndAnHour is the rule itself: on
// the defaults, nothing stops a task but its own ending.
func TestATaskWithNoBudgetRunsPastAHundredRoundsAndAnHour(t *testing.T) {
	tool := &tickingTool{step: time.Minute}
	built := newHarness(t, aTaskOfRounds(theLongTask), tool)
	tool.clock = built.clock
	loopWithCaps(t, built, contract.DefaultConfig().Caps)

	outcome := built.ask(t, "read every part of the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: nothing but its own answer may end a task with no budget. %s",
			outcome.Status, outcome.Report)
	}
	if tool.reads != theLongTask {
		t.Errorf("the tool ran %d times, want %d: every round of a task with no budget runs", tool.reads, theLongTask)
	}
	if spent := built.clock.Now().Sub(theStartOfTime); spent < time.Hour {
		t.Errorf("the task spent %s, and this test is only a proof past an hour", spent)
	}
	held := built.held(t, outcome.TaskID)
	if !held.Header.NoRoundBudget || !held.Header.NoTimeBudget {
		t.Errorf("the record's header is %+v, want both limits off", held.Header)
	}
	if first := strings.SplitN(string(record.Print(held)), "\n", 2)[0]; !strings.HasSuffix(first, "no budget") {
		t.Errorf("the record's first line reads %q, want it to end with \"no budget\"", first)
	}
	for _, sent := range built.channel.Sent() {
		if strings.Contains(strings.ToLower(sent), "budget") {
			t.Errorf("the user was told %q, and a task with no budget never mentions one", sent)
		}
	}
}

// TestARoundBudgetSetInTheCapsStillStopsTheTaskWithTheFinalAnswer proves the
// other half: a user who sets rounds_per_task gets the forced final answer.
func TestARoundBudgetSetInTheCapsStillStopsTheTaskWithTheFinalAnswer(t *testing.T) {
	tool := &tickingTool{step: time.Second}
	built := newHarness(t, aTaskOfRounds(theLongTask), tool)
	tool.clock = built.clock
	caps := contract.DefaultConfig().Caps
	caps.RoundsPerTask = 3
	loopWithCaps(t, built, caps)

	outcome := built.ask(t, "read every part of the notes")

	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped, because the user set a budget of three rounds", outcome.Status)
	}
	if tool.reads != 3 {
		t.Errorf("the tool ran %d times, want the three rounds the budget allows", tool.reads)
	}
	if !sentSomethingLike(built.channel.Sent(), "budget of 3 rounds is used up") {
		t.Errorf("the user was sent %v, want the one report saying why the task ended", built.channel.Sent())
	}
	held := built.held(t, outcome.TaskID)
	if held.Header.NoRoundBudget || held.Header.RoundsLeft != 0 || !held.Header.NoTimeBudget {
		t.Errorf("the record's header is %+v, want a spent round budget and no time budget", held.Header)
	}
	if first := strings.SplitN(string(record.Print(held)), "\n", 2)[0]; !strings.HasSuffix(first, "budget left: 0 rounds") {
		t.Errorf("the record's first line reads %q, want it to name only the rounds", first)
	}
}

// TestATimeBudgetSetInTheCapsStillStopsTheTask proves the clock half: a user
// who sets time_per_task gets the same ending, and the header names only the
// minutes.
func TestATimeBudgetSetInTheCapsStillStopsTheTask(t *testing.T) {
	tool := &tickingTool{step: 10 * time.Minute}
	built := newHarness(t, aTaskOfRounds(theLongTask), tool)
	tool.clock = built.clock
	caps := contract.DefaultConfig().Caps
	caps.TimePerTask = 25 * time.Minute
	loopWithCaps(t, built, caps)

	outcome := built.ask(t, "read every part of the notes")

	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped, because the user set a budget of twenty-five minutes", outcome.Status)
	}
	if tool.reads != 3 {
		t.Errorf("the tool ran %d times, want three: the third read takes the clock past the budget", tool.reads)
	}
	if !sentSomethingLike(built.channel.Sent(), "budget of 25m0s is used up") {
		t.Errorf("the user was sent %v, want the one report saying why the task ended", built.channel.Sent())
	}
	// The budget is written after every model call and before the tools run,
	// so the header carries what was left before the read that spent it.
	held := built.held(t, outcome.TaskID)
	if !held.Header.NoRoundBudget || held.Header.NoTimeBudget || held.Header.MinutesLeft != 5 {
		t.Errorf("the record's header is %+v, want no round budget and five minutes left on the time budget", held.Header)
	}
	if first := strings.SplitN(string(record.Print(held)), "\n", 2)[0]; !strings.HasSuffix(first, "budget left: 5 minutes") {
		t.Errorf("the record's first line reads %q, want it to name only the minutes", first)
	}
}

// TestAStoppedTaskWithNoBudgetCarriesOnWithNone is the resume rule under the
// new default: a task the person stops and then continues runs on with no
// budget, and its situation says it was asked to carry on.
func TestAStoppedTaskWithNoBudgetCarriesOnWithNone(t *testing.T) {
	tool := &deliveringTool{name: "read", stops: true, answers: []string{"the notes", "the brand file"}}
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("The notes are read. I will read the brand file.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("I read the notes and the brand file. Nothing is left."),
	}, tool)
	tool.holder = built

	stopped := built.ask(t, "read the notes and the brand file")
	if stopped.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped, because the person asked it to", stopped.Status)
	}
	carriedOn := continueTheTask(t, built, stopped.TaskID)

	if carriedOn.Status != contract.StatusDone {
		t.Fatalf("the continued task ended %q, want done. %s", carriedOn.Status, carriedOn.Report)
	}
	held := built.held(t, stopped.TaskID)
	if !held.Header.NoRoundBudget || !held.Header.NoTimeBudget {
		t.Errorf("the continued task's header is %+v, want no budget, because none is set", held.Header)
	}
	situation := strings.Join(held.Work.Situation, "\n")
	if !strings.Contains(situation, "carry on") || !strings.Contains(situation, "no budget") {
		t.Errorf("the situation reads %q, and one line of it says the person asked this task to carry on and that it has no budget", situation)
	}
}

// TestAStopThePersonAsksForCostsNoMoreModelCalls is what pressing Escape
// means: the call in flight is cancelled and nothing is asked of the model
// after it. The loop used to ask the four review questions on top, a model
// call Escape could not reach, so the person pressed it twice and waited.
func TestAStopThePersonAsksForCostsNoMoreModelCalls(t *testing.T) {
	tool := &deliveringTool{name: "read", stops: true, answers: []string{"the notes"}}
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("This answer is never asked for, because the person stopped the task."),
	}, tool)
	tool.holder = built

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped", outcome.Status)
	}
	if calls := len(built.model.Requests()); calls != 1 {
		t.Errorf("the model was called %d times, want one: the call the person stopped, and nothing after it", calls)
	}
	if !sentSomethingLike(built.channel.Sent(), "the user asked the task to stop") {
		t.Errorf("the user was sent %v, want the stopped report", built.channel.Sent())
	}
}
