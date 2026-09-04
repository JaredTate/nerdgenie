//go:build integration

package log

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// crashHelperVariable names the environment variable that turns this test binary
// into the helper the crash test starts and kills. Re-invoking the test binary
// with a variable set is the standard Go way to get a second process that shares
// the code under test.
const crashHelperVariable = "COEUS_LOG_CRASH_HELPER_DATABASE"

// helperReadyLine is what the helper prints once it has written its first event,
// so the parent knows the file is in use before it kills anything.
const helperReadyLine = "the crash helper is writing"

// helperEventLimit is how many events the helper writes before giving up on its
// own. The parent kills it long before this, and the limit is here so that a
// parent that dies first cannot leave a process writing forever.
const helperEventLimit = 1_000_000

// helperWaitLimit is how long the parent gives the whole helper run.
const helperWaitLimit = 30 * time.Second

// writingHeadStart is how long the helper is left running after its first event,
// so that the kill lands in the middle of the writing rather than beside it.
const writingHeadStart = 150 * time.Millisecond

func TestMain(tests *testing.M) {
	if path := os.Getenv(crashHelperVariable); path != "" {
		os.Exit(writeUntilKilled(path))
	}
	os.Exit(tests.Run())
}

// writeUntilKilled is the helper process. It opens the log the parent named and
// appends events as fast as it can, printing one line after the first event so
// that the parent knows writing has started. It expects to be killed part way
// through a write.
func writeUntilKilled(path string) int {
	ctx := context.Background()
	eventLog, err := Open(ctx, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the crash helper cannot open %s: %v\n", path, err)
		return 1
	}

	for number := 1; number <= helperEventLimit; number++ {
		_, err := eventLog.Append(ctx, contract.Event{
			Occurred: aTime,
			TaskID:   "t1",
			Kind:     contract.EventToolResult,
			Body:     json.RawMessage(fmt.Sprintf(`{"step":%d}`, number)),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "the crash helper cannot append event %d: %v\n", number, err)
			return 1
		}
		if number == 1 {
			fmt.Println(helperReadyLine)
		}
	}
	return 0
}

func TestTheRealFileInATemporaryHomeSurvivesACloseAndReopen(t *testing.T) {
	home := testkit.NewTempHome(t)
	ctx := context.Background()

	first, err := Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the log at %s failed: %v", home.DatabaseFile(), err)
	}
	for step := 1; step <= 3; step++ {
		if _, err := first.Append(ctx, contract.Event{
			Occurred: aTime,
			TaskID:   "t1",
			Kind:     contract.EventToolResult,
			Body:     json.RawMessage(fmt.Sprintf(`{"step":%d}`, step)),
		}); err != nil {
			t.Fatalf("appending event %d failed: %v", step, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}

	second, err := Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("reopening the log failed: %v", err)
	}
	defer second.Close()

	found, err := second.ByTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reading the task's events back failed: %v", err)
	}
	if len(found) != 3 {
		t.Fatalf("the reopened log holds %d events, want 3", len(found))
	}
	for position, event := range found {
		wanted := fmt.Sprintf(`{"step":%d}`, position+1)
		if string(event.Body) != wanted {
			t.Errorf("event %d carries the body %s, want %s", event.Sequence, event.Body, wanted)
		}
		if !event.Occurred.Equal(aTime) {
			t.Errorf("event %d happened at %s, want %s", event.Sequence, event.Occurred, aTime)
		}
	}

	next, err := second.Append(ctx, contract.Event{Occurred: aTime, TaskID: "t1", Kind: contract.EventReply})
	if err != nil {
		t.Fatalf("appending to the reopened log failed: %v", err)
	}
	if next != 4 {
		t.Errorf("the first event after reopening was numbered %d, want 4", next)
	}
}

func TestAProcessKilledMidWriteLeavesAWholeLog(t *testing.T) {
	home := testkit.NewTempHome(t)
	path := home.DatabaseFile()
	startAndKillTheHelper(t, path)

	checkIntegrity(t, path)

	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopening the log the killed helper left behind failed: %v", err)
	}
	defer reopened.Close()

	written := int64(0)
	err = reopened.Replay(context.Background(), func(event contract.Event) error {
		if event.Sequence != written+1 {
			return fmt.Errorf("event %d came after event %d, and the sequence must have no gaps", event.Sequence, written)
		}
		wanted := fmt.Sprintf(`{"step":%d}`, event.Sequence)
		if string(event.Body) != wanted {
			return fmt.Errorf("event %d carries the body %s, want %s, so it was left half written", event.Sequence, event.Body, wanted)
		}
		written = event.Sequence
		return nil
	})
	if err != nil {
		t.Fatalf("replaying the log the killed helper left behind failed: %v", err)
	}
	if written < 1 {
		t.Fatalf("the killed helper left %d events, and it must have written at least one", written)
	}
	t.Logf("the killed helper left %d whole events", written)
}

// startAndKillTheHelper runs the helper against the file, waits until it says it
// is writing, lets it get well into the loop, and kills that exact process.
func startAndKillTheHelper(t *testing.T, path string) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), helperWaitLimit)
	defer stop()

	helper := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	helper.Env = append(os.Environ(), crashHelperVariable+"="+path)
	helper.Stderr = os.Stderr
	spoken, err := helper.StdoutPipe()
	if err != nil {
		t.Fatalf("cannot listen to the crash helper: %v", err)
	}
	if err := helper.Start(); err != nil {
		t.Fatalf("cannot start the crash helper: %v", err)
	}
	// The process id is saved here and killed below, so that nothing is ever
	// matched by name.
	killed := helper.Process

	line, err := bufio.NewReader(spoken).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("the crash helper never said it was writing: %v", err)
	}
	if line != helperReadyLine+"\n" {
		t.Fatalf("the crash helper said %q, want %q", line, helperReadyLine)
	}

	time.Sleep(writingHeadStart)
	if err := killed.Kill(); err != nil {
		t.Fatalf("cannot kill the crash helper: %v", err)
	}
	_ = helper.Wait()
}

// checkIntegrity asks SQLite itself whether the file the killed helper left is
// whole.
func checkIntegrity(t *testing.T, path string) {
	t.Helper()
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("cannot open %s to check it: %v", path, err)
	}
	defer handle.Close()

	verdict := ""
	if err := handle.QueryRow("PRAGMA integrity_check").Scan(&verdict); err != nil {
		t.Fatalf("cannot check the file %s: %v", path, err)
	}
	if verdict != "ok" {
		t.Fatalf("SQLite says the file %s is damaged: %s", path, verdict)
	}
}
