package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheComputerToolSaysWhenTheDesktopWorkerIsNotBuilt is the wiring's half of
// brief 6.7's finding 48: the desktop is opened beside the browser at start,
// and a binary with no desktop worker beside it says so in its log and refuses
// the computer tool in plain words rather than crashing or staying silent.
func TestTheComputerToolSaysWhenTheDesktopWorkerIsNotBuilt(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})
	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "readyz"})
	screen.waitFor(t, contract.SocketReply, 20*time.Second)

	said := whatItSaid(agent.saidPath)
	if !strings.Contains(said, "the desktop worker is not built, so the computer tool is switched off") {
		t.Errorf("the serve did not say the desktop worker is missing:\n%s", said)
	}
}
