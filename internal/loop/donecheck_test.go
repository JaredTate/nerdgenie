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
// /home/jared/Desktop/Tater Tots Tetrisv1 with a test script", because the
// check split the line on spaces and looked for /home/jared/Desktop/Tater. A
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
