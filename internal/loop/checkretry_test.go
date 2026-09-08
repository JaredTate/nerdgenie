package loop_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// onceInterruptedTool fails its first call the way the browser tool does when
// the worker has died and will be started again on the next call, then
// answers like a scripted tool. Run 25's shows and looks checks both failed
// at the end of a task that never used the browser, because the idle worker
// had died and the check's one open hit the restart.
type onceInterruptedTool struct {
	name   string
	answer string
	calls  int
}

func (tool *onceInterruptedTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: tool.name, Description: "A browser the test interrupted once.", Classes: []contract.PermissionClass{contract.ClassRead}}
}

func (tool *onceInterruptedTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	tool.calls++
	if tool.calls == 1 {
		return contract.ToolOutput{}, errors.New("cannot open the page http://127.0.0.1:8097: the browser was interrupted during open and will be started again on the next call: Chrome stopped working")
	}
	return contract.ToolOutput{Text: tool.answer}, nil
}

func TestAShowsCheckTriesOnceMoreWhenTheBrowserIsBeingStartedAgain(t *testing.T) {
	page := &onceInterruptedTool{name: contract.ToolBrowserOpen, answer: "http://127.0.0.1:8097/ Tic Tac Toe\ne1 heading Tic Tac Toe"}
	built := aTaskWhoseDoneLineIsChecked(t, `The game loads. [shows: "Tic Tac Toe" at http://127.0.0.1:8097]`, nil, page)

	outcome := built.ask(t, "prove the page")

	theLineIsProvedByACheck(t, built, outcome, `check: shows: "Tic Tac Toe"`)
	if page.calls != 2 {
		t.Errorf("the browser was opened %d times, want 2: once into the restart, once more after it", page.calls)
	}
}

func TestALooksCheckTriesOnceMoreWhenTheBrowserIsBeingStartedAgain(t *testing.T) {
	page := &onceInterruptedTool{name: contract.ToolBrowserOpen, answer: "http://127.0.0.1:8097/ Tic Tac Toe\ne1 heading Tic Tac Toe"}
	resize := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserResize, Description: "A resize the test scripted."}, "the page is 1440 wide")
	shot := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserScreenshot, Description: "A camera the test scripted."}, "the picture is saved at /pictures/one.png")
	read := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserRead, Description: "A read the test scripted."}, "Tic Tac Toe\nthe page answered: 0\n")
	built := aTaskWhoseDoneLineIsChecked(t, "The board fits. [looks: http://127.0.0.1:8097 at 1440]", nil, page, resize, shot, read)

	outcome := built.ask(t, "prove the sizes")

	theLineIsProvedByACheck(t, built, outcome, "check: looks: http://127.0.0.1:8097 at 1440")
	if page.calls != 2 {
		t.Errorf("the browser was opened %d times, want 2", page.calls)
	}
}

// TestACheckDoesNotRetryAnyOtherFailure: a page that refuses the connection is
// refused once, with the reason, and nothing is tried twice.
func TestACheckDoesNotRetryAnyOtherFailure(t *testing.T) {
	page := &failingTool{name: contract.ToolBrowserOpen, reason: errors.New("cannot open the page http://127.0.0.1:8097: connection refused")}
	built := aTaskWhoseDoneLineIsChecked(t, `The game loads. [shows: "Tic Tac Toe" at http://127.0.0.1:8097]`, nil, page)

	built.ask(t, "prove the page")

	if page.calls != 1 {
		t.Errorf("the browser was opened %d times, want 1: a refused connection is not a restart", page.calls)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "connection refused") {
		t.Error("the refusal does not carry the reason")
	}
}
