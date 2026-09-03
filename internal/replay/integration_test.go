//go:build integration

package replay_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/replay"
	"github.com/JaredTate/coeus/internal/testkit"
)

// openARealLog opens one SQLite event log in a temporary folder and closes it
// when the test is over.
func openARealLog(t *testing.T, name string) *log.Log {
	t.Helper()
	opened, err := log.Open(context.Background(), filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("cannot open the event log %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("cannot close the event log %s: %v", name, err)
		}
	})
	return opened
}

// TestAReplayReadsAndWritesTheRealEventLog runs a task into a real SQLite log
// in a temporary home and replays it out of that same log into another, which
// is the whole path the replay subcommand takes.
func TestAReplayReadsAndWritesTheRealEventLog(t *testing.T) {
	ctx := context.Background()
	testkit.NewTempHome(t)
	recording := openARealLog(t, "recorded.db")
	scripted := twoRoundScript()

	made, err := loop.New(loop.Options{
		Model:      testkit.NewFakeModel(testkit.Script{Name: "test", ContextLength: 24000, Steps: scripted.steps}),
		Tools:      testkit.NewFakeToolRegistry(scripted.tools...),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Store:      recording,
		Clock:      testkit.NewFakeClock(theStartOfTime),
		Context:    theWorkingContext(t),
	})
	if err != nil {
		t.Fatalf("cannot build the loop that writes the real log: %v", err)
	}
	outcome, err := made.Run(ctx, loop.Task{
		Message: contract.Inbound{ID: "in-1", Text: scripted.ask, Channel: "terminal"},
		Channel: testkit.NewFakeChannel("terminal"),
	})
	if err != nil {
		t.Fatalf("the recorded run did not finish: %v", err)
	}

	options := theReplayOptions(t, recording)
	options.Into = openARealLog(t, "replayed.db")
	result, err := replay.Run(ctx, options, outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot replay task %s out of the real log: %v", outcome.TaskID, err)
	}
	if !result.Passed {
		t.Fatalf("the replay out of the real log did not reproduce it: %s", result.Report)
	}
}

// TestAGeneratedTestIsWrittenToDisk writes the two files "--as-test" makes into
// a folder of its own, which is what the subcommand does with them.
func TestAGeneratedTestIsWrittenToDisk(t *testing.T) {
	generated := generateFromACleanRun(t)
	root := t.TempDir()

	if err := generated.WriteInto(root); err != nil {
		t.Fatalf("cannot write the generated test: %v", err)
	}
	for _, one := range []string{generated.TestPath, generated.FixturePath} {
		held, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(one)))
		if err != nil {
			t.Fatalf("cannot read back the generated %s: %v", one, err)
		}
		if len(held) == 0 {
			t.Errorf("the generated %s was written empty", one)
		}
	}
}
