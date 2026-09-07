package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheTestsFirstLineNamesTheProjectsTestFile: a write to a source file in
// the work folder, after a green run, opens with the line naming the test
// file that covers it, found by the project's own naming, so the model knows
// where the failing test goes without a search.
func TestTheTestsFirstLineNamesTheProjectsTestFile(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theGreenRun, theFileChecksOut, theGreenRun)
	var path string
	built := newHarness(t, nil, shell)
	path = filepath.Join(built.workFolder, "src", "engine.js")
	built.model = testkit.NewFakeModel(testkit.Script{Name: "test", ContextLength: 24000, Steps: []testkit.Step{
		callStep("I will run the tests.", callFor("c1", contract.ToolShell, `{"command":"npm test"}`)),
		callStep("I will write the file.", callFor("c2", contract.ToolWrite, `{"path":"`+path+`","content":"export const x = 1;"}`)),
		answerStep("Done. What changed: one file. What I checked: the tests. What is left: nothing."),
	}})
	built.tools = testkit.NewFakeToolRegistry(shell, scriptedTool(contract.ToolWrite, "created "+path+", 21 bytes"))
	made, err := loop.New(built.options())
	if err != nil {
		t.Fatalf("cannot build the loop from the fakes: %v", err)
	}
	built.loop = made
	if err := os.MkdirAll(filepath.Join(built.workFolder, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(built.workFolder, "tests", "engine.test.js"), []byte("test('x', () => {});\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	built.ask(t, "make the tests pass")

	request := wholeRequestText(built.model.Requests()[2])
	want := "tests first: no failing test covers this change; add a failing test to tests/engine.test.js first\ncreated " + path
	if !strings.Contains(request, want) {
		t.Errorf("the write's result does not name the test file, and the request reads:\n%s", request)
	}
}
