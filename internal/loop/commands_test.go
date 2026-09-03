package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// runTasksCommand runs the tasks command with the arguments given.
func runTasksCommand(t *testing.T, built *harness, arguments string) (string, error) {
	t.Helper()
	return built.loop.TasksCommand().Run(t.Context(), arguments, contract.CommandContext{Channel: built.channel})
}

// TestTheTasksCommandListsWhatThereIs proves "/tasks" shows what is running,
// waiting, and done.
func TestTheTasksCommandListsWhatThereIs(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	if command := built.loop.TasksCommand(); command.Name != "tasks" {
		t.Fatalf("the command is called %q, and the orchestrator registers it as \"tasks\"", command.Name)
	}

	empty, err := runTasksCommand(t, built, "")
	if err != nil {
		t.Fatalf("the tasks command failed on an empty log: %v", err)
	}
	if !strings.Contains(empty, "no tasks yet") {
		t.Errorf("the listing reads %q with no tasks at all, and it should say so", empty)
	}

	outcome := built.ask(t, "read the notes")
	listed, err := runTasksCommand(t, built, "")
	if err != nil {
		t.Fatalf("the tasks command failed: %v", err)
	}
	if !strings.Contains(listed, "task "+outcome.TaskID) || !strings.Contains(listed, "done") {
		t.Errorf("the listing reads %q, want the task and where it stands", listed)
	}
	if !strings.Contains(listed, "read the notes") {
		t.Errorf("the listing reads %q, want the user's own ask on the line", listed)
	}
}

// TestTheTasksCommandPrintsOneRecord proves "/tasks 17" shows the record the
// model reads.
func TestTheTasksCommandPrintsOneRecord(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	outcome := built.ask(t, "read the notes")

	printed, err := runTasksCommand(t, built, outcome.TaskID)
	if err != nil {
		t.Fatalf("the tasks command could not print task %s: %v", outcome.TaskID, err)
	}
	for _, wanted := range []string{"# task " + outcome.TaskID, "## Goal", "read the notes", "## Work"} {
		if !strings.Contains(printed, wanted) {
			t.Errorf("the printed record does not carry %q", wanted)
		}
	}
}

// TestTheTasksCommandWindsARecordBack proves "/tasks 17 back 1" reloads an
// earlier checkpoint so the model can try another path. A task saves one
// checkpoint per round now, so a step back is a round of work: this script runs
// three rounds, and one step back is the moment before its last.
func TestTheTasksCommandWindsARecordBack(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	outcome := built.ask(t, "read the notes")

	wound, err := runTasksCommand(t, built, outcome.TaskID+" back 1")
	if err != nil {
		t.Fatalf("the tasks command could not wind task %s back: %v", outcome.TaskID, err)
	}
	if !strings.Contains(wound, "is back at checkpoint") {
		t.Errorf("winding back said %q, want which checkpoint the record is at now", wound)
	}
	if strings.Contains(wound, "# task "+outcome.TaskID+"   "+string(contract.StatusDone)) {
		t.Error("the record wound back three checkpoints still says it is done")
	}
	if !strings.Contains(wound, "# task "+outcome.TaskID+"   "+string(contract.StatusRunning)) {
		t.Errorf("the wound-back record reads %q, want it back at a moment when the task was still running", wound)
	}
}

// TestTheTasksCommandRefusesWhatItCannotDo proves every bad way of asking gets
// an answer saying what to write instead.
func TestTheTasksCommandRefusesWhatItCannotDo(t *testing.T) {
	built := newHarness(t, nil)
	for _, arguments := range []string{"17 sideways 3", "17 back three", "404", "404 back 1"} {
		if _, err := runTasksCommand(t, built, arguments); err == nil {
			t.Errorf("the tasks command took %q, and it cannot do that", arguments)
		}
	}
}

// TestTheLoopSaysWhichTaskIsRunning proves what the stop command reads to name
// the task it is stopping.
func TestTheLoopSaysWhichTaskIsRunning(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))

	if built.loop.Running() != "" {
		t.Error("the loop says a task is running before anything has been asked of it")
	}
	built.ask(t, "read the notes")
	if built.loop.Running() != "" {
		t.Errorf("the loop still says task %q is running after it finished", built.loop.Running())
	}
}
