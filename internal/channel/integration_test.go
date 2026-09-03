//go:build integration

package channel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheQueueAndTheEventLogShareTheOneDatabaseFile(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)

	// The event log opens first, because it owns the file and refuses one whose
	// tables are not its own. Everything that starts the agent opens it first.
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log in the temporary home failed: %v", err)
	}
	defer func() { _ = eventLog.Close() }()

	queue, err := OpenQueue(ctx, home.DatabaseFile(), contract.DefaultConfig().Caps.QueuedMessages)
	if err != nil {
		t.Fatalf("opening the queue beside the event log failed: %v", err)
	}

	sequence, err := queue.Add(ctx, anInbound("book me a flight to Denver"))
	if err != nil {
		t.Fatalf("adding a message to the queue on the real database failed: %v", err)
	}
	if _, err := eventLog.Append(ctx, contract.Event{
		Occurred: arrived,
		TaskID:   "t1",
		Kind:     contract.EventMessage,
		Body:     json.RawMessage(`{"text":"book me a flight to Denver"}`),
	}); err != nil {
		t.Fatalf("the event log stopped working once the queue shared its file: %v", err)
	}

	taken, held, err := queue.Take(ctx)
	if err != nil || !held {
		t.Fatalf("taking the message failed: held %t, error %v", held, err)
	}
	if taken.Sequence != sequence || taken.Message.Text != "book me a flight to Denver" {
		t.Fatalf("the queue handed out %+v, want message %d", taken, sequence)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("closing the queue failed: %v", err)
	}

	takeItAgainAfterTheRestart(t, home)

	events, err := eventLog.ByTask(ctx, "t1")
	if err != nil || len(events) != 1 {
		t.Fatalf("the event log read back %d events with the error %v, want the one that was written", len(events), err)
	}
}

// takeItAgainAfterTheRestart opens the queue in the same home a second time,
// which is what happens when the agent stopped between taking a message and
// finishing the task it started, and checks the message comes back marked.
func takeItAgainAfterTheRestart(t *testing.T, home contract.Home) {
	t.Helper()
	ctx := context.Background()
	reopened, err := OpenQueue(ctx, home.DatabaseFile(), contract.DefaultConfig().Caps.QueuedMessages)
	if err != nil {
		t.Fatalf("reopening the queue failed: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	again, held, err := reopened.Take(ctx)
	if err != nil || !held {
		t.Fatalf("the message was not handed out again after the restart: held %t, error %v", held, err)
	}
	if !again.Duplicate {
		t.Error("the message came back without the duplicate marker, and the task it started may already have run")
	}
	if err := reopened.Done(ctx, again.Sequence); err != nil {
		t.Fatalf("marking the message done failed: %v", err)
	}
}

func TestTheEventLogRefusesAFileThatHoldsOnlyTheQueue(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)

	queue, err := OpenQueue(ctx, home.DatabaseFile(), 10)
	if err != nil {
		t.Fatalf("opening the queue on a new file failed: %v", err)
	}
	defer func() { _ = queue.Close() }()

	// This is the reason the event log is opened first: a file holding only the
	// queue's table is not one the log will take.
	if _, err := log.Open(ctx, home.DatabaseFile()); err == nil {
		t.Fatal("the event log opened a file holding only the queue's table, so the order the two are opened in no longer matters and this test can go")
	}
}

func TestTheWholeSocketRunsInARealHomeFolder(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)

	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log failed: %v", err)
	}
	defer func() { _ = eventLog.Close() }()

	queue, err := OpenQueue(ctx, home.DatabaseFile(), 10)
	if err != nil {
		t.Fatalf("opening the queue failed: %v", err)
	}
	defer func() { _ = queue.Close() }()

	stream := NewStream()
	defer stream.Close()
	socket, err := Listen(Options{
		Path:    home.SocketFile(),
		Stream:  stream,
		Queue:   queue,
		Secrets: testkit.NewFakeSecrets(),
		Clock:   testkit.NewFakeClock(arrived),
	})
	if err != nil {
		t.Fatalf("opening the socket in the home folder's run folder failed: %v", err)
	}
	served := make(chan error, 1)
	serving, stopServing := context.WithCancel(ctx)
	go func() { served <- socket.Serve(serving) }()
	defer func() {
		stopServing()
		_ = socket.Close()
		if err := <-served; err != nil {
			t.Errorf("serving the socket ended with %v", err)
		}
	}()

	if socket.Name() != contract.TerminalChannelName {
		t.Errorf("the socket calls itself %q, want %q", socket.Name(), contract.TerminalChannelName)
	}
	details, err := os.Stat(home.SocketFile())
	if err != nil {
		t.Fatalf("cannot look at the socket in the home folder: %v", err)
	}
	if mode := details.Mode().Perm(); mode != contract.SecretFileMode {
		t.Errorf("the socket's mode is %o, want %o", mode, contract.SecretFileMode)
	}
	if folder := filepath.Dir(home.SocketFile()); folder != home.RunFolder() {
		t.Errorf("the socket sits in %s, want the home folder's run folder %s", folder, home.RunFolder())
	}
	if held, err := queue.Held(ctx); err != nil || held != 0 {
		t.Errorf("the queue holds %d messages before anything was sent, with the error %v", held, err)
	}
	if health := socket.Health(ctx); !health.Healthy {
		t.Errorf("a socket in a real home says it is not working: %s", strings.TrimSpace(health.Detail))
	}
}
