package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// writeAfterARun is the smallest task that writes a file after the model has
// run the tests once: the model runs the tests, the shell answers the run
// given, the model writes the path given, and the shell answers the syntax
// check and the harness's own rerun of the tests. It hands back the text of
// the request that carries the write's result.
func writeAfterARun(t *testing.T, run string, path string) string {
	t.Helper()
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, run, theFileChecksOut, run)
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.", callFor("c1", contract.ToolShell, `{"command":"cd ~/game && node --test test/"}`)),
		callStep("I will write the file.", callFor("c2", contract.ToolWrite, `{"path":"`+path+`","content":"export const x = 1;"}`)),
		answerStep("Done. What changed: one file. What I checked: the tests. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created "+path+", 21 bytes"))
	built.ask(t, "make the tests pass")
	return wholeRequestText(built.model.Requests()[2])
}

// TestACodeWriteAfterAGreenRunGetsTheTestsFirstLine: the suite is green, so no
// failing test covers the change the model is about to make, and the write's
// own result says so on its first line, where the record's summary shows it.
func TestACodeWriteAfterAGreenRunGetsTheTestsFirstLine(t *testing.T) {
	request := writeAfterARun(t, theGreenRun, "/game/src/engine.js")
	if !strings.Contains(request, loop.TheTestsFirstLine+"\ncreated /game/src/engine.js") {
		t.Errorf("the write's result does not open with the tests-first line, and the request reads:\n%s", request)
	}
}

// TestACodeWriteAfterARedRunGetsNoLine: a failing test covers the change, which
// is the order the rule asks for, so nothing is said.
func TestACodeWriteAfterARedRunGetsNoLine(t *testing.T) {
	request := writeAfterARun(t, theRedRun, "/game/src/engine.js")
	if strings.Contains(request, loop.TheTestsFirstLine) {
		t.Errorf("the write after a red run carries the tests-first line, and the request reads:\n%s", request)
	}
}

// TestATestFileNeverGetsTheLine: writing the test is the first move, so a
// test file, by its name or by its folder, in any language, gets no line.
func TestATestFileNeverGetsTheLine(t *testing.T) {
	for _, path := range []string{
		"/game/test/engine.test.js",
		"/game/src/engine.spec.ts",
		"/game/internal/loop/engine_test.go",
		"/game/tests/test_engine.py",
		"/game/spec/engine_spec.rb",
		"/game/src/__tests__/engine.js",
		"/game/specs/EngineTest.java",
	} {
		t.Run(path, func(t *testing.T) {
			request := writeAfterARun(t, theGreenRun, path)
			if strings.Contains(request, loop.TheTestsFirstLine) {
				t.Errorf("the write of the test file %s carries the tests-first line", path)
			}
		})
	}
}

// TestAFileThatIsNotCodeGetsNoLine: a page, a stylesheet, a manifest or a
// document is not a change a test covers, so nothing is said about it.
func TestAFileThatIsNotCodeGetsNoLine(t *testing.T) {
	for _, path := range []string{"/game/package.json", "/game/index.html", "/game/styles.css", "/game/README.md", "/game/assets/roar.wav"} {
		t.Run(path, func(t *testing.T) {
			request := writeAfterARun(t, theGreenRun, path)
			if strings.Contains(request, loop.TheTestsFirstLine) {
				t.Errorf("the write of %s carries the tests-first line, and it is not code", path)
			}
		})
	}
}

// TestBeforeAnyTestRunThereIsNoLine: the harness holds no test state before
// the model has run the tests once, so a scaffold is written freely.
func TestBeforeAnyTestRunThereIsNoLine(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theFileChecksOut)
	built := newHarness(t, []testkit.Step{
		callStep("I will write the engine.", callFor("c1", contract.ToolWrite, `{"path":"/game/src/engine.js","content":"export const x = 1;"}`)),
		answerStep("Done. What changed: the engine. What I checked: nothing yet. What is left: the tests."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/src/engine.js, 21 bytes"))

	built.ask(t, "start the game")

	if request := wholeRequestText(built.model.Requests()[1]); strings.Contains(request, loop.TheTestsFirstLine) {
		t.Errorf("a write before any test run carries the tests-first line, and the request reads:\n%s", request)
	}
}
