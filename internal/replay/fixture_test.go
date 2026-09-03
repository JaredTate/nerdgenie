package replay_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theBudgetTaskFixture is the recorded failing task kept in testdata, and
// theFixtureTaskID is the number it carries, which is the first number a fresh
// log hands out.
const (
	theBudgetTaskFixture = "budget-task.json"
	theFixtureTaskID     = "1"
)

// theRoundsTheFixtureWasRecordedWith is the budget the recorded task ran out
// of, and it is the flag the replay test flips: the recording is a task that
// failed because it was given two rounds, and the fix is giving it more.
const theRoundsTheFixtureWasRecordedWith = 2

// tooSmallABudget is the limits of a machine whose round budget is the one the
// fixture task ran out of.
func tooSmallABudget() contract.Caps {
	limits := contract.DefaultConfig().Caps
	limits.RoundsPerTask = theRoundsTheFixtureWasRecordedWith
	return limits
}

// budgetTaskScript is the task the fixture log records: the model reads the
// notes folder twice and writes a done list the second result proves, and the
// budget runs out before it can say so. The task ends stopped with its done
// list already true, which is a task that failed for a reason somebody can fix.
func budgetTaskScript() scriptedTask {
	return scriptedTask{
		ask:   "count the files in the notes folder",
		caps:  tooSmallABudget(),
		tools: []contract.Tool{scriptedTool("search", "three files", "notes.md, list.md, plan.md")},
		steps: []testkit.Step{
			{
				Text: "I am starting on the notes folder.",
				ToolCalls: []contract.ToolCall{
					callTo("c1", "search", `{"pattern":"*"}`),
					callTo("c2", "task", `{"why":"the user wants a count","plan":["read the folder","name the files"],`+
						`"done_when":["the files are counted"]}`),
				},
			},
			{
				Text: "I have the count and now want the names.",
				ToolCalls: []contract.ToolCall{
					callTo("c3", "search", `{"pattern":"*.md"}`),
					callTo("c4", "task", `{"done_when":[{"text":"the files are counted","done":true,"resultId":"r3"}]}`),
				},
			},
			{Text: "I read the folder twice and the budget ran out before I could report."},
			{Text: "Keep the plan and give this task more rounds next time."},
		},
	}
}

// TestTheFixtureLogHoldsTheRecordedFailingTask keeps the fixture in testdata
// honest: it is the log the scripted failing task really writes, and the day
// the loop writes a different one this test says so instead of letting the
// replay quietly test something else.
func TestTheFixtureLogHoldsTheRecordedFailingTask(t *testing.T) {
	made := runScript(t, budgetTaskScript())

	if made.outcome.Status != contract.StatusStopped {
		t.Fatalf("the fixture task ended %q and the fixture is of a task whose budget ran out", made.outcome.Status)
	}
	if made.outcome.TaskID != theFixtureTaskID {
		t.Fatalf("the fixture task is numbered %q and the fixture names %q", made.outcome.TaskID, theFixtureTaskID)
	}
	testkit.Golden(t, theBudgetTaskFixture, dumpEventLog(t, made.store))
}

// dumpEventLog writes every event of a log out in the shape the fixture file
// holds, which is a list the loader can put straight back.
func dumpEventLog(t *testing.T, store *testkit.FakeStore) []byte {
	t.Helper()
	events := []contract.Event{}
	if err := store.Replay(context.Background(), func(event contract.Event) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("cannot read the log the recorded run wrote: %v", err)
	}
	written, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		t.Fatalf("cannot write the recorded log as JSON: %v", err)
	}
	return append(written, '\n')
}

// loadFixtureLog reads one fixture log out of testdata into a fake store, which
// is the log a replay then reads its recording from.
func loadFixtureLog(t *testing.T, name string) *testkit.FakeStore {
	t.Helper()
	held, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("cannot read the fixture log %s: %v", name, err)
	}
	events := []contract.Event{}
	if err := json.Unmarshal(held, &events); err != nil {
		t.Fatalf("cannot read the fixture log %s as JSON: %v", name, err)
	}
	store := testkit.NewFakeStore()
	for _, event := range events {
		if _, err := store.Append(context.Background(), event); err != nil {
			t.Fatalf("cannot put the fixture log %s back into a store: %v", name, err)
		}
	}
	return store
}
