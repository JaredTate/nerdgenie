package main

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

// aMomentForTheStreamToCarryIt is how long a test waits for the envelope the
// clear command publishes to reach a subscriber of the stream.
const aMomentForTheStreamToCarryIt = 5 * time.Second

func TestTheClearCommandForgetsTheTerminalsTaskAndTellsTheScreenToClear(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})
	watching, err := running.stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing to the stream failed: %v", err)
	}
	defer watching.Close()

	ctx, giveUp := context.WithTimeout(context.Background(), aMomentForTheStreamToCarryIt)
	defer giveUp()
	answer, err := running.clearCommand(remembered).Run(ctx, "", contract.CommandContext{Channel: running.userChannel()})
	if err != nil {
		t.Fatalf("running /clear failed: %v", err)
	}
	if answer != "" {
		t.Errorf("/clear answered %q on top of the reply it sends itself, so the screen would be told twice", answer)
	}
	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "" {
		t.Errorf("the next message would carry on task %q after /clear, and a clear means the next message starts a fresh task", picked)
	}

	sent := theReplyOn(t, watching.Events())
	if !sent.Clear {
		t.Error("the reply /clear sent does not tell the screen to empty its transcript")
	}
	if sent.Text != theClearedText {
		t.Errorf("the reply says %q, want %q", sent.Text, theClearedText)
	}
}

// theReplyOn reads the stream until a reply arrives, stepping over the status the
// stream sends a new subscriber, and fails when none arrives inside the moment.
func theReplyOn(t *testing.T, events <-chan contract.SocketEnvelope) contract.SocketEnvelope {
	t.Helper()
	deadline := time.After(aMomentForTheStreamToCarryIt)
	for {
		select {
		case envelope, more := <-events:
			if !more {
				t.Fatal("the stream closed before the reply arrived")
			}
			if envelope.Type == contract.SocketReply {
				return envelope
			}
		case <-deadline:
			t.Fatalf("no reply reached the stream in %s", aMomentForTheStreamToCarryIt)
		}
	}
}

func TestTheClearCommandIsRegisteredAndKeptToTheTerminal(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	found, there := running.registry.Lookup(clearName)
	if !there {
		t.Fatalf("/%s is not registered, and the person asked for it", clearName)
	}
	if !found.TerminalOnly {
		t.Errorf("/%s runs on any channel, and only the terminal has a transcript to clear", clearName)
	}
	if found.Help == "" {
		t.Errorf("/%s has no help line, and the palette shows one for every command", clearName)
	}
}
