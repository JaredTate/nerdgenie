// The whole-program test for a screen that asks a question while the model is
// busy: a command that only reads has to answer at once, from any screen, or a
// person watching a long task has no way to see what is happening.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// howLongAReadingCommandMayTake is what a person is promised: a command that
// only reads is answered at once, whatever the model is doing.
const howLongAReadingCommandMayTake = time.Second

func TestAReadingCommandIsAnsweredWhileTheModelIsBusy(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Text:   "this reply arrives long after the question was asked",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 100, OutputTokens: 5},
		}},
	})
	agent.model.MisbehaveNext(testkit.StallTheStream, 30*time.Second)

	working := agent.attach(t)
	working.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "take a long time over this"})
	working.waitForStatusCarrying(t, contract.StatusFieldCallStarted, 30*time.Second)

	// A second screen, exactly as a person opening another terminal would.
	watching := agent.attach(t)
	for _, typed := range []string{"status", "tasks", "jobs", "help"} {
		asked := time.Now()
		watching.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: typed})
		reply := watching.waitFor(t, contract.SocketReply, 20*time.Second)
		if waited := time.Since(asked); waited > howLongAReadingCommandMayTake {
			t.Errorf("/%s was answered after %s, and a reading command must answer inside %s",
				typed, waited.Round(time.Millisecond), howLongAReadingCommandMayTake)
		}
		if strings.TrimSpace(reply.Text) == "" {
			t.Errorf("/%s answered with nothing at all", typed)
		}
	}
}
