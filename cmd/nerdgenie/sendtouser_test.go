package main

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestSendingToTheUserWithNoScreenSaysTheReplyIsOwed(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// No screen is attached, so a send is not a delivery; the reply is owed
	// rather than thrown away, so the ledger does not read it as delivered.
	if err := running.sendToTheUser(context.Background(), contract.TerminalChannelName, "still owed"); err == nil {
		t.Error("a send with no screen attached said the reply reached the user")
	}
}

func TestSendingToAChannelThatIsNotRunningSaysSo(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if err := running.sendToTheUser(context.Background(), "a-channel-nobody-runs", "hello"); err == nil {
		t.Error("a send to a channel the agent is not running said nothing was wrong")
	}
}

func TestShortenedKeepsAShortLineAndCutsALongOne(t *testing.T) {
	if got := shortened("a short line"); got != "a short line" {
		t.Errorf("a short line was changed to %q", got)
	}
	long := strings.Repeat("x", 200)
	got := shortened(long)
	if len(got) > 60 {
		t.Errorf("a long line was cut to %d letters, want no more than sixty", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a cut line %q does not say the rest was cut", got)
	}
}
