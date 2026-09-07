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

// aTaskWhoseDoneLineIsChecked scripts the smallest task with one done line
// the harness checks itself: the model writes the line and says it is done,
// and the steps given follow, for a model that has to go on after a refusal.
func aTaskWhoseDoneLineIsChecked(t *testing.T, line string, after []testkit.Step, tools ...contract.Tool) *harness {
	t.Helper()
	steps := []testkit.Step{
		callStep("I will write the finish.", taskCall("c1t", `{"why":"the user wants it proved","doneWhen":["`+line+`"]}`)),
		answerStep("It is done. What changed: the work. What I checked: the harness's check. What is left: nothing."),
	}
	return newHarness(t, append(steps, after...), tools...)
}

// TestABracketedDoneLineIsRunByTheHarness: each of the four kinds of check is
// run by the harness when the model says it is done, and a check that passes
// is written into the record as a result the line then points at, so the task
// closes on the harness's proof and not on the model's word.
func TestABracketedDoneLineIsRunByTheHarness(t *testing.T) {
	t.Run("tests pass", func(t *testing.T) {
		built := aTaskWhoseDoneLineIsChecked(t, "Every test passes. [tests pass: npm test]", nil)
		built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theGreenRun)})

		outcome := built.ask(t, "prove the tests")

		theLineIsProvedByACheck(t, built, outcome, "check: tests pass: npm test: all 10 passing")
	})
	t.Run("exit 0", func(t *testing.T) {
		built := aTaskWhoseDoneLineIsChecked(t, "The build is clean. [exit 0: node build.js]", nil)
		built.sandbox.Script("sh -c node build.js", contract.SandboxResult{StandardOutput: []byte("built")})

		outcome := built.ask(t, "prove the build")

		theLineIsProvedByACheck(t, built, outcome, "check: exit 0: node build.js")
	})
	t.Run("shows", func(t *testing.T) {
		page := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen, Description: "A browser the test scripted."}, "http://127.0.0.1:8091/ Tater Tots Tetris\ne1 canvas the board")
		built := aTaskWhoseDoneLineIsChecked(t, `The game loads. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`, nil, page)

		outcome := built.ask(t, "prove the page")

		theLineIsProvedByACheck(t, built, outcome, `check: shows: "Tater Tots Tetris" at http://127.0.0.1:8091`)
		if inputs := page.Inputs(); len(inputs) != 1 || !strings.Contains(string(inputs[0]), "http://127.0.0.1:8091") {
			t.Errorf("the browser was opened %d times with %s, want once on the line's address", len(inputs), inputs)
		}
	})
	t.Run("exists", func(t *testing.T) {
		built := aTaskWhoseDoneLineIsChecked(t, "The bundle is written. [exists: dist/index.html]", nil)
		if err := os.MkdirAll(filepath.Join(built.workFolder, "dist"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(built.workFolder, "dist", "index.html"), []byte("<html>"), 0o644); err != nil {
			t.Fatal(err)
		}

		outcome := built.ask(t, "prove the bundle")

		theLineIsProvedByACheck(t, built, outcome, "check: exists: dist/index.html")
	})
}

// theLineIsProvedByACheck holds that the task closed done and its one done
// line points at a result the harness wrote for the check.
func theLineIsProvedByACheck(t *testing.T, built *harness, outcome loop.Outcome, summary string) {
	t.Helper()
	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q with %q, want done on the harness's own check", outcome.Status, outcome.Report)
	}
	held := built.held(t, outcome.TaskID)
	line := held.Goal.DoneWhen[0]
	if !line.Done || line.ResultID == "" {
		t.Fatalf("the done line reads %+v, want it marked with the check's result", line)
	}
	for _, result := range held.Work.Results {
		if result.ID == line.ResultID {
			if !strings.Contains(result.Summary, summary) {
				t.Errorf("the check's result reads %q, want it to say %q", result.Summary, summary)
			}
			return
		}
	}
	t.Errorf("the done line points at %s, and the record holds no such result", line.ResultID)
}

// TestTheFinishIsRefusedWhileACheckFails: the person wrote the check, so a red
// check sends the model back with the line named and the output's tail, and
// nothing the model says closes the task while it stays red.
func TestTheFinishIsRefusedWhileACheckFails(t *testing.T) {
	built := aTaskWhoseDoneLineIsChecked(t, "Every test passes. [tests pass: npm test]", []testkit.Step{
		answerStep("The tests still fail on the row clear. What should I do?"),
	})
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theRedRun), ExitCode: 1})

	outcome := built.ask(t, "prove the tests")

	if outcome.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting on the model's question after the refusal", outcome.Status)
	}
	requests := requestsJoined(built.model.Requests())
	refusal := `The done line "Every test passes. [tests pass: npm test]" has its check, and 2 failing of 10: clears a row, spawns, so this line is not true yet.`
	if !strings.Contains(requests, refusal) {
		t.Errorf("the model was not sent back with the line and the count, and the requests read:\n%s", requests)
	}
	if !strings.Contains(requests, "✖ clears a row") {
		t.Errorf("the refusal does not carry the output's tail, and the requests read:\n%s", requests)
	}
	if line := built.held(t, outcome.TaskID).Goal.DoneWhen[0]; line.Done {
		t.Errorf("the done line reads %+v after a red check, want it unmarked", line)
	}
}

// TestAFailingCheckBecomesAFailureAfterThreeTries: a check that fails three
// times in one task goes into the record as a failure with its cause, so the
// model goes on by another route with the lesson in front of it, and the
// finish stays refused; the refusals never count as the model's own done-check
// nudges, so the task is not failed for them.
func TestAFailingCheckBecomesAFailureAfterThreeTries(t *testing.T) {
	built := aTaskWhoseDoneLineIsChecked(t, "Every test passes. [tests pass: npm test]", []testkit.Step{
		answerStep("It is done now. What changed: nothing. What I checked: the tests. What is left: nothing."),
		answerStep("It is done, really. What changed: nothing. What I checked: the tests. What is left: nothing."),
		answerStep("It is done, I promise. What changed: nothing. What I checked: the tests. What is left: nothing."),
		answerStep("The row clear will not pass. What should I do?"),
	})
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theRedRun), ExitCode: 1})

	outcome := built.ask(t, "prove the tests")

	if outcome.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting: four refused finishes are not a failed task", outcome.Status)
	}
	held := built.held(t, outcome.TaskID)
	if len(held.Lessons.Failures) != 1 || !strings.Contains(held.Lessons.Failures[0].Text, "tests pass: npm test") {
		t.Fatalf("the record's failures read %+v, want one for the check that failed three times", held.Lessons.Failures)
	}
	if !strings.Contains(held.Lessons.Failures[0].Text, "2 failing of 10") {
		t.Errorf("the failure %q does not carry what the check showed", held.Lessons.Failures[0].Text)
	}
	if held.Goal.DoneWhen[0].Done {
		t.Errorf("the done line is marked after four red checks")
	}
}
