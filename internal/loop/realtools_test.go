package loop_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool"
)

// TestTheLoopDrivesTheRealToolRegistry proves the loop against the eighteen
// tools rather than against a fake: the model is told about all of them, a real
// read runs and its whole text reaches the log, and the real task tool writes
// the record, because the registry is built for the task with its own keeper.
func TestTheLoopDrivesTheRealToolRegistry(t *testing.T) {
	work := t.TempDir()
	notes := filepath.Join(work, "notes.md")
	if err := os.WriteFile(notes, []byte("the notes name two accounts"), 0o600); err != nil {
		t.Fatalf("cannot write the file the task reads: %v", err)
	}
	built := newHarness(t, scriptReading(notes))
	options := built.options()
	options.ToolsForTask = theRealTools(t, work, built)
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the real tool registry: %v", err)
	}

	outcome, err := made.Run(t.Context(), built.task("read the notes"))
	if err != nil {
		t.Fatalf("the loop could not run the task through the real tools: %v", err)
	}
	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s\nrecord: %+v", outcome.Status, outcome.Report,
			built.held(t, "1"))
	}
	checkTheRealToolsWereUsed(t, built, outcome.TaskID)
}

// TestToolsThatCannotBeBuiltStopTheTask proves the loop says so rather than
// running a task with no tools at all.
func TestToolsThatCannotBeBuiltStopTheTask(t *testing.T) {
	built := newHarness(t, nil)
	options := built.options()
	options.ToolsForTask = func(string, loop.TaskRecord) (contract.ToolRegistry, error) {
		return nil, errors.New("the tools folder cannot be read, so check the folder and its modes")
	}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop: %v", err)
	}

	if _, err := made.Run(t.Context(), built.task("read the notes")); err == nil {
		t.Error("the loop ran a task whose tools could not be built")
	}
}

// checkTheRealToolsWereUsed reads the record the real tools left behind.
func checkTheRealToolsWereUsed(t *testing.T, built *harness, taskID string) {
	t.Helper()
	held := built.held(t, taskID)
	if len(held.Goal.DoneWhen) != 1 || !held.Goal.DoneWhen[0].Done {
		t.Fatalf("the done list reads %v, and the real task tool should have written it", held.Goal.DoneWhen)
	}
	keeper, err := record.Load(t.Context(), built.store, contract.RecordTask, taskID)
	if err != nil {
		t.Fatalf("cannot load the record of task %s: %v", taskID, err)
	}
	text, err := keeper.Read(t.Context(), contract.ResultID(1))
	if err != nil {
		t.Fatalf("cannot read the first result back: %v", err)
	}
	if !strings.Contains(text, "the notes name two accounts") {
		t.Errorf("the first result reads back as %q, and the real read tool returned the file", text)
	}
	whole := requestsJoined(built.model.Requests())
	for _, name := range []string{contract.ToolRead, contract.ToolShell, contract.ToolTask, contract.ToolBrowserOpen} {
		if !strings.Contains(whole, name) {
			t.Errorf("the model was never told about the %s tool, and it sees all eighteen", name)
		}
	}
}

// scriptReading is one whole task written the way brief 2.5 fixed the task
// tool's own fields, with the real path of the file it reads in it.
func scriptReading(path string) []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", contract.ToolRead, `{"path":"`+path+`"}`),
			taskCall("c1t", `{"operation":"done_when","done_when":[{"text":"the notes are read"}]}`)),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"operation":"done_when","done_when":[{"text":"the notes are read","done":true,"result":"r1"}]}`)),
		answerStep("Read. What changed: nothing. What I checked: the notes. What is left: nothing."),
	}
}

// theRealTools builds the registry of eighteen tools for one task, the way
// serve.go will: the tools that need the record of the task running now are
// given the keeper the loop just made.
func theRealTools(t *testing.T, work string, built *harness) func(string, loop.TaskRecord) (contract.ToolRegistry, error) {
	t.Helper()
	home := testkit.NewTempHome(t)
	settings := contract.DefaultConfig()
	settings.SandboxRoots = []string{work}
	return func(taskID string, records loop.TaskRecord) (contract.ToolRegistry, error) {
		return tool.New(t.Context(), tool.Settings{
			Configuration: settings,
			Home:          home,
			UserHome:      t.TempDir(),
			TaskID:        taskID,
			Log:           built.store,
			Sandbox:       built.sandbox,
			Permission:    built.rulings,
			Memory:        built.memory,
			Skills:        built.skills,
			Jobs:          built.jobs,
			Clock:         built.clock,
			Records:       records,
			Results:       records,
		})
	}
}
