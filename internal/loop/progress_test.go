package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// anEditRound is one round that changes a line of a file that already exists,
// which moves nothing the harness can measure: no test, no mark, no page, no
// new file. It is the shape of every stall the meter is for.
func anEditRound(number int) testkit.Step {
	return callStep("I will try one more change.", callFor(fmt.Sprintf("e%d", number), contract.ToolEdit,
		fmt.Sprintf(`{"path":"/game/src/engine.js","old":"a%d","new":"b%d"}`, number, number)))
}

// editsForever is a script of edit rounds followed by an answer nobody
// should reach, with an edit tool that answers every one.
func editsForever(t *testing.T, rounds int, extra ...contract.Tool) *harness {
	t.Helper()
	steps := []testkit.Step{}
	outputs := []string{}
	for at := 1; at <= rounds; at++ {
		steps = append(steps, anEditRound(at))
		outputs = append(outputs, fmt.Sprintf("edited /game/src/engine.js by 1 line (%d)", at))
	}
	steps = append(steps, answerStep("Fixed. What changed: the engine. What I checked: nothing. What is left: nothing."))
	tools := append([]contract.Tool{scriptedTool(contract.ToolEdit, outputs...)}, extra...)
	return newHarness(t, steps, tools...)
}

// requestsCarrying counts the model calls whose prompt carried the words.
func requestsCarrying(built *harness, words string) (int, int) {
	first, count := -1, 0
	for at, request := range built.model.Requests() {
		if strings.Contains(wholeRequestText(request), words) {
			count++
			if first < 0 {
				first = at
			}
		}
	}
	return first, count
}

// TestRoundsWithoutProgressClimbTheLadderNudgeThenRewindThenStop is the
// meter's whole promise. The fourth game build spent sixty rounds on two tests
// with every probe a character different, so the same-call guard never fired;
// the fifth asked the desktop tool to launch an unnamed application eleven
// times. Ten such rounds earn one line, twenty clear the conversation with the
// stall written into the record, and twenty more after that stop the task.
func TestRoundsWithoutProgressClimbTheLadderNudgeThenRewindThenStop(t *testing.T) {
	built := editsForever(t, 2*loop.RewindAfterRoundsWithoutProgress+5)

	outcome := built.ask(t, "make the tests pass")

	first, _ := requestsCarrying(built, loop.TheStallLine)
	if first != loop.NudgeAfterRoundsWithoutProgress {
		t.Errorf("the stall line first rode on model call %d, want call %d, the one after ten rounds without progress", first, loop.NudgeAfterRoundsWithoutProgress)
	}
	firstRewind, _ := requestsCarrying(built, loop.TheRewindLine)
	if firstRewind != loop.RewindAfterRoundsWithoutProgress {
		t.Errorf("the rewind line first rode on model call %d, want call %d, the one after twenty rounds without progress", firstRewind, loop.RewindAfterRoundsWithoutProgress)
	}
	if outcome.Status != contract.StatusStopped || !strings.Contains(outcome.StopLine, "without progress") {
		t.Errorf("the task ended %q on %q, want it stopped for rounds without progress after the cleared conversation stalled again", outcome.Status, outcome.StopLine)
	}
	if calls := len(built.model.Requests()); calls < 2*loop.RewindAfterRoundsWithoutProgress || calls > 2*loop.RewindAfterRoundsWithoutProgress+1 {
		t.Errorf("the model was called %d times, want %d rounds of edits and at most one call for the stopped report", calls, 2*loop.RewindAfterRoundsWithoutProgress)
	}
	held := built.held(t, outcome.TaskID)
	stalled := 0
	for _, failure := range held.Lessons.Failures {
		if strings.Contains(failure.Text, "rounds in which no test went green") {
			stalled++
		}
	}
	if stalled != 1 {
		t.Errorf("the record's failures read %+v, want one line for the stall the rewind cleared", held.Lessons.Failures)
	}
}

// TestProgressStartsTheCountAgain holds that the meter measures the work and
// not the clock: a test run that improves, a new file, a page that changed
// under an action, or a mark on the record starts the count again.
func TestProgressStartsTheCountAgain(t *testing.T) {
	steps := []testkit.Step{}
	for at := 1; at <= loop.NudgeAfterRoundsWithoutProgress-2; at++ {
		steps = append(steps, anEditRound(at))
	}
	steps = append(steps, callStep("I will run the tests.", callFor("t1", contract.ToolShell, `{"command":"node --test"}`)))
	for at := 20; at < 20+loop.NudgeAfterRoundsWithoutProgress-2; at++ {
		steps = append(steps, anEditRound(at))
	}
	steps = append(steps, callStep("I will run the tests again.", callFor("t2", contract.ToolShell, `{"command":"node --test"}`)))
	for at := 40; at < 40+loop.NudgeAfterRoundsWithoutProgress-2; at++ {
		steps = append(steps, anEditRound(at))
	}
	steps = append(steps, answerStep("Green. What changed: the engine. What I checked: the tests. What is left: nothing."))
	edits := []string{}
	for range 3 * loop.NudgeAfterRoundsWithoutProgress {
		edits = append(edits, "edited /game/src/engine.js by 1 line")
	}
	// Once the model has run the tests, the harness runs them again after
	// every edit, so the shell answers the model's two runs and every run
	// after an edit: the same red each time, which is no progress.
	twoRed := "finished with exit code 1\n✖ clears a row (1ms)\n✖ spawns (1ms)\nℹ tests 10\nℹ pass 8\nℹ fail 2\nexit 1"
	oneRed := "finished with exit code 1\n✖ clears a row (1ms)\nℹ tests 10\nℹ pass 9\nℹ fail 1\nexit 1"
	runs := []string{twoRed}
	for range loop.NudgeAfterRoundsWithoutProgress - 2 {
		runs = append(runs, twoRed)
	}
	runs = append(runs, oneRed)
	for range 2 * (loop.NudgeAfterRoundsWithoutProgress - 2) {
		runs = append(runs, oneRed)
	}
	built := newHarness(t, steps,
		scriptedTool(contract.ToolEdit, edits...),
		scriptedTool(contract.ToolShell, runs...))

	outcome := built.ask(t, "make the tests pass")

	if _, count := requestsCarrying(built, loop.TheStallLine); count != 0 {
		t.Errorf("the stall line was said %d times, and a test run that improves every eight rounds is progress", count)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done in the model's own words", outcome.Status)
	}
	if shown := wholeRequestText(built.model.Requests()[5]); !strings.Contains(shown, "rounds since progress: 5") {
		t.Errorf("the sixth call does not carry the meter in the situation, and the request reads:\n%s", shown)
	}
}

// TestPollingALongCommandIsNotARoundWithoutProgress keeps a build that takes a
// while from reading as a stall: a round that only asks after a running
// command is waiting, and is not counted.
func TestPollingALongCommandIsNotARoundWithoutProgress(t *testing.T) {
	steps := []testkit.Step{}
	answers := []string{}
	for at := 1; at <= loop.NudgeAfterRoundsWithoutProgress+2; at++ {
		steps = append(steps, callStep("I will wait for the build.", callFor(fmt.Sprintf("p%d", at), contract.ToolShell, `{"action":"poll","id":"p1"}`)))
		answers = append(answers, fmt.Sprintf("p1 is still running, %d seconds in", 12*at))
	}
	steps = append(steps, answerStep("The build finished. What changed: nothing. What I checked: the build. What is left: nothing."))
	built := newHarness(t, steps, scriptedTool(contract.ToolShell, answers...))

	built.ask(t, "wait for the build")

	if _, count := requestsCarrying(built, loop.TheStallLine); count != 0 {
		t.Errorf("the stall line was said %d times while a build was polled, and waiting is not a stall", count)
	}
}

// TestAskingThePageSomethingNewIsProgress is the fifth game build's play-test
// once its fix landed: twelve rounds of browser_read with a different ask each
// time, moving the piece, dropping it, pausing, restarting and forcing the
// dragon, every one answered with new state, and the meter read "rounds since
// progress: 12" because a browser read was not on its list of reads. A read
// of the page with a new intent is a read of something new; the same intent
// again is not.
func TestAskingThePageSomethingNewIsProgress(t *testing.T) {
	steps := []testkit.Step{}
	answers := []string{}
	for at := 1; at <= loop.NudgeAfterRoundsWithoutProgress+2; at++ {
		steps = append(steps, callStep("I will ask the page.", callFor(fmt.Sprintf("a%d", at), "browser_read",
			fmt.Sprintf(`{"intent":"check the state after move %d","ask":"window.__engine.state"}`, at))))
		answers = append(answers, fmt.Sprintf("Tater Tots Tetris\nhttp://localhost:8091/\nthe page answered: \"move %d\"\n", at))
	}
	steps = append(steps, answerStep("The page moved. What changed: nothing. What I checked: the state. What is left: nothing."))
	built := newHarness(t, steps, scriptedTool("browser_read", answers...))

	built.ask(t, "play-test the game")

	if _, count := requestsCarrying(built, loop.TheStallLine); count != 0 {
		t.Errorf("the stall line was said %d times over twelve reads of the page that each asked something new", count)
	}

	same := []testkit.Step{}
	sameAnswers := []string{}
	// The same intent every round, with an ask that differs so that the
	// same-call guard lets the calls through: the meter, not the guard, is
	// what this half proves.
	for at := 1; at <= loop.NudgeAfterRoundsWithoutProgress+2; at++ {
		same = append(same, callStep("I will read the page.", callFor(fmt.Sprintf("s%d", at), "browser_read",
			fmt.Sprintf(`{"intent":"read the page","ask":"window.__probe%d"}`, at))))
		sameAnswers = append(sameAnswers, "Tater Tots Tetris\nhttp://localhost:8091/\ne1 heading\n")
	}
	same = append(same, answerStep("The page is the same. What changed: nothing. What I checked: the page. What is left: nothing."))
	stuck := newHarness(t, same, scriptedTool("browser_read", sameAnswers...))

	stuck.ask(t, "play-test the game")

	if _, count := requestsCarrying(stuck, loop.TheStallLine); count == 0 {
		t.Error("twelve reads of the same page with the same intent never drew the stall line")
	}
}
