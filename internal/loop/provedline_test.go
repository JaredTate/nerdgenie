package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// Run 23's visual QA task had its done lines proved and its suite green and
// went on editing the stylesheet for half an hour. The tests-first line
// ignores a stylesheet on purpose, so a different line is owed once every
// done line is proved: an edit then serves no line, and the model is told to
// finish or to name the line it serves.

// editWithDoneLines is a task whose model writes the done lines given,
// proves the first `proved` of them by reads that carry the marks, and then
// edits theme.css. It hands back the text of the request that carries the
// edit's result.
func editWithDoneLines(t *testing.T, lines []string, proved int) string {
	t.Helper()
	doneWhen := ""
	if len(lines) > 0 {
		doneWhen = `"` + strings.Join(lines, `","`) + `"`
	}
	steps := []testkit.Step{
		callStep("I will plan.", taskCall("p1", `{"plan":["read the notes","polish the theme"],"doneWhen":[`+doneWhen+`]}`)),
	}
	paths := []string{"notes.md", "brand.md"}
	for at, path := range paths {
		arguments := `{"path":"` + path + `"}`
		if at < proved {
			arguments = `{"path":"` + path + `","proves":` + string(rune('1'+at)) + `}`
		}
		steps = append(steps, callStep("Reading.", callFor("c"+string(rune('1'+at)), contract.ToolRead, arguments)))
	}
	steps = append(steps,
		callStep("Polishing.", callFor("e1", contract.ToolEdit, `{"path":"/game/css/theme.css","old":"a","new":"b"}`)),
		answerStep("Done. What changed: the theme. What I checked: nothing. What is left: nothing."))
	built := newHarness(t, steps, scriptedTool(contract.ToolRead, "the notes", "the brand"), scriptedTool(contract.ToolEdit, "edited /game/css/theme.css"))
	built.ask(t, "polish the theme")
	requests := built.model.Requests()
	return wholeRequestText(requests[len(requests)-1])
}

func TestAnEditAfterEveryDoneLineIsProvedGetsTheProvedLine(t *testing.T) {
	request := editWithDoneLines(t, []string{"the notes are read", "the brand is read"}, 2)
	if !strings.Contains(request, loop.TheEveryLineProvedLine) {
		t.Errorf("the edit after every done line was proved does not carry the line; the request reads:\n%s", request)
	}
}

func TestAnEditWithADoneLineStillOpenGetsNoProvedLine(t *testing.T) {
	request := editWithDoneLines(t, []string{"the notes are read", "the brand is read"}, 1)
	if strings.Contains(request, loop.TheEveryLineProvedLine) {
		t.Errorf("the edit with a done line open carries the line; the request reads:\n%s", request)
	}
}

func TestAnEditBeforeAnyDoneLineExistsGetsNoProvedLine(t *testing.T) {
	request := editWithDoneLines(t, nil, 0)
	if strings.Contains(request, loop.TheEveryLineProvedLine) {
		t.Errorf("the edit before any done line carries the line; the request reads:\n%s", request)
	}
}
