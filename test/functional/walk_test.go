// The whole-program test for the /walk command: it is always offered, and on a
// machine with no browser worker built it says so in plain words rather than
// failing in a way nobody can act on.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheWalkCommandIsOfferedAndSaysWhenTheBrowserIsOff(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "help"})
	listed := screen.waitFor(t, contract.SocketReply, 30*time.Second)

	if !strings.Contains(listed.Text, "/walk") {
		t.Errorf("the help listing offers no /walk:\n%s", listed.Text)
	}

	// The worker bundle is not built in a test's temporary folder, so the
	// browser is off, and the command has to say that rather than fall over.
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "walk check something"})
	answered := screen.waitFor(t, contract.SocketReply, 30*time.Second)

	if strings.TrimSpace(answered.Text) == "" {
		t.Fatal("/walk answered with nothing at all on a machine with no browser")
	}
	if !strings.Contains(strings.ToLower(answered.Text), "browser") {
		t.Errorf("/walk answered %q, and it does not say the browser is what is missing", answered.Text)
	}
}
