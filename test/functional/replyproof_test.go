// The whole-program test for a task whose done list the answer itself proves,
// which the wave 6 live serve found: the person asked for a three-word reply,
// the model had nothing to point its one done line at, and the task failed.
package functional

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theThreeWordsAsked is the whole of what the person wanted, and the whole of
// what proves the done line.
const theThreeWordsAsked = "Coeus is running."

// aTaskTheAnswerItselfProves is one whole task in two calls: write down what
// done looks like, naming the answer as the proof of the one line, and then
// give the answer.
func aTaskTheAnswerItselfProves(_ string) testkit.Script {
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Text:   "Nothing is written yet. I will write down what done looks like.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"operation":"done_when","why":"the user wants three words",` +
						`"done_when":[{"text":"reply to the user with exactly three words","done":true,"result":"reply"}]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   theThreeWordsAsked,
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 500, OutputTokens: 10},
		},
	}}
}

// TestATaskTheAnswerItselfProvesCloses drives the real task tool, the real
// record, and the real done-check through the socket a screen uses.
func TestATaskTheAnswerItselfProvesCloses(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskTheAnswerItselfProves)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "reply with exactly three words"})
	replied := screen.waitFor(t, contract.SocketReply, 90*time.Second).Text

	if !strings.Contains(replied, theThreeWordsAsked) {
		t.Errorf("the user was sent %q, want the three words they asked for", replied)
	}
	if strings.Contains(replied, "could not finish") {
		t.Errorf("the task failed on a done line the answer itself proves: %q", replied)
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "tasks 1"})
	written := screen.waitFor(t, contract.SocketReply, 30*time.Second).Text

	header := strings.SplitN(written, "\n", 2)[0]
	if !strings.Contains(header, string(contract.StatusDone)) {
		t.Errorf("the record of the task opens %q and reads:\n%s\nand it never reached done", header, written)
	}
	if !strings.Contains(written, "the reply to the user") {
		t.Errorf("the record of the task reads:\n%s\nand the answer that proved its done line is not a result in it", written)
	}
	if !strings.Contains(written, "[x] reply to the user with exactly three words") {
		t.Errorf("the record of the task reads:\n%s\nand its one done line was never ticked", written)
	}
}
