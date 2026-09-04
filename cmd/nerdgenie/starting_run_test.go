package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestStartTaskRunsAMessageThroughTheLoopAndAnswers drives one ordinary message
// the whole way through startTask: the loop is taken, the turn runs against a
// fake model swapped in for the real chain, and its answer reaches the attached
// screen. This is the normal path that a status question and a carried-on task
// both branch away from.
func TestStartTaskRunsAMessageThroughTheLoopAndAnswers(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	// The real chain points at a model server that is not running in a test, so
	// a fake one is swapped in under the watch that counts what calls cost.
	running.watched.use(testkit.NewFakeModel(testkit.Script{
		Name:          "local-coder",
		ContextLength: 262144,
		Steps:         []testkit.Step{{Text: "here is the answer to your question"}},
	}))
	reader := aScreenAttachedTo(t, running)

	message := contract.Inbound{Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName, Text: "please answer this"}
	if err := running.startTask(context.Background(), newScreenTasks(), message); err != nil {
		t.Fatalf("starting the task failed: %v", err)
	}

	if reply := readReplyText(t, reader); !strings.Contains(reply, "here is the answer") {
		t.Errorf("the task answered %q, want the model's own words", reply)
	}
	waitUntilNotBusy(t, running)
}

// waitUntilNotBusy waits for the task goroutine startTask ran to free the loop,
// so that the test does not tear the agent down while a turn is still going.
func waitUntilNotBusy(t *testing.T, running *agent) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !running.loopIsBusy() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the task never freed the loop, so it is still running after its answer")
}
