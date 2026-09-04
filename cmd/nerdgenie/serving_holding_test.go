package main

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheDrainerRoutesAQueuedMessageAndFinishesIt puts a status question on the
// queue and drains one pass: the message is routed, answered from the records
// with no task started, and marked done so a restart does not hand it out again.
func TestTheDrainerRoutesAQueuedMessageAndFinishesIt(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	reader := aScreenAttachedTo(t, running)

	if _, err := running.queue.Add(context.Background(), contract.Inbound{
		Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName, Text: "status",
	}); err != nil {
		t.Fatalf("adding a message to the queue failed: %v", err)
	}

	if more := running.takeWhatIsWaiting(context.Background()); more {
		t.Error("the drainer says more messages are waiting after the only one, so it would spin")
	}
	if reply := readReplyText(t, reader); !strings.Contains(reply, nothingInFlight) {
		t.Errorf("the drained status question was answered %q, want the records' own answer", reply)
	}
	if running.loop.Running() != "" {
		t.Errorf("a task %q is running after a drained status question", running.loop.Running())
	}
}

// TestTheDrainerHoldsAMessageUntilATaskTakesItOver walks the little state
// machine that decides who finishes a queued message: the drainer holds it, a
// task that starts from it takes it over, and after that the drainer no longer
// owns it.
func TestTheDrainerHoldsAMessageUntilATaskTakesItOver(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	running.holdTheMessage(5)
	if !running.stillHolding() {
		t.Fatal("the drainer does not hold the message it just took, so nothing would finish it")
	}

	finish := running.tookTheMessage()
	if running.stillHolding() {
		t.Error("the drainer still holds a message a task took over, so it would be finished twice")
	}

	// Finishing a sequence the queue never held is noted rather than fatal, and
	// this exercises that path without a message really being on the queue.
	finish()
}

func TestAMessageThatStartsNoTaskIsFinishedByTheDrainer(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	running.holdTheMessage(9)
	// A message that started no task is still held, so the drainer finishes it.
	if !running.stillHolding() {
		t.Fatal("a message that started no task is not held, so the drainer would never finish it")
	}
	running.finishTheMessage(9)
}
