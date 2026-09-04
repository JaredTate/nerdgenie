// The whole-program test for "/clear": a task has asked a question and is
// waiting for the answer, the person types /clear, and what they type next is a
// fresh task rather than the answer to that question. Without the clear, any
// message at all carries a waiting task on, which resume_test.go proves.
package functional

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theAskAfterTheClear is what the person types once the screen is clear. It is
// the same words that answered the question in resume_test.go, so that the only
// thing that makes it a fresh task is the clear between.
const theAskAfterTheClear = "the top one"

// theClearedReply is the one line the clear command answers with.
const theClearedReply = "cleared: the next message starts a fresh task"

// aTaskThatAsksAQuestionAndIsCleared is two tasks in four calls: read the file
// and open the record, ask the person which cupboard they meant, and then, on a
// fresh task, open a record of its own for what the person typed next and
// report.
func aTaskThatAsksAQuestionAndIsCleared(work string) testkit.Script {
	path := filepath.Join(work, theFileTheToolReads)
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Expect: []string{theAskThatIsAnsweredWithAQuestion},
			Text:   "I will open the file.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"` + path + `"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants what the file says","doneWhen":["the file has been read"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   "The note names two cupboards. Which cupboard did you mean?",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 500, OutputTokens: 12},
		},
		{
			Expect: []string{theAskAfterTheClear},
			Text:   "I will write down what is wanted.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-fresh", Name: contract.ToolTask, Input: json.RawMessage(
				`{"why":"the user names a cupboard","doneWhen":["the cupboard is named"]}`)}},
			Usage: contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
		{
			Text:   "The top cupboard it is. What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 700, OutputTokens: 12},
		},
	}}
}

func TestAClearBetweenTwoMessagesStartsTheSecondAsAFreshTask(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatAsksAQuestionAndIsCleared)
	writeTheNote(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatIsAnsweredWithAQuestion})
	screen.waitForReplySaying(t, "which cupboard did you mean", 60*time.Second)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "clear"})
	cleared := screen.waitForReplySaying(t, "cleared", 15*time.Second)
	if !cleared.Clear {
		t.Error("the reply to /clear does not tell the screen to empty its transcript")
	}
	if cleared.Text != theClearedReply {
		t.Errorf("the reply to /clear says %q, want %q", cleared.Text, theClearedReply)
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskAfterTheClear})

	started := screen.waitForRecordLineSaying(t, theAskAfterTheClear, 60*time.Second)
	if !strings.HasPrefix(started, "task 2 ") {
		t.Errorf("the message after /clear was written down as %q, and it should start task 2 rather than answer task 1's question", started)
	}
}
