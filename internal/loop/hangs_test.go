package loop_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theHungPageMessage is what the browser tool says when a click leaves the
// page's own script busy for good.
const theHungPageMessage = "cannot click the element e3: The page could not be read after 3000 milliseconds: the page did not answer the scan call: it was still busy after 5000 milliseconds. That is the page's own script keeping it busy, which is code that does not yield, most often an endless loop that starts on this action."

// theGamePage is a page with one inline loop and one script of its own.
const theGamePage = "<!doctype html>\n<title>Tater Tots Tetris</title>\n<script src=\"js/main.js\"></script>\n<script>\nfor (const tot of tots) { fry(tot); }\n</script>\n<button>START GAME</button>\n"

// theGameScript is the script with the loop that never yields on line 17,
// behind fourteen counted for loops that draw the board, the way the real
// game's script had its ghost-piece while behind every for of its renderer.
var theGameScript = strings.Repeat("for (let i = 0; i < n; i++) draw(i);\n", 14) +
	"function ghost(p) {\n  let ghostY = p.y;\n  while (engine.board.canPlace(p.cells().map(([cx, cy]) => [cx, cy + 1]), 0, 0)) {\n    ghostY++;\n  }\n}\nwhile (arrTimer >= CONFIG.ARR) {\n  arrTimer -= CONFIG.ARR;\n}\n"

// aServedGame serves the page and its script the way the model's own server
// does, from this machine.
func aServedGame(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.html":
			_, _ = w.Write([]byte(theGamePage))
		case "/js/main.js":
			_, _ = w.Write([]byte(theGameScript))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// aPlayTest scripts the play-test round: open the page, click Start, and read
// the answer, with the browser tool answering the open with the page's address
// and the click with the hung-page message.
func aPlayTest(t *testing.T, address string, clickAnswer string) (*harness, *testkit.ScriptedTool) {
	t.Helper()
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, "finished with exit code 0\nexit 0")
	opened := "Tater Tots Tetris\n" + address + "\ntab t1\ne3 button \"START GAME\"\n"
	built := newHarness(t, []testkit.Step{
		callStep("I will open the game.", callFor("c1", "browser_open", `{"url":"`+address+`","intent":"open the game"}`)),
		callStep("I will click Start.", callFor("c2", "browser_click", `{"element":"e3","expectation":"the game starts"}`)),
		answerStep("The page hangs in the ghost loop. What changed: nothing. What I checked: the loops. What is left: the fix."),
	}, shell, scriptedTool("browser_open", opened), scriptedTool("browser_click", clickAnswer))
	return built, shell
}

// TestAHungPageListsTheLoopsInTheScriptsThePageRuns is the fifth game build's
// two hours: the Start button ran a loop whose condition never changed, the
// browser tool said so, and the model wrote nine play-test drivers rather than
// reading the three while loops in the game's script, which an earlier task of
// the job had written. When the browser reports a page whose script never
// yields, the harness reads the page and the scripts it loads from the local
// server and lists their loops, by file and line, on the same result, the
// while loops first: the first list the live run got was twelve counted for
// loops of the renderer, and the ghost-piece while on line 495 fell off its
// end.
func TestAHungPageListsTheLoopsInTheScriptsThePageRuns(t *testing.T) {
	server := aServedGame(t)
	built, shell := aPlayTest(t, server.URL+"/index.html", theHungPageMessage)

	built.ask(t, "play-test the game")

	if len(shell.Inputs()) != 0 {
		t.Errorf("the shell ran %d times, want never: the scripts are read from the server, not searched on disk", len(shell.Inputs()))
	}
	shown := wholeRequestText(built.model.Requests()[2])
	for _, words := range []string{loop.TheLoopsLine, "main.js:17: while (engine.board.canPlace(p.cells().map(([cx, cy]) => [cx, cy + 1]), 0, 0)) { " + loop.TheNeverChangesMark, "main.js:21: while (arrTimer", "index.html:5: for (const tot of tots)"} {
		if !strings.Contains(shown, words) {
			t.Errorf("the click's result does not carry %q, and the request after it reads:\n%s", words, shown)
		}
	}
	if strings.Contains(shown, "main.js:21: while (arrTimer >= CONFIG.ARR) { "+loop.TheNeverChangesMark) {
		t.Errorf("the loop that takes from arrTimer in its body is marked as never changing:\n%s", shown[strings.Index(shown, loop.TheLoopsLine):])
	}
	if strings.Index(shown, "main.js:17: while") > strings.Index(shown, "main.js:1: for") || strings.Index(shown, "main.js:21: while") > strings.Index(shown, "index.html:5: for") {
		t.Errorf("the while loops do not come before the for loops:\n%s", shown[strings.Index(shown, loop.TheLoopsLine):])
	}
	if !strings.Contains(shown, "and 5 more for loops") {
		t.Errorf("the list does not say how many for loops fell off its end:\n%s", shown[strings.Index(shown, loop.TheLoopsLine):])
	}
	situation := shown[strings.Index(shown, "## Work"):strings.Index(shown, "## Lessons")]
	if !strings.Contains(situation, loop.TheHungPageFact+"main.js:17: while (engine.board.canPlace") {
		t.Errorf("the situation does not keep the marked loop in front of the model, and it reads:\n%s", situation)
	}
}

// TestAnOrdinaryBrowserResultListsNoLoops keeps the reading to the hang: a
// click that worked, or that failed for any other reason, gets no list.
func TestAnOrdinaryBrowserResultListsNoLoops(t *testing.T) {
	server := aServedGame(t)
	built, _ := aPlayTest(t, server.URL+"/index.html", "what was expected happened\n---\nTater Tots Tetris\n"+server.URL+"/index.html\n")

	built.ask(t, "play-test the game")

	if shown := wholeRequestText(built.model.Requests()[2]); strings.Contains(shown, loop.TheLoopsLine) {
		t.Errorf("a click that worked got a list of loops:\n%s", shown)
	}
}

// TestAPageOnAnotherMachineListsNoLoops keeps the harness off the wider web:
// only a page served from this machine, or a file, is read for its scripts.
func TestAPageOnAnotherMachineListsNoLoops(t *testing.T) {
	built, _ := aPlayTest(t, "https://example.com/index.html", theHungPageMessage)

	built.ask(t, "play-test the game")

	if shown := wholeRequestText(built.model.Requests()[2]); strings.Contains(shown, loop.TheLoopsLine) {
		t.Errorf("a page on another machine got a list of loops:\n%s", shown)
	}
}

// TestAPageOnDiskListsItsLoops covers the page opened as a file, which the
// skill allows: the file and the scripts beside it are read from disk.
func TestAPageOnDiskListsItsLoops(t *testing.T) {
	folder := t.TempDir()
	writeFile(t, folder+"/index.html", theGamePage)
	writeFile(t, folder+"/js/main.js", theGameScript)
	built, _ := aPlayTest(t, "file://"+folder+"/index.html", theHungPageMessage)

	built.ask(t, "play-test the game")

	shown := wholeRequestText(built.model.Requests()[2])
	for _, words := range []string{loop.TheLoopsLine, "main.js:17: while (engine.board.canPlace"} {
		if !strings.Contains(shown, words) {
			t.Errorf("the click's result does not carry %q, and the request after it reads:\n%s", words, shown)
		}
	}
}

// writeFile writes one file under the test's folder, making the folders it
// sits in.
func writeFile(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("cannot make the folder for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}
