// The whole-program test for the record line: when a task starts and when it
// ends, the status carries one line saying so, which is how a person sees what
// the agent is working on. The first human trial asked how to see the tasks and
// jobs at all.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theAskTheRecordLineNames is what the user types, and what the line about the
// task has to carry so that a person can tell one task from another.
const theAskTheRecordLineNames = "count the jars in the cupboard"

func TestTheStatusCarriesOneRecordLineWhenATaskStartsAndWhenItEnds(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Expect: []string{theAskTheRecordLineNames},
			Text:   "There are four jars.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 6},
		}},
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskTheRecordLineNames})

	started := screen.waitForStatusCarrying(t, contract.StatusFieldRecordLine, 60*time.Second)
	line := started.Fields[contract.StatusFieldRecordLine]
	if !strings.HasPrefix(line, "task ") {
		t.Errorf("the record line is %q, and it does not begin by naming the task", line)
	}
	if !strings.Contains(line, theAskTheRecordLineNames) {
		t.Errorf("the record line is %q, and it does not say what the task is about", line)
	}
	if !strings.Contains(line, "started") {
		t.Errorf("the first record line is %q, and it does not say the task started", line)
	}

	ended := screen.waitForStatusWhere(t, 60*time.Second, func(fields map[string]string) bool {
		return strings.Contains(fields[contract.StatusFieldRecordLine], string(contract.StatusDone))
	})
	if !strings.Contains(ended.Fields[contract.StatusFieldRecordLine], theAskTheRecordLineNames) {
		t.Errorf("the record line at the end is %q, and it does not say which task ended",
			ended.Fields[contract.StatusFieldRecordLine])
	}
}
