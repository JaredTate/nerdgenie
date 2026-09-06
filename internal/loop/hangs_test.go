package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theHungPageMessage is what the browser tool says when a click leaves the
// page's own script busy for good.
const theHungPageMessage = "cannot click the element e3: The page could not be read after 3000 milliseconds: the page did not answer the scan call: it was still busy after 5000 milliseconds. That is the page's own script keeping it busy, which is code that does not yield, most often an endless loop that starts on this action."

// theLoopsGrepFinds is what grep prints for the loops in the game's script.
const theLoopsGrepFinds = "finished with exit code 0\n/home/jared/game/main.js:495:      while (engine.board.canPlace(p.cells().map(([cx, cy]) => [cx, cy + 1]), 0, 0)) {\n/home/jared/game/main.js:713:      while (arrTimer >= CONFIG.ARR) {\n/home/jared/game/main.js:724:    while (softDropAccumulator >= 1) {\nexit 0"

// TestAHungPageListsTheLoopsInTheScriptsTheTaskChanged is the fifth game
// build's two hours: the Start button ran a loop whose condition never
// changed, the browser tool said so, and the model wrote nine play-test
// drivers rather than reading the three while loops in the one file it had
// written. When the browser reports a page whose script never yields, the
// harness lists the loops in the scripts this task changed, by file and line,
// on the same result.
func TestAHungPageListsTheLoopsInTheScriptsTheTaskChanged(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, "finished with exit code 0\nexit 0", theLoopsGrepFinds)
	built := newHarness(t, []testkit.Step{
		callStep("I will write the game.", callFor("c1", contract.ToolWrite, `{"path":"/home/jared/game/main.js","content":"while (x) {}"}`)),
		callStep("I will click Start.", callFor("c2", "browser_click", `{"element":"e3","expectation":"the game starts"}`)),
		answerStep("The page hangs in the ghost loop at line 495. What changed: nothing. What I checked: the loops. What is left: the fix."),
	}, shell, scriptedTool(contract.ToolWrite, "created /home/jared/game/main.js, 12 bytes"), scriptedTool("browser_click", theHungPageMessage))

	built.ask(t, "play-test the game")

	inputs := shell.Inputs()
	if len(inputs) != 2 || !strings.Contains(string(inputs[1]), "while") || !strings.Contains(string(inputs[1]), "/home/jared/game/main.js") {
		t.Fatalf("the shell ran %d times with %s, want the syntax check and then a search for loops in the file the task changed", len(inputs), inputs)
	}
	shown := wholeRequestText(built.model.Requests()[2])
	for _, words := range []string{loop.TheLoopsLine, "main.js:495", "main.js:713"} {
		if !strings.Contains(shown, words) {
			t.Errorf("the click's result does not carry %q, and the request after it reads:\n%s", words, shown)
		}
	}
}

// TestAnOrdinaryBrowserResultListsNoLoops keeps the search to the hang: a
// click that worked, or that failed for any other reason, gets no list.
func TestAnOrdinaryBrowserResultListsNoLoops(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, "finished with exit code 0\nexit 0")
	built := newHarness(t, []testkit.Step{
		callStep("I will write the game.", callFor("c1", contract.ToolWrite, `{"path":"/home/jared/game/main.js","content":"x"}`)),
		callStep("I will click Start.", callFor("c2", "browser_click", `{"element":"e3","expectation":"the game starts"}`)),
		answerStep("It started. What changed: the page. What I checked: the click. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /home/jared/game/main.js, 1 byte"), scriptedTool("browser_click", "what was expected happened\n---\nTater Tots Tetris\nhttp://localhost:8091/\n"))

	built.ask(t, "play-test the game")

	if len(shell.Inputs()) != 1 {
		t.Errorf("the shell ran %d times, want once for the syntax check and never for loops after a click that worked", len(shell.Inputs()))
	}
}
