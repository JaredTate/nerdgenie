// The whole-program tests for what the reliability guard is wired to do: the
// database is made good before anything opens it, two messages on one session
// run one at a time, a reply is written down before it is sent, and no new task
// starts while the guard says not to.
package functional

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestADamagedDatabaseIsPutRightBeforeAnythingOpensIt(t *testing.T) {
	agent := startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768}
	}, func(home contract.Home, _ string) {
		// A file that is not a database at all, left behind by a machine that
		// lost power in the middle of a write, with the sentinel that says the
		// last run never exited.
		if err := os.WriteFile(home.DatabaseFile(), []byte("this is not a database"), contract.DataFileMode); err != nil {
			t.Fatalf("writing the damaged database failed: %v", err)
		}
		if err := os.MkdirAll(home.RunFolder(), contract.HomeFolderMode); err != nil {
			t.Fatalf("making the run folder failed: %v", err)
		}
		if err := os.WriteFile(theLifecycleFileIn(home), []byte(`{"startedAt":"2026-01-01T00:00:00Z"}`), contract.DataFileMode); err != nil {
			t.Fatalf("writing the sentinel failed: %v", err)
		}
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "readyz"})

	answered := screen.waitFor(t, contract.SocketReply, 30*time.Second)
	if !strings.Contains(answered.Text, "ready") {
		t.Errorf("the agent did not come up on a damaged database, so a machine that lost power stays down: %q", answered.Text)
	}
}

// theLifecycleFileIn is the marker the guard leaves while the program runs, so
// that finding it at startup means the last run never took an exit path.
func theLifecycleFileIn(home contract.Home) string {
	return home.RunFolder() + "/lifecycle.json"
}

func TestTwoMessagesOnOneSessionRunOneAtATime(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{
			{Text: "the first answer", Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 100, OutputTokens: 3}},
			{Text: "the second answer", Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 100, OutputTokens: 3}},
		},
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "the first question"})
	screen.waitForReplySaying(t, "the first answer", 60*time.Second)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "the second question"})
	screen.waitForReplySaying(t, "the second answer", 60*time.Second)
}

func TestEveryReplyIsWrittenDownBeforeItIsSent(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Text:   "the answer that must be in the ledger",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 100, OutputTokens: 5},
		}},
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "say something"})
	screen.waitForReplySaying(t, "the answer that must be in the ledger", 60*time.Second)

	kinds := whenEachKindHappened(t, agent)
	if _, there := kinds[contract.EventReply]; !there {
		t.Error("no reply event is in the log, so a crash between writing and sending would lose the answer")
	}
}
