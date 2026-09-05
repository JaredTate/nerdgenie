package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The three runs of a project's tests the scripted shell tool hands back: red
// twice on the same two tests, then green after the edit.
const (
	aRedRun   = "finished with exit code 1\n✖ clears a full row (0.5ms)\n✖ locks the piece (0.2ms)\nℹ tests 51\nℹ pass 49\nℹ fail 2\nexit 1"
	aGreenRun = "finished with exit code 0\n✔ clears a full row (0.5ms)\nℹ tests 51\nℹ pass 51\nℹ fail 0\nexit 0"
)

// aRedThenGreenBuild is a task that writes a test, sees it red twice, edits the
// code, and sees it green: the shape of every round of test-driven work.
func aRedThenGreenBuild(t *testing.T) *harness {
	t.Helper()
	return newHarness(t, []testkit.Step{
		callStep("I will write the test.", callFor("c1", contract.ToolWrite, `{"path":"/p/tests/board.test.js","content":"test"}`)),
		callStep("Now I will run the tests.", callFor("c2", contract.ToolShell, `{"command":"node --test tests/"}`)),
		callStep("I will run them once more to be sure.", callFor("c3", contract.ToolShell, `{"command":"node --test tests/ 2>&1 | tail -20"}`)),
		callStep("The board does not clear. I will fix it.", callFor("c4", contract.ToolEdit, `{"path":"/p/src/board.js","old":"a","new":"b"}`)),
		callStep("Now the tests again.", callFor("c5", contract.ToolShell, `{"command":"node --test tests/ --test-reporter spec"}`)),
		answerStep("The suite is green."),
	},
		scriptedTool(contract.ToolWrite, "wrote /p/tests/board.test.js, 120 bytes"),
		scriptedTool(contract.ToolShell, aRedRun, aRedRun, aGreenRun),
		scriptedTool(contract.ToolEdit, "edited /p/src/board.js by 1 line"),
	)
}

// TestTheHarnessWritesTheTestStateIntoTheRecord is the state the live game
// build never had. Its record listed forty-five test runs as "finished with
// exit code 0", because the model piped the runner through grep, and it held
// no plan, no done list and no failure, so the only memory of what was red was
// sixty thousand tokens of old output in the window. The harness can read a
// test runner's summary for itself, so it writes the state into the situation,
// puts the counts on the result's own line, and writes a failure with the edit
// that preceded it as the cause, once per change of what is failing.
func TestTheHarnessWritesTheTestStateIntoTheRecord(t *testing.T) {
	built := aRedThenGreenBuild(t)

	outcome := built.ask(t, "make the board clear full rows")

	held := built.held(t, outcome.TaskID)
	if !situationHolds(held, "tests: all 51 passing") {
		t.Errorf("the situation %v does not carry the state of the last test run", held.Work.Situation)
	}
	summaries := []string{}
	for _, result := range held.Work.Results {
		summaries = append(summaries, result.Summary)
	}
	joined := strings.Join(summaries, "\n")
	if !strings.Contains(joined, "tests: 2 failing of 51: clears a full row; locks the piece") {
		t.Errorf("the result lines read:\n%s\nwant the red run's own line to say what was failing", joined)
	}
	if !strings.Contains(joined, "tests: all 51 passing") {
		t.Errorf("the result lines read:\n%s\nwant the green run's own line to say so", joined)
	}
	if strings.Contains(joined, "finished with exit code") {
		t.Errorf("the result lines read:\n%s\nand a test run's line says the counts, not the exit code", joined)
	}
}

// TestARedRunAfterAnEditIsWrittenAsAFailureWithTheEditAsItsCause holds the
// lesson half: the record's failures are what keep a small model from trying
// the same thing again, and after a hundred and forty-five rounds the live
// record held none, because the model never wrote one. A red run that follows
// a change to a file is a failure the harness can see, and the file it changed
// is the cause it can name. The same red run seen again is not a second
// failure.
func TestARedRunAfterAnEditIsWrittenAsAFailureWithTheEditAsItsCause(t *testing.T) {
	built := aRedThenGreenBuild(t)

	outcome := built.ask(t, "make the board clear full rows")

	held := built.held(t, outcome.TaskID)
	if len(held.Lessons.Failures) != 1 {
		t.Fatalf("the record holds %d failures %+v, want the one red run after the write, and not the same run seen again",
			len(held.Lessons.Failures), held.Lessons.Failures)
	}
	failure := held.Lessons.Failures[0]
	for _, said := range []string{"2 failing", "clears a full row", "locks the piece"} {
		if !strings.Contains(failure.Text, said) {
			t.Errorf("the failure reads %q, want it to say %q", failure.Text, said)
		}
	}
	if !strings.Contains(failure.Cause, "board.test.js") {
		t.Errorf("the failure's cause reads %q, want it to name the file changed before the red run", failure.Cause)
	}
}

// TestATestRunWithNoChangeBeforeItIsNoFailure keeps the failure list to what
// the harness can stand behind: a red run with no edit since the last run is
// the same failure still standing, and a red run before any change at all is
// the state the task was given, not something it did.
func TestATestRunWithNoChangeBeforeItIsNoFailure(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("First I will see where the tests stand.", callFor("c1", contract.ToolShell, `{"command":"node --test tests/"}`)),
		callStep("Again, to be sure.", callFor("c2", contract.ToolShell, `{"command":"node --test tests/ 2>&1 | tail"}`)),
		answerStep("Two tests fail. Which should I fix first?"),
	}, scriptedTool(contract.ToolShell, aRedRun, aRedRun))

	outcome := built.ask(t, "see where the tests stand")

	held := built.held(t, outcome.TaskID)
	if len(held.Lessons.Failures) != 0 {
		t.Errorf("the record holds the failures %+v, want none: nothing was changed before the runs", held.Lessons.Failures)
	}
	if !situationHolds(held, "tests: 2 failing of 51") {
		t.Errorf("the situation %v does not carry the state of the test run", held.Work.Situation)
	}
}

// TestAFailureNamesAFewChangedFilesByTheirShortNames keeps the failure line
// readable: the live run's first failure named eight files by their full
// paths and ran to three hundred characters. A failure names the changed files
// by their last path element, at most three of them, and says how many more.
func TestAFailureNamesAFewChangedFilesByTheirShortNames(t *testing.T) {
	steps := []testkit.Step{}
	for at, name := range []string{"config", "rng", "pieces", "board", "engine"} {
		steps = append(steps, callStep("I will write "+name+".",
			callFor("c"+string(rune('1'+at)), contract.ToolWrite, `{"path":"/p/src/`+name+`.js","content":"x"}`)))
	}
	steps = append(steps,
		callStep("Now I will run the tests.", callFor("c9", contract.ToolShell, `{"command":"node --test tests/"}`)),
		answerStep("Two tests fail."),
	)
	built := newHarness(t, steps, scriptedTool(contract.ToolWrite, "wrote", "wrote", "wrote", "wrote", "wrote"), scriptedTool(contract.ToolShell, aRedRun))

	outcome := built.ask(t, "build the engine")

	held := built.held(t, outcome.TaskID)
	if len(held.Lessons.Failures) != 1 {
		t.Fatalf("the record holds %d failures, want one", len(held.Lessons.Failures))
	}
	failure := held.Lessons.Failures[0]
	for _, said := range []string{"config.js, rng.js, pieces.js and 2 more"} {
		if !strings.Contains(failure.Text, said) || !strings.Contains(failure.Cause, said) {
			t.Errorf("the failure reads %q with cause %q, want both to say %q", failure.Text, failure.Cause, said)
		}
	}
	if strings.Contains(failure.Text, "/p/src") {
		t.Errorf("the failure reads %q, and a full path is noise on a one-line lesson", failure.Text)
	}
}

// TestTheSituationNamesChangedFilesByTheirShortNames keeps the record's tail
// short: the situation's files line used to carry eight full paths, a
// quarter of a thousand tokens rewritten under the conversation every call,
// when the last path element says which file it was.
func TestTheSituationNamesChangedFilesByTheirShortNames(t *testing.T) {
	built := aRedThenGreenBuild(t)

	outcome := built.ask(t, "make the board clear full rows")

	held := built.held(t, outcome.TaskID)
	if !situationHolds(held, "files changed in this task: board.test.js, board.js") {
		t.Errorf("the situation %v does not name the changed files by their short names", held.Work.Situation)
	}
	if situationHolds(held, "/p/src/board.js") {
		t.Errorf("the situation %v carries a full path, and the short name says which file it was", held.Work.Situation)
	}
}
