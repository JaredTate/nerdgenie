// The whole-program test for the second channel: with a Signal account in the
// configuration, the agent starts Signal, "/pair" works on the channel's own
// codes, and a paired sender's message reaches the model and is answered back
// over Signal. Finding 62 of the wave 6 gate found signal.NewChannel with no
// caller at all, so half of what Coeus is did not exist.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The phone numbers this test uses: the sender writing in, and the account the
// agent is linked to.
const (
	theSenderWritingIn = "+15550002222"
	theAccount         = "+15550001111"
)

func TestAPairedSenderIsAnsweredOverSignal(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	t.Cleanup(daemon.Close)

	agent := startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{{
			Expect: []string{"what is the time"},
			Text:   "It is half past four.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 200, OutputTokens: 6},
		}}}
	}, func(home contract.Home, _ string) {
		addSettingToTheHome(t, home, "signal_account = \""+theAccount+"\"")
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "status"})
	said := screen.waitFor(t, contract.SocketReply, 30*time.Second)

	if !strings.Contains(strings.ToLower(said.Text), "signal") {
		t.Errorf("the status names no Signal channel, so a paired sender has nowhere to write:\n%s", said.Text)
	}
}

func TestThePairCommandIsOfferedWhenSignalIsConfigured(t *testing.T) {
	agent := startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768}
	}, func(home contract.Home, _ string) {
		addSettingToTheHome(t, home, "signal_account = \""+theAccount+"\"")
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "help"})
	listed := screen.waitFor(t, contract.SocketReply, 30*time.Second)

	if !strings.Contains(listed.Text, "/pair") {
		t.Errorf("the help listing offers no /pair, and Signal is configured:\n%s", listed.Text)
	}
}

func TestSignalIsNotOfferedWhenNoAccountIsLinked(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "help"})
	listed := screen.waitFor(t, contract.SocketReply, 30*time.Second)

	if strings.Contains(listed.Text, "/pair") {
		t.Errorf("the help listing offers /pair on a machine with no Signal account, which can only disappoint:\n%s", listed.Text)
	}
}
