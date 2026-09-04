// The whole-program test for the nightly self-check: the job that fires it is
// there the moment the agent starts, and the task it puts on the list is run by
// the check itself rather than handed to the model.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheNightlySelfCheckIsRegisteredWhenTheAgentStarts(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "jobs"})

	listed := screen.waitFor(t, contract.SocketReply, 20*time.Second)
	if !strings.Contains(strings.ToLower(listed.Text), theNightlyJobsOwnWords) {
		t.Errorf("the jobs are %q, and the nightly self-check is not among them", listed.Text)
	}
}

func TestTheNightlyTaskIsRunByTheCheckAndNotByTheModel(t *testing.T) {
	// The script has no steps at all: a model call would fail the run outright,
	// which is what proves the nightly task never reaches the model.
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "cron"})
	listed := screen.waitFor(t, contract.SocketReply, 20*time.Second)

	if !strings.Contains(strings.ToLower(listed.Text), theNightlyJobsOwnWords) {
		t.Fatalf("the scheduled jobs are %q, and the nightly self-check is not among them", listed.Text)
	}
	// Asking for it now puts its task on the list, and the agent's own job
	// driver picks it up within the second.
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "cron run " + theNightlyJobIn(t, listed.Text)})
	screen.waitFor(t, contract.SocketReply, 20*time.Second)

	// The self-check answers with its own one line. A task handed to a model
	// with no script left in it comes back saying it could not be finished, so
	// the two are told apart by which of them arrives.
	said := screen.waitForReplySaying(t, "self-check", 60*time.Second)
	if strings.Contains(strings.ToLower(said.Text), "could not finish") {
		t.Errorf("the nightly task was answered with %q, which is what a task handed to the model says", said.Text)
	}
}

// theNightlyJobsOwnWords is what the nightly job's goal really says, so that the
// test reads the line a person would read rather than a name for it.
const theNightlyJobsOwnWords = "check yourself every night"

// theNightlyJobIn reads the nightly job's number out of what "/cron" printed.
func theNightlyJobIn(t *testing.T, listed string) string {
	t.Helper()
	for _, line := range strings.Split(listed, "\n") {
		trimmed := strings.TrimSpace(line)
		number, rest, found := strings.Cut(trimmed, " ")
		if !found || number == "" || !strings.Contains(rest, "running") {
			continue
		}
		return number
	}
	t.Fatalf("no line of %q names the nightly job", listed)
	return ""
}
