package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestAReplyStreamsToTheScreenBeforeItArrivesWhole is the wave 5 checklist's
// "streaming visible": at least one piece of the answer reaches the screen as a
// delta before the reply does, and the pieces are the start of the reply.
func TestAReplyStreamsToTheScreenBeforeItArrivesWhole(t *testing.T) {
	answer := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 6)
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{Text: answer, Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 100, OutputTokens: 60}},
	}})
	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "say the fox sentence six times"})

	pieces := ""
	reply := screen.waitUntil(t, 30*time.Second, "the reply after its pieces", func(envelope contract.SocketEnvelope) bool {
		if envelope.Type == contract.SocketDelta && !envelope.Reset {
			pieces += envelope.Text
		}
		return envelope.Type == contract.SocketReply
	})
	if pieces == "" {
		t.Fatalf("no piece of the answer reached the screen before the reply %q", reply.Text)
	}
	if !strings.HasPrefix(reply.Text, pieces) {
		t.Errorf("the pieces %q are not the start of the reply %q", pieces, reply.Text)
	}
}
