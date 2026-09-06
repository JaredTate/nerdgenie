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
	theRedRun   = "finished with exit code 1\n✖ clears a row (1ms)\n✖ spawns (1ms)\nℹ tests 10\nℹ pass 8\nℹ fail 2\nexit 1"
	theGreenRun = "finished with exit code 0\nℹ tests 10\nℹ pass 10\nℹ fail 0\nexit 0"
)

// TestTheTestsRunThemselvesAfterAWriteOnceTheModelHasRunThemOnce is the
// largest measured saving on the plan. On the fifth game build the model never
// put two calls in one reply, so every test run was a round of its own,
// eighteen percent of the run. Once the model has run the tests once, the
// harness runs the same command after every write or edit and puts the line
// on the change's own result.
func TestTheTestsRunThemselvesAfterAWriteOnceTheModelHasRunThemOnce(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theRedRun, theGreenRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.", callFor("c1", contract.ToolShell, `{"command":"cd ~/game && node --test test/"}`)),
		callStep("I will fix the engine.", callFor("c2", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = 1;"}`)),
		answerStep("Green. What changed: the engine. What I checked: the tests. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 21 bytes"))

	built.ask(t, "make the tests pass")

	inputs := shell.Inputs()
	if len(inputs) != 2 || !strings.Contains(string(inputs[1]), "node --test test/") {
		t.Fatalf("the shell ran %d times with %s, want twice: the model's run and the harness's run of the same command after the write", len(inputs), inputs)
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
// has shown: with no test run seen yet, a write is a write.
func TestNoTestCommandSeenMeansNoRunAfterAWrite(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theGreenRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will write the engine.", callFor("c1", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = 1;"}`)),
		answerStep("Written. What changed: the engine. What I checked: nothing. What is left: the tests."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 21 bytes"))

	built.ask(t, "write the engine")

	if len(shell.Inputs()) != 0 {
		t.Errorf("the shell ran %d times, want none: no test command has been seen", len(shell.Inputs()))
	}
	if strings.Contains(wholeRequestText(built.model.Requests()[1]), loop.TheTestsAfterAChange) {
		t.Error("a tests line rode on the write with no test command known")
	}
}
