package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// closingScript is the shape of every finished task: the model writes the done
// list, does the work, points each line at the result that proves it, and then
// reports.
func closingScript(doneLine string) []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["`+doneLine+`"]}`)),
		answerStep("It is done. What changed: nothing. What I checked: the notes. What is left: nothing."),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"doneWhen":[{"text":"`+doneLine+`","done":true,"resultId":"r1"}]}`)),
		answerStep("It is done. What changed: nothing. What I checked: the notes. What is left: nothing."),
	}
}

// TestADoneLineWithNothingBehindItSendsTheModelBack proves the done-check: a
// task cannot close while any line of its done list has no proof.
func TestADoneLineWithNothingBehindItSendsTheModelBack(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done once the line pointed at a result", outcome.Status)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "cannot close yet") {
		t.Error("the model was never sent back to work over a done line with nothing behind it")
	}
	held := built.held(t, outcome.TaskID)
	if held.Header.Status != contract.StatusDone {
		t.Errorf("the record says %q, want done", held.Header.Status)
	}
	if line := held.Goal.DoneWhen[0]; !line.Done || line.ResultID != contract.ResultID(1) {
		t.Errorf("the done line reads %+v, want it ticked and pointing at r1", line)
	}
}

// TestADoneLineThatNamesAFilePassesWhenTheFileIsThere proves the mechanical half
// of the done-check for a file.
func TestADoneLineThatNamesAFilePassesWhenTheFileIsThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(path, []byte("the draft"), 0o600); err != nil {
		t.Fatalf("cannot write the file the done line names: %v", err)
	}
	built := newHarness(t, closingScript("the draft is written to "+path), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "write the draft")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done, because the file the line names is there", outcome.Status)
	}
}

// TestADoneLineThatNamesAFolderWithSpacesInItsNameIsReadWhole: the ninth fresh
// run's scaffold task ended failed on "package.json exists at
// /home/user/Desktop/Tater Tots Tetrisv1 with a test script", because the
// check split the line on spaces and looked for /home/user/Desktop/Tater. A
// path that is not there is tried with the words after it, one at a time,
// before it is called missing.
func TestADoneLineThatNamesAFolderWithSpacesInItsNameIsReadWhole(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "Tater Tots Tetrisv1")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatalf("cannot make the folder the done line names: %v", err)
	}
	built := newHarness(t, closingScript("package.json exists at "+folder+" with a test script"), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "scaffold the project")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done, because the folder the line names is there under its whole name", outcome.Status)
	}
}

// TestADoneLineThatNamesAMissingFileSendsTheModelBack proves the same check
// refuses a line whose file was never written.
func TestADoneLineThatNamesAMissingFileSendsTheModelBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-written.md")
	built := newHarness(t, closingScript("the draft is written to "+path), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "write the draft")

	if outcome.Status == contract.StatusDone {
		t.Error("the task closed with a done line naming a file that is not there")
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "it is not there") {
		t.Error("the model was never told which file was missing")
	}
}

// TestAPinnedDoneLineTheHarnessCannotConfirmIsTakenOnTheModelsProof: on the
// ninth fresh run the done check misread a folder's name, refused a done line
// the model had proved with a result, and failed the task on its own mistake.
// A mechanical check of the harness may send the model back twice for a line
// that names a result; after that the line stands on the model's proof, the
// task closes, and the record says which line the harness could not confirm.
func TestAPinnedDoneLineTheHarnessCannotConfirmIsTakenOnTheModelsProof(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-written.md")
	steps := closingScript("the draft is written to " + path)
	for range 3 {
		steps = append(steps, answerStep("It is done. What changed: nothing. What I checked: the draft. What is left: nothing."))
	}
	built := newHarness(t, steps, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "write the draft")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done on the model's proof after two send-backs", outcome.Status)
	}
	// Each send-back is a message of its own in the conversation, so the
	// request that carries the most of them carries them all.
	sentBack := 0
	for _, request := range built.model.Requests() {
		inThisOne := 0
		for _, message := range request.Messages {
			if strings.Contains(message.Text, "it is not there") {
				inThisOne++
			}
		}
		sentBack = max(sentBack, inThisOne)
	}
	if sentBack != 2 {
		t.Errorf("the model was sent back %d times for the same line, want exactly two", sentBack)
	}
	held := built.held(t, outcome.TaskID)
	noted := false
	for _, decision := range held.Lessons.Decisions {
		if strings.Contains(decision.Text, "could not confirm") && strings.Contains(decision.Reason, "r1") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("the record does not say the harness could not confirm the line and took it on r1: %+v", held.Lessons.Decisions)
	}
}

// TestADoneLineThatNamesACommandRunsItAndChecksItExitsZero proves the other
// mechanical check: a command in backticks has to succeed.
func TestADoneLineThatNamesACommandRunsItAndChecksItExitsZero(t *testing.T) {
	built := newHarness(t, closingScript("the tests pass: `go test ./...`"), scriptedTool("read", "the notes"))
	built.sandbox.Script("sh -c go test ./...", contract.SandboxResult{ExitCode: 0})

	outcome := built.ask(t, "make the tests pass")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done, because the command the line names exits zero", outcome.Status)
	}
	commands := built.sandbox.Commands()
	if len(commands) == 0 || commands[0].Program != "sh" {
		t.Errorf("the sandbox was asked to run %v, want the command the done line named", commands)
	}
}

// TestADoneLineWhoseCommandFailsSendsTheModelBack proves a command that does not
// exit zero is a line that is not true yet.
func TestADoneLineWhoseCommandFailsSendsTheModelBack(t *testing.T) {
	built := newHarness(t, closingScript("the tests pass: `go test ./...`"), scriptedTool("read", "the notes"))
	built.sandbox.Script("sh -c go test ./...", contract.SandboxResult{ExitCode: 1})

	outcome := built.ask(t, "make the tests pass")

	if outcome.Status == contract.StatusDone {
		t.Error("the task closed with a done line whose command exited one")
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "it exited 1") {
		t.Error("the model was never told the command failed")
	}
}

// TestATaskThatWillNotProveItsDoneListIsGivenUpOn proves the nudging is bounded,
// because every loop in Nerd Genie is bounded.
func TestATaskThatWillNotProveItsDoneListIsGivenUpOn(t *testing.T) {
	steps := []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":["the notes are read"]}`)),
	}
	for range 6 {
		steps = append(steps, answerStep("It is done, honestly."))
	}
	built := newHarness(t, steps, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusFailed {
		t.Errorf("the task ended %q, want failed, because it would not prove its done list", outcome.Status)
	}
}

// theGreenJestRun is a test run the shell answers with, read as three passing.
const theGreenJestRun = "finished with exit code 0\nTests: 3 passed, 3 total"

// TestADoneLineRestingOnATestRunFromBeforeTheLastChangeIsSentBack: a done
// line may point at a green test run and then the model changes a file, so the
// proof is older than the change and says nothing about the file as it is now.
// The harness sends the model back once to run the tests again and point the
// line at the new result, and the task closes on that.
func TestADoneLineRestingOnATestRunFromBeforeTheLastChangeIsSentBack(t *testing.T) {
	// The shell answers the model's run, the harness's run after the write,
	// and the model's run again.
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theGreenJestRun, theGreenJestRun, theGreenJestRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.",
			callFor("c1", contract.ToolShell, `{"command":"npx jest"}`),
			taskCall("c1t", `{"why":"the user wants the notes kept and proved","doneWhen":[{"text":"the tests pass","done":true,"resultId":"r1"}]}`)),
		callStep("I will write the notes.", callFor("c2", contract.ToolWrite, `{"path":"/notes/today.md","content":"kept"}`)),
		answerStep("It is done. What changed: the notes. What I checked: the tests. What is left: nothing."),
		callStep("I will run the tests again.", callFor("c3", contract.ToolShell, `{"command":"npx jest"}`)),
		callStep("I will point the line at the new run.",
			taskCall("c3t", `{"doneWhen":[{"text":"the tests pass","done":true,"resultId":"r4"}]}`)),
		answerStep("It is done. What changed: the notes. What I checked: the tests again. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /notes/today.md, 4 bytes"))

	outcome := built.ask(t, "keep the notes and prove it")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done once the line pointed at a run from after the change", outcome.Status)
	}
	sentBack := `The done line "the tests pass" rests on r1, a test run from before your last change; run the tests again and point the line at the new result.`
	if !strings.Contains(requestsJoined(built.model.Requests()), sentBack) {
		t.Errorf("the model was never sent back over a test run older than its last change, and the requests read:\n%s", requestsJoined(built.model.Requests()))
	}
	if line := built.held(t, outcome.TaskID).Goal.DoneWhen[0]; line.ResultID != "r4" {
		t.Errorf("the done line points at %s, want r4, the run after the change", line.ResultID)
	}
}

// TestADoneLineRestingOnATestRunAfterTheLastChangePasses keeps the check to
// what it is for: a test run that came after the last write proves the file as
// it is, and the model is not sent back.
func TestADoneLineRestingOnATestRunAfterTheLastChangePasses(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell, Description: "A shell the test scripted."}, theGreenJestRun)
	built := newHarness(t, []testkit.Step{
		callStep("I will write the notes.",
			callFor("c1", contract.ToolWrite, `{"path":"/notes/today.md","content":"kept"}`),
			taskCall("c1t", `{"why":"the user wants the notes kept and proved","doneWhen":["the tests pass"]}`)),
		callStep("I will run the tests.", callFor("c2", contract.ToolShell, `{"command":"npx jest"}`)),
		callStep("I will point the line at the run.",
			taskCall("c2t", `{"doneWhen":[{"text":"the tests pass","done":true,"resultId":"r3"}]}`)),
		answerStep("It is done. What changed: the notes. What I checked: the tests. What is left: nothing."),
	}, shell, scriptedTool(contract.ToolWrite, "created /notes/today.md, 4 bytes"))

	outcome := built.ask(t, "keep the notes and prove it")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done, because the test run came after the write", outcome.Status)
	}
	if strings.Contains(requestsJoined(built.model.Requests()), "from before your last change") {
		t.Error("the model was sent back over a test run that came after its last change")
	}
}
