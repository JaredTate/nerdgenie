// The whole-program tests for the jobs the agent runs on its own: a job that
// gives up says so to the person, the nightly self-check outlives a run of bad
// nights, and the loop that looks for due work rests between looks.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheNightlySelfCheckKeepsRunningAfterABadNight(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	// Ten nights in a row, asked for one after another. A job that stopped
	// itself after a run of failures would be switched off by the end of them,
	// and the self-check is the one job that must not be: it exists to say when
	// something is wrong, and a run of bad nights is when it is needed most.
	number := theNightlyJobNumber(t, screen)
	for at := 0; at < 10; at++ {
		screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "cron run " + number})
		screen.waitFor(t, contract.SocketReply, 30*time.Second)
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "cron"})
	listed := screen.waitFor(t, contract.SocketReply, 30*time.Second)
	if !strings.Contains(strings.ToLower(listed.Text), "running") {
		t.Errorf("the nightly self-check is no longer running after ten nights:\n%s", listed.Text)
	}
}

func TestTheDueJobsLoopRestsWhileATaskIsRunning(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Text:   "the answer",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 100, OutputTokens: 3},
		}},
	})
	agent.model.MisbehaveNext(testkit.StallTheStream, 3*time.Second)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "take a moment over this"})
	screen.waitForStatusCarrying(t, contract.StatusFieldCallStarted, 30*time.Second)

	// While that call is in flight the job driver has a nightly job whose next
	// moment is in the past. A loop that skipped its rest would turn hundreds of
	// thousands of times a second; one that rests answers a command as promptly
	// as an idle agent does.
	asked := time.Now()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "status"})
	screen.waitFor(t, contract.SocketReply, 20*time.Second)
	if waited := time.Since(asked); waited > time.Second {
		t.Errorf("a command took %s to answer while a task ran, so the job driver is taking the machine with it",
			waited.Round(time.Millisecond))
	}
}

// theNightlyJobNumber is the number of the one job a fresh agent has.
func theNightlyJobNumber(t *testing.T, screen *attachedScreen) string {
	t.Helper()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "cron"})
	listed := screen.waitFor(t, contract.SocketReply, 30*time.Second)
	return theNightlyJobIn(t, listed.Text)
}
