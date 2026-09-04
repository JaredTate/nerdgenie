// The whole-program test for the Escape key: a stop typed while the model is
// thinking cancels the call in flight and the person is told inside a second.
// The first human trial found that Escape did nothing at all.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// howLongAStopMayTake is what the person is promised: a stop typed while the
// model is thinking is answered inside a second.
const howLongAStopMayTake = time.Second

func TestAStopWhileTheModelIsThinkingIsAnsweredInsideASecond(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Text:   "this reply never arrives, because the call is stopped first",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 100, OutputTokens: 5},
		}},
	})
	// The model is made to say nothing at all for far longer than the test
	// runs, which is a model that has gone quiet: exactly what the person was
	// looking at when Escape did nothing.
	agent.model.MisbehaveNext(testkit.StallTheStream, time.Minute)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "think about this for a long time"})
	screen.waitForStatusCarrying(t, contract.StatusFieldCallStarted, 30*time.Second)

	// The screen's Escape key sends exactly this.
	asked := time.Now()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "stop"})

	reply := screen.waitForReplySaying(t, "stopped", 15*time.Second)
	if waited := time.Since(asked); waited > howLongAStopMayTake {
		t.Errorf("the stop was answered after %s, and the person is promised an answer inside %s: %q",
			waited.Round(time.Millisecond), howLongAStopMayTake, reply.Text)
	}
	if !strings.Contains(strings.ToLower(reply.Text), "stopped") {
		t.Errorf("the reply is %q, and it does not say the task was stopped", reply.Text)
	}
}
