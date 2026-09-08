package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheSameResultOverAndOverBuysARethinkTheMeterWouldMiss is the same-wall
// detector's whole promise, and it is the gap the meter and the guard both
// leave. The model runs a check that comes back the same every time — twelve
// failing — and between each run it writes a new file, which the progress meter
// reads as progress and the guard reads as new work, so neither ever fires. On
// 8 September 2026 the tic-tac-toe-against-the-computer task did exactly this
// for ninety rounds with a frozen test suite. Because a new file is written
// every round, the meter's count never climbs past one, so a rethink here can
// only be the wall's: it fires the moment the same result has come back
// SameWallRecurrences times, far inside the twenty rounds the meter needs.
func TestTheSameResultOverAndOverBuysARethinkTheMeterWouldMiss(t *testing.T) {
	steps := []testkit.Step{}
	checkOutputs := []string{}
	editOutputs := []string{}
	for at := 1; at <= loop.SameWallRecurrences; at++ {
		steps = append(steps, callStep("I will run the checks.",
			callFor(fmt.Sprintf("c%d", at), "check", fmt.Sprintf(`{"intent":"run the checks, try %d"}`, at))))
		checkOutputs = append(checkOutputs, "12 failing of 42")
		if at < loop.SameWallRecurrences {
			steps = append(steps, callStep("I will try one more change.",
				callFor(fmt.Sprintf("e%d", at), contract.ToolEdit, fmt.Sprintf(`{"path":"/game/file%d.js","old":"a","new":"b"}`, at))))
			editOutputs = append(editOutputs, fmt.Sprintf("wrote /game/file%d.js, one new file", at))
		}
	}
	// The wall trips on the last check; the next model call is the rethink's
	// question, and the fresh window on its answer takes the closing answer.
	steps = append(steps,
		aRethinkAnswer("the same check came back twelve failing every time",
			"the check reads something the edits never touch",
			"the check runs against a stale build, or the edits are in the wrong file",
			"run the check with its output shown in full"),
		answerStep("Done. What changed: the file the check actually reads. What I checked: the check passes. What is left: nothing."))

	built := newHarness(t, steps,
		scriptedTool("check", checkOutputs...),
		scriptedTool(contract.ToolEdit, editOutputs...))

	outcome := built.ask(t, "make the check pass")

	// The rethink fired, and it fired far before the meter could: the meter
	// needs twenty rounds without progress, and there was progress — a new
	// file — every other round, so its count never reached even two.
	firstRethink, rethinks := requestsCarrying(built, loop.TheRethinkLine)
	if rethinks == 0 {
		t.Fatalf("the same result came back %d times and no rethink fired; the wall never caught what the meter cannot", loop.SameWallRecurrences)
	}
	if firstRethink >= loop.RewindAfterRoundsWithoutProgress {
		t.Errorf("the rethink first rode on model call %d, at or past the meter's %d-round mark; the wall is meant to fire long before the meter", firstRethink, loop.RewindAfterRoundsWithoutProgress)
	}
	// The wall drove the rethink through the record like any stall: a
	// Rethink decision is written from the answer's Next line.
	held := built.held(t, outcome.TaskID)
	rethought := false
	for _, decision := range held.Lessons.Decisions {
		if strings.HasPrefix(decision.Text, "Rethink:") {
			rethought = true
		}
	}
	if !rethought {
		t.Errorf("the record holds no Rethink decision, so the wall's stall did not run the rethink: %+v", held.Lessons.Decisions)
	}
}

// TestTheSameWallHitTooManyTimesStopsTheTask: a wall the rethink never breaks
// cannot be rethought forever. Each episode is SameWallRecurrences rounds that
// run the same frozen check and change one new file, which trips the wall once
// and shares the meter's stall budget; the first StallsBeforeStop-1 episodes
// each buy a rethink, and the last stops the task with the wall's own line, so
// the job takes it as a stopped task and goes on rather than grinding. The
// check and the edit ride in one round so the rethink closes the edit, not the
// check, and the next episode's checks still run.
func TestTheSameWallHitTooManyTimesStopsTheTask(t *testing.T) {
	steps := []testkit.Step{}
	checkOutputs, editOutputs := []string{}, []string{}
	fileNo := 0
	for episode := 1; episode <= loop.StallsBeforeStop; episode++ {
		for at := 1; at <= loop.SameWallRecurrences; at++ {
			fileNo++
			steps = append(steps, callStep("Check, then change one thing.",
				callFor(fmt.Sprintf("c%d_%d", episode, at), "check", fmt.Sprintf(`{"intent":"check %d.%d"}`, episode, at)),
				callFor(fmt.Sprintf("e%d_%d", episode, at), contract.ToolEdit, fmt.Sprintf(`{"path":"/game/file%d.js","old":"a","new":"b"}`, fileNo))))
			checkOutputs = append(checkOutputs, "12 failing of 42")
			editOutputs = append(editOutputs, fmt.Sprintf("wrote /game/file%d.js, one new file", fileNo))
		}
		if episode < loop.StallsBeforeStop {
			steps = append(steps, aRethinkAnswer(
				fmt.Sprintf("the check is frozen at twelve failing, round %d", episode),
				fmt.Sprintf("the check reads a stale build, %d", episode),
				fmt.Sprintf("the build is cached, or the wrong files change, %d", episode),
				fmt.Sprintf("clear the cache and rebuild, attempt %d", episode)))
		}
	}
	steps = append(steps, answerStep("Somehow done."))

	built := newHarness(t, steps, scriptedTool("check", checkOutputs...), scriptedTool(contract.ToolEdit, editOutputs...))

	outcome := built.ask(t, "make the check pass")

	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped after the same wall was hit %d times: %s", outcome.Status, loop.StallsBeforeStop, outcome.Report)
	}
	if !strings.Contains(outcome.StopLine, "the same result came back") {
		t.Errorf("the task stopped on %q, want the wall's own stop line", outcome.StopLine)
	}
}

// TestAPollLoopDoesNotFireTheWall: a poll's repeats are waiting for something
// to finish, not a wall, so a model polling the same build over and over — even
// masked by an edit between polls, which is where the guard cannot see it —
// never buys a rethink. This proves the isAPollOrTail exemption through the
// real loop: without it these identical poll results would recur and fire.
func TestAPollLoopDoesNotFireTheWall(t *testing.T) {
	steps := []testkit.Step{}
	pollOutputs, editOutputs := []string{}, []string{}
	for at := 1; at <= loop.SameWallRecurrences+1; at++ {
		steps = append(steps, callStep("Is it done yet?",
			callFor(fmt.Sprintf("p%d", at), contract.ToolShell, `{"action":"poll","command":"tail -f build.log"}`)))
		pollOutputs = append(pollOutputs, "still running")
		steps = append(steps, callStep("One change while I wait.",
			callFor(fmt.Sprintf("e%d", at), contract.ToolEdit, fmt.Sprintf(`{"path":"/game/file%d.js","old":"a","new":"b"}`, at))))
		editOutputs = append(editOutputs, fmt.Sprintf("wrote /game/file%d.js", at))
	}
	steps = append(steps, answerStep("The build is running. What changed: nothing. What I checked: the build. What is left: nothing."))

	built := newHarness(t, steps,
		testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, pollOutputs...),
		scriptedTool(contract.ToolEdit, editOutputs...))

	outcome := built.ask(t, "wait for the build")

	if _, rethinks := requestsCarrying(built, loop.TheRethinkLine); rethinks != 0 {
		t.Errorf("a poll loop fired the wall %d times; a poll's repeats are waiting, not a wall", rethinks)
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the poll loop stopped the task on %q; polling is not a stall", outcome.StopLine)
	}
}
