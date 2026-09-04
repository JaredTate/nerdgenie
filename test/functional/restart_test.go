// The whole-program test for the memory of what each screen was last doing
// surviving a restart: a task stops, "coeus serve" is stopped and started again
// on the same home, and the word "continue" picks the same task up under the
// same number. Before this, that memory lived in the process, so after a restart
// "continue" started a fresh task and the record's own numbering and budget were
// lost, which is what the fourth Tetris run of the wave 6 trial found.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestTheWordContinueAfterARestartCarriesOnTheTaskThatStopped(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskStoppedAtItsBudget,
		func(home contract.Home, _ string) {
			addSettingToTheHome(t, home, "[caps]\nrounds_per_task = 1")
		})
	writeTheNote(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatRunsOutOfBudget})
	// The record line saying the task stopped is sent once the record has been
	// written, so waiting for it is waiting for the log to hold the stop.
	stopped := screen.waitForRecordLineSaying(t, " stopped", 60*time.Second)
	if !strings.HasPrefix(stopped, "task 1 ") {
		t.Fatalf("the task that ran out of budget was written down as %q, want task 1 stopped", stopped)
	}

	restarted := agent.restart(t)

	screen = restarted.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "continue"})

	carriedOn := screen.waitForRecordLineSaying(t, "continue", 60*time.Second)
	if !strings.HasPrefix(carriedOn, "task 1 ") {
		t.Errorf("the word continue after the restart was written down as %q, and it should carry on task 1 rather than start a task of its own",
			carriedOn)
	}
}
