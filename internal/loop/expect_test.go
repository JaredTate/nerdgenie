package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The expect line is the harness keeping the books: the model says what a
// command, a write or an edit should show, and the harness says whether it
// did, marks the hit as progress, and writes the miss into the record as a
// failure with its cause. On 6 September 2026, 297 rounds of 2,173 did nothing
// but write the record, and 47 test rounds reran tests the harness had just
// run to learn what an expectation would have told the model.

func TestACommandThatMeetsItsExpectationSaysSo(t *testing.T) {
	shell := scriptedTool(contract.ToolShell, "finished with exit code 0\nhello\nexit 0")
	built := newHarness(t, []testkit.Step{
		callStep("I will greet.", callFor("c1", contract.ToolShell, `{"command":"echo hello","expect":"exit 0"}`)),
		answerStep("Greeted. What changed: nothing. What I checked: the greeting. What is left: nothing."),
	}, shell)

	built.ask(t, "say hello")

	after := wholeRequestText(built.model.Requests()[1])
	if !strings.Contains(after, "finished with exit code 0 (as expected)") {
		t.Errorf("the result's first line does not say the expectation was met:\n%s", after)
	}
	for _, failure := range built.held(t, "1").Lessons.Failures {
		t.Errorf("a met expectation wrote the failure %+v", failure)
	}
}

func TestACommandThatMissesItsExpectationIsAFailureWithItsCause(t *testing.T) {
	shell := scriptedTool(contract.ToolShell, "finished with exit code 1\nno such file\nexit 1")
	built := newHarness(t, []testkit.Step{
		callStep("I will list it.", callFor("c1", contract.ToolShell, `{"command":"ls missing","expect":"exit 0"}`)),
		answerStep("Missing. What changed: nothing. What I checked: the folder. What is left: nothing."),
	}, shell)

	built.ask(t, "list the folder")

	after := wholeRequestText(built.model.Requests()[1])
	if !strings.Contains(after, "not as expected: expected exit 0, got exit 1") {
		t.Errorf("the result's first line does not say what was expected and what came:\n%s", after)
	}
	failures := built.held(t, "1").Lessons.Failures
	if len(failures) != 1 || failures[0].Text != "expected exit 0, got exit 1" || !strings.Contains(failures[0].Cause, "ls missing") {
		t.Errorf("the record holds %+v, want one failure saying expected exit 0, got exit 1, caused by the ls call", failures)
	}
}

func TestATestRunIsCheckedAgainstAnExpectedCount(t *testing.T) {
	shell := scriptedTool(contract.ToolShell, theRedRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.", callFor("c1", contract.ToolShell, `{"command":"node --test test/","expect":"all passing"}`)),
		answerStep("Red. What changed: nothing. What I checked: the tests. What is left: two tests."),
	}, shell)

	built.ask(t, "run the tests")

	after := wholeRequestText(built.model.Requests()[1])
	if !strings.Contains(after, "not as expected: expected all passing, got 2 failing of 10") {
		t.Errorf("the red run was not checked against the expectation:\n%s", after)
	}
}

func TestAWriteIsCheckedAgainstTheTestsTheHarnessRanAfterIt(t *testing.T) {
	// The shell answers the model's red run, the syntax check on the written
	// file, and the harness's own green run after the write, which is what the
	// expectation "tests 8 to 10" is checked against: ten tests, all passing.
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theRedRun, theFileChecksOut, theGreenRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.", callFor("c1", contract.ToolShell, `{"command":"cd ~/game && node --test test/"}`)),
		callStep("I will fix the engine.", callFor("c2", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = 1;","expect":"tests 8 to 10"}`)),
		answerStep("Green. What changed: the engine. What I checked: the tests. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 21 bytes"))

	built.ask(t, "make the tests pass")

	after := wholeRequestText(built.model.Requests()[2])
	if !strings.Contains(after, "created /game/src/engine.js, 21 bytes (as expected)") {
		t.Errorf("the write's result does not say its expectation was met:\n%s", after)
	}
}

func TestAWriteExpectedToParseIsCheckedAgainstTheSyntaxCheck(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, "finished with exit code 1\nSyntaxError: unexpected token\nexit 1")
	built := newHarness(t, []testkit.Step{
		callStep("I will write the engine.", callFor("c1", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = ;","expect":"parses"}`)),
		answerStep("Written. What changed: the engine. What I checked: nothing. What is left: the tests."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 19 bytes"))

	built.ask(t, "write the engine")

	after := wholeRequestText(built.model.Requests()[1])
	if !strings.Contains(after, "not as expected: expected parses, got a file that does not parse") {
		t.Errorf("the broken file was not checked against the expectation:\n%s", after)
	}
	if failures := built.held(t, "1").Lessons.Failures; len(failures) != 1 || !strings.Contains(failures[0].Cause, "engine.js") {
		t.Errorf("the record holds %+v, want one failure caused by the write of engine.js", failures)
	}
}

func TestAnExpectationTheHarnessCannotReadIsSaidSoOnce(t *testing.T) {
	shell := scriptedTool(contract.ToolShell, "finished with exit code 0\nexit 0", "finished with exit code 0\nexit 0")
	built := newHarness(t, []testkit.Step{
		callStep("I will try.", callFor("c1", contract.ToolShell, `{"command":"true","expect":"it works"}`)),
		callStep("I will try again.", callFor("c2", contract.ToolShell, `{"command":"true # again","expect":"it still works"}`)),
		answerStep("Done. What changed: nothing. What I checked: nothing. What is left: nothing."),
	}, shell)

	built.ask(t, "try it")

	first := wholeRequestText(built.model.Requests()[1])
	if !strings.Contains(first, loop.TheExpectationCouldNotBeChecked) {
		t.Errorf("the first unreadable expectation drew no note:\n%s", first)
	}
	second := wholeRequestText(built.model.Requests()[2])
	if strings.Count(second, loop.TheExpectationCouldNotBeChecked) != 1 {
		t.Errorf("the note was said %d times over two unreadable expectations, want once", strings.Count(second, loop.TheExpectationCouldNotBeChecked))
	}
	for _, failure := range built.held(t, "1").Lessons.Failures {
		t.Errorf("an unreadable expectation wrote the failure %+v", failure)
	}
}

func TestAContainsExpectationReadsTheResultText(t *testing.T) {
	shell := scriptedTool(contract.ToolShell, "finished with exit code 0\nv24.18.0\nexit 0", "finished with exit code 0\nv24.18.0\nexit 0")
	built := newHarness(t, []testkit.Step{
		callStep("I will check node.", callFor("c1", contract.ToolShell, `{"command":"node --version","expect":"contains v24"}`)),
		callStep("I will check again.", callFor("c2", contract.ToolShell, `{"command":"node --version # again","expect":"contains v22"}`)),
		answerStep("Checked. What changed: nothing. What I checked: node. What is left: nothing."),
	}, shell)

	built.ask(t, "check node")

	texts := wholeRequestText(built.model.Requests()[2])
	if !strings.Contains(texts, "finished with exit code 0 (as expected)") {
		t.Errorf("the contained text was not read as a hit:\n%s", texts)
	}
	if !strings.Contains(texts, "not as expected: expected contains v22, got a result without it") {
		t.Errorf("the missing text was not read as a miss:\n%s", texts)
	}
}
