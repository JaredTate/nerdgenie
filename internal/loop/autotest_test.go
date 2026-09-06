package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theRedRun and theGreenRun are two runs of one small suite as Node prints
// them, before and after a change.
const (
	theRedRun        = "finished with exit code 1\n✖ clears a row (1ms)\n✖ spawns (1ms)\nℹ tests 10\nℹ pass 8\nℹ fail 2\nexit 1"
	theGreenRun      = "finished with exit code 0\nℹ tests 10\nℹ pass 10\nℹ fail 0\nexit 0"
	theFileChecksOut = "finished with exit code 0\nexit 0"
)

// TestTheTestsRunThemselvesAfterAWriteOnceTheModelHasRunThemOnce is the
// largest measured saving on the plan. On the fifth game build the model never
// put two calls in one reply, so every test run was a round of its own,
// eighteen percent of the run. Once the model has run the tests once, the
// harness runs the same command after every write or edit and puts the line
// on the change's own result.
func TestTheTestsRunThemselvesAfterAWriteOnceTheModelHasRunThemOnce(t *testing.T) {
	// The shell answers the model's run, the syntax check the harness runs
	// on the written file, and the harness's own test run after it.
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theRedRun, theFileChecksOut, theGreenRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.", callFor("c1", contract.ToolShell, `{"command":"cd ~/game && node --test test/"}`)),
		callStep("I will fix the engine.", callFor("c2", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = 1;"}`)),
		answerStep("Green. What changed: the engine. What I checked: the tests. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 21 bytes"))

	built.ask(t, "make the tests pass")

	inputs := shell.Inputs()
	if len(inputs) != 3 || !strings.Contains(string(inputs[2]), "node --test test/") {
		t.Fatalf("the shell ran %d times with %s, want three: the model's run, the syntax check, and the harness's run of the same command after the write", len(inputs), inputs)
	}
	last := wholeRequestText(built.model.Requests()[2])
	if !strings.Contains(last, "tests after this change: all 10 passing") {
		t.Errorf("the write's result does not carry the tests line, and the request after it reads:\n%s", last)
	}
	held := built.held(t, "1")
	for _, failure := range held.Lessons.Failures {
		t.Errorf("the record holds the failure %+v, and a run the model did not ask for writes none", failure)
	}
	if !strings.Contains(last, "tests: all 10 passing") {
		t.Errorf("the situation's tests line was not moved by the harness's run, and the request reads:\n%s", last)
	}
}

// TestNoTestCommandSeenMeansNoRunAfterAWrite keeps the rule to what the model
// has shown: with no test run seen yet, a write is a write, and the only
// shell run behind it is the syntax check.
func TestNoTestCommandSeenMeansNoRunAfterAWrite(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theFileChecksOut)
	built := newHarness(t, []testkit.Step{
		callStep("I will write the engine.", callFor("c1", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = 1;"}`)),
		answerStep("Written. What changed: the engine. What I checked: nothing. What is left: the tests."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 21 bytes"))

	built.ask(t, "write the engine")

	if inputs := shell.Inputs(); len(inputs) != 1 || !strings.Contains(string(inputs[0]), "node --check") {
		t.Errorf("the shell ran %d times with %s, want once for the syntax check and never for the tests, because no test command has been seen", len(inputs), inputs)
	}
	if strings.Contains(wholeRequestText(built.model.Requests()[1]), loop.TheTestsAfterAChange) {
		t.Error("a tests line rode on the write with no test command known")
	}
}
