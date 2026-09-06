package shell_test

// The poll tests: what a poll answers while a command runs and once it has
// finished, and the helper that polls from another goroutine, because a poll
// on a running command waits.

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// pollInTheBackground polls a command from another goroutine and hands the
// answer back on a channel, because a poll on a running command waits.
func pollInTheBackground(t *testing.T, tool *shell.Tool, id string) <-chan contract.ToolOutput {
	t.Helper()
	answers := make(chan contract.ToolOutput, 1)
	go func() {
		polled, err := run(t, tool, map[string]any{"action": "poll", "id": id})
		if err != nil {
			t.Errorf("polling %s failed: %v", id, err)
			polled = contract.ToolOutput{Text: err.Error()}
		}
		answers <- polled
	}()
	return answers
}

func TestAPollWaitsForTheCommandAndHandsBackWhatItDid(t *testing.T) {
	sandbox := newSlowSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("done at last\n")})
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), clock)

	started := make(chan contract.ToolOutput, 1)
	go func() {
		output, _ := run(t, tool, map[string]any{"command": "sleep 30"})
		started <- output
	}()
	waitForSleepers(t, clock, 1)
	clock.Advance(shell.YieldAfter)
	<-started

	// The fresh game build's play-test task polled its own script every five
	// seconds, a round each, fourteen rounds for one run. A poll now waits for
	// the command, so the command finishing is what answers it, not the clock.
	polled := pollInTheBackground(t, tool, shell.FirstProcessID)
	waitForSleepers(t, clock, 1)
	sandbox.Release()
	select {
	case answer := <-polled:
		if !strings.Contains(answer.Text, "done at last") {
			t.Fatalf("the poll said %q, want what the command wrote", answer.Text)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the poll did not answer when the command finished")
	}
}
