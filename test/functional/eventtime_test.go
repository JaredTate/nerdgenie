// The whole-program test for the event log's own times: every event a task
// writes carries the moment it happened. Driving the real agent found checkpoint
// and tool-result events dated the year one, which makes the log useless for
// saying when anything happened.
package functional

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
)

func TestEveryEventATaskWritesCarriesTheMomentItHappened(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatReadsAFile)
	writeTheNote(t, agent.work)
	began := time.Now().Add(-time.Minute)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "what does the note say?"})
	screen.waitForReplySaying(t, "kettle", 90*time.Second)

	for kind, occurred := range whenEachKindHappened(t, agent) {
		if occurred.Before(began) {
			t.Errorf("a %s event is dated %s, which is not when it happened", kind, occurred.Format(time.RFC3339))
		}
	}
}

// whenEachKindHappened reads the log back and gives the earliest time each kind
// of event carries, so that one wrong kind fails by name.
func whenEachKindHappened(t *testing.T, agent runningAgent) map[contract.EventKind]time.Time {
	t.Helper()
	ctx, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
	defer giveUp()

	// The agent is still holding the file, and the log opens read-only paths of
	// its own, so a second reader is fine.
	eventLog, err := log.Open(ctx, agent.home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log to read it back failed: %v", err)
	}
	defer func() { _ = eventLog.Close() }()

	earliest := map[contract.EventKind]time.Time{}
	err = eventLog.Replay(ctx, func(event contract.Event) error {
		if held, there := earliest[event.Kind]; !there || event.Occurred.Before(held) {
			earliest[event.Kind] = event.Occurred
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the event log back failed: %v", err)
	}
	if len(earliest) == 0 {
		t.Fatal("the event log is empty, so the task wrote nothing down")
	}
	return earliest
}
