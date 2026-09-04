package main

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestWithoutAGuardTheChannelIsLeftUnwrapped(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	where := running.userChannel()

	got := throughTheLedger(where, nil)

	if _, wrapped := got.(ledgeredChannel); wrapped {
		t.Error("a channel with no guard was wrapped in the ledger, and with no guard there is nothing to write the reply down in")
	}
}

func TestALedgeredReplyIsWrittenDownBeforeItIsSent(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	// A screen has to be attached for the send itself to reach anybody; without
	// one the reply is written down and then owed, which is a path of its own.
	reader := aScreenAttachedTo(t, running)

	ledgered := throughTheLedger(running.userChannel(), running.guard)
	if _, wrapped := ledgered.(ledgeredChannel); !wrapped {
		t.Fatal("a channel with a guard was not wrapped in the ledger, so its replies would not be written down")
	}

	if err := ledgered.Send(context.Background(), "the answer to the question"); err != nil {
		t.Fatalf("sending through the ledger failed: %v", err)
	}
	if reply := readReplyText(t, reader); reply != "the answer to the question" {
		t.Errorf("the screen was sent %q, want the ledgered reply", reply)
	}

	replies, err := running.events.ByKind(context.Background(), contract.EventReply)
	if err != nil {
		t.Fatalf("reading the replies out of the log failed: %v", err)
	}
	if len(replies) == 0 {
		t.Error("the reply was sent without being written down first, so a crash between the two would lose it")
	}
}
