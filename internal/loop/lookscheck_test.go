package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A looks check is run by the harness through the browser tools: the page
// is opened, then for each width resized, photographed and asked whether it
// overflows, and its errors read. The tools here are scripted the way the
// shows check's browser is.

func theLooksTools(t *testing.T, answers ...string) (page, resize, shot, read *testkit.ScriptedTool) {
	t.Helper()
	page = testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen, Description: "A browser the test scripted."}, "http://127.0.0.1:8097/ Tic Tac Toe\ne1 heading Tic Tac Toe")
	resize = testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserResize, Description: "A resize the test scripted."}, "the page is 1440 wide", "the page is 768 wide", "the page is 390 wide")
	shot = testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserScreenshot, Description: "A camera the test scripted."}, "the picture is saved at /pictures/one.png", "the picture is saved at /pictures/two.png", "the picture is saved at /pictures/three.png")
	read = testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserRead, Description: "A read the test scripted."}, answers...)
	return page, resize, shot, read
}

func TestALooksCheckPassesWhenEveryWidthFitsAndTheConsoleIsClean(t *testing.T) {
	page, resize, shot, read := theLooksTools(t,
		"Tic Tac Toe\nthe page answered: 0\n", "Tic Tac Toe\nthe page answered: 0\n", "Tic Tac Toe\nthe page answered: 0\n")
	built := aTaskWhoseDoneLineIsChecked(t, "The board fits every window. [looks: http://127.0.0.1:8097 at 1440, 768, 390]", nil, page, resize, shot, read)

	outcome := built.ask(t, "prove the sizes")

	theLineIsProvedByACheck(t, built, outcome, "check: looks: http://127.0.0.1:8097 at 1440, 768, 390")
	if len(page.Inputs()) != 1 || len(resize.Inputs()) != 3 || len(shot.Inputs()) != 3 || len(read.Inputs()) != 3 {
		t.Errorf("the browser was opened %d times, resized %d, photographed %d and read %d; want 1, 3, 3 and 3", len(page.Inputs()), len(resize.Inputs()), len(shot.Inputs()), len(read.Inputs()))
	}
	for at, width := range []string{`"width":1440`, `"width":768`, `"width":390`} {
		if !strings.Contains(string(resize.Inputs()[at]), width) || !strings.Contains(string(resize.Inputs()[at]), `"height":900`) {
			t.Errorf("resize %d was asked for %s, want %s at height 900", at+1, resize.Inputs()[at], width)
		}
	}
}

func TestALooksCheckFailsOnOverflowAndNamesTheWidth(t *testing.T) {
	page, resize, shot, read := theLooksTools(t,
		"Tic Tac Toe\nthe page answered: 0\n", "Tic Tac Toe\nthe page answered: 0\n", "Tic Tac Toe\nthe page answered: 42\n")
	built := aTaskWhoseDoneLineIsChecked(t, "The board fits every window. [looks: http://127.0.0.1:8097 at 1440, 768, 390]",
		[]testkit.Step{answerStep("I cannot make it fit. What should I do?")}, page, resize, shot, read)

	outcome := built.ask(t, "prove the sizes")

	if outcome.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting on the model's question after the refusal", outcome.Status)
	}
	requests := requestsJoined(built.model.Requests())
	if !strings.Contains(requests, "at 390 the page overflows by 42 pixels") || !strings.Contains(requests, "/pictures/three.png") {
		t.Errorf("the refusal does not name the width, the overflow and the picture; the requests read:\n%s", requests)
	}
	if built.held(t, outcome.TaskID).Goal.DoneWhen[0].Done {
		t.Errorf("the done line was marked although the page overflows")
	}
}

func TestALooksCheckFailsOnAPageErrorAndNamesIt(t *testing.T) {
	page, resize, shot, read := theLooksTools(t,
		"Tic Tac Toe\nthe page answered: 0\n", "Tic Tac Toe\nthe page answered: 0\npage errors:\n- TypeError: cells is not iterable at render.js:12\n", "Tic Tac Toe\nthe page answered: 0\n")
	built := aTaskWhoseDoneLineIsChecked(t, "The board fits every window. [looks: http://127.0.0.1:8097 at 1440, 768, 390]",
		[]testkit.Step{answerStep("There is an error. What should I do?")}, page, resize, shot, read)

	outcome := built.ask(t, "prove the sizes")

	if outcome.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting after the refusal", outcome.Status)
	}
	requests := requestsJoined(built.model.Requests())
	if !strings.Contains(requests, "at 768 the console holds: TypeError: cells is not iterable at render.js:12") {
		t.Errorf("the refusal does not name the width and the error; the requests read:\n%s", requests)
	}
}

func TestALooksCheckWithoutABrowserSaysSo(t *testing.T) {
	built := aTaskWhoseDoneLineIsChecked(t, "The board fits every window. [looks: http://127.0.0.1:8097 at 1440]",
		[]testkit.Step{answerStep("No browser. What should I do?")})

	outcome := built.ask(t, "prove the sizes")

	if outcome.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting after the refusal", outcome.Status)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "there is no browser tool on this machine") {
		t.Errorf("the refusal does not say the browser is missing")
	}
}
