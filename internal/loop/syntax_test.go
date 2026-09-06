package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theBracketError is what node --check prints for a file with an unbalanced
// bracket, as the shell tool hands it back.
const theBracketError = "finished with exit code 1\n/game/diag2.js:52\n]);\n^\n\nSyntaxError: Unexpected token ']'\n    at compileSourceTextModule (node:internal/modules/esm/utils:318:16)\nexit 1"

// TestAWriteThatDoesNotParseSaysSoOnItsOwnResult is the fifth game build's
// play-test task: it wrote a diagnostic script with an unbalanced bracket and
// ran it four times, reading the same syntax error each time, five rounds on
// a mistake the language's own checker finds in a tenth of a second. After
// every write or edit of a file the checker knows, the harness runs the
// checker and puts its answer on the change's own result.
func TestAWriteThatDoesNotParseSaysSoOnItsOwnResult(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theBracketError)
	built := newHarness(t, []testkit.Step{
		callStep("I will write the diagnostic.", callFor("c1", contract.ToolWrite, `{"path":"/game/diag2.js","content":"const x = [1, 2]);"}`)),
		answerStep("Written. What changed: the script. What I checked: nothing. What is left: running it."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/diag2.js, 19 bytes"))

	built.ask(t, "write a diagnostic")

	inputs := shell.Inputs()
	if len(inputs) != 1 || !strings.Contains(string(inputs[0]), "node --check") || !strings.Contains(string(inputs[0]), "/game/diag2.js") {
		t.Fatalf("the shell ran %d times with %s, want once: node --check on the file just written", len(inputs), inputs)
	}
	shown := wholeRequestText(built.model.Requests()[1])
	if !strings.Contains(shown, loop.TheFileDoesNotParse+"SyntaxError: Unexpected token ']'") {
		t.Errorf("the write's result does not carry the checker's line, and the request after it reads:\n%s", shown)
	}
}

// TestAWriteThatParsesSaysSoAndAFileWithNoCheckerIsLeftAlone keeps the rule
// to what a checker can say: a file that parses gets one word, and a file in
// a language with no checker gets nothing and costs no shell run.
func TestAWriteThatParsesSaysSoAndAFileWithNoCheckerIsLeftAlone(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, "finished with exit code 0\nexit 0")
	built := newHarness(t, []testkit.Step{
		callStep("I will write the notes.", callFor("c1", contract.ToolWrite, `{"path":"/game/NOTES.md","content":"# notes"}`)),
		callStep("I will write the engine.", callFor("c2", contract.ToolWrite, `{"path":"/game/src/engine.py","content":"x = 1"}`)),
		answerStep("Written. What changed: two files. What I checked: nothing. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /game/NOTES.md, 7 bytes", "created /game/src/engine.py, 5 bytes"))

	built.ask(t, "write the files")

	inputs := shell.Inputs()
	if len(inputs) != 1 || !strings.Contains(string(inputs[0]), "py_compile") {
		t.Fatalf("the shell ran %d times with %s, want once: the checker for the Python file and none for the notes", len(inputs), inputs)
	}
	afterNotes := wholeRequestText(built.model.Requests()[1])
	if strings.Contains(afterNotes, loop.TheFileParses) || strings.Contains(afterNotes, loop.TheFileDoesNotParse) {
		t.Errorf("a file with no checker got a checker's line, and the request reads:\n%s", afterNotes)
	}
	if afterEngine := wholeRequestText(built.model.Requests()[2]); !strings.Contains(afterEngine, loop.TheFileParses) {
		t.Errorf("the Python file that parses got no word, and the request reads:\n%s", afterEngine)
	}
}
