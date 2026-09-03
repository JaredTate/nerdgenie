//go:build integration

package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheLoopRunsAgainstTheRealEventLog runs the same task the unit tests run,
// with the real SQLite log in a temporary home in place of the fake store, so
// that the record, the checkpoints, and the results are proved against the file
// they will actually live in.
func TestTheLoopRunsAgainstTheRealEventLog(t *testing.T) {
	home := testkit.NewTempHome(t)
	eventLog, err := log.Open(t.Context(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log in the temporary home: %v", err)
	}
	t.Cleanup(func() {
		if err := eventLog.Close(); err != nil {
			t.Errorf("cannot close the event log: %v", err)
		}
	})

	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the whole of the notes"))
	options := built.options()
	options.Store = eventLog
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the real event log: %v", err)
	}

	outcome, err := made.Run(t.Context(), built.task("read the notes"))
	if err != nil {
		t.Fatalf("the loop could not run the task against the real log: %v", err)
	}
	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}

	keeper, err := record.Load(t.Context(), eventLog, contract.RecordTask, outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot load task %s back out of the real log: %v", outcome.TaskID, err)
	}
	held := keeper.Record()
	if held.Goal.Ask != "read the notes" {
		t.Errorf("the ask reads %q, and the user wrote %q", held.Goal.Ask, "read the notes")
	}
	text, err := keeper.Read(t.Context(), contract.ResultID(1))
	if err != nil {
		t.Fatalf("cannot read r1 back out of the real log: %v", err)
	}
	if text != "the whole of the notes" {
		t.Errorf("r1 reads back as %q, and the tool returned %q", text, "the whole of the notes")
	}
}

// TestTheCommandsReadTheRealLog proves the two commands the orchestrator
// registers work against the file rather than only against the fake.
func TestTheCommandsReadTheRealLog(t *testing.T) {
	home := testkit.NewTempHome(t)
	eventLog, err := log.Open(t.Context(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log in the temporary home: %v", err)
	}
	t.Cleanup(func() {
		if err := eventLog.Close(); err != nil {
			t.Errorf("cannot close the event log: %v", err)
		}
	})

	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	options := built.options()
	options.Store = eventLog
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the real event log: %v", err)
	}
	outcome, err := made.Run(t.Context(), built.task("read the notes"))
	if err != nil {
		t.Fatalf("the loop could not run the task against the real log: %v", err)
	}

	listed, err := made.TasksCommand().Run(t.Context(), "", contract.CommandContext{Channel: built.channel})
	if err != nil {
		t.Fatalf("the tasks command failed against the real log: %v", err)
	}
	if !strings.Contains(listed, "task "+outcome.TaskID) {
		t.Errorf("the listing reads %q, want the task the loop just ran", listed)
	}
	printed, err := made.TasksCommand().Run(t.Context(), outcome.TaskID, contract.CommandContext{Channel: built.channel})
	if err != nil {
		t.Fatalf("the tasks command could not print the record: %v", err)
	}
	if !strings.Contains(printed, "read the notes") {
		t.Errorf("the printed record reads %q, want the user's own ask in it", printed)
	}
}
