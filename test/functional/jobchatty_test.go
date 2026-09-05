// The whole-program test for a job's task that finishes with a chatty
// question on the end of its reply. A small model ends finished work with
// "Anything else?", and the loop read that one character before the done
// list: a task with every done line proven was put into waiting, and the job
// was put down on it at "1 of 3" with nothing left to do. A proven done list
// closes the task, its report goes into the job, and the job runs its next
// task.
package functional

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theChattyEndOfTheFirstTask is how the job's first task ends its reply.
const theChattyEndOfTheFirstTask = "The tweet is posted. Anything else?"

// aJobWhoseFirstTaskEndsWithAQuestion is the job of jobdone_test.go, with the
// first task doing its work in a tool call, writing and proving its done
// line, and then closing with a question on the end.
func aJobWhoseFirstTaskEndsWithAQuestion(_ string) testkit.Script {
	steps := append(theStepsThatMakeTheJob(),
		testkit.Step{
			Expect: []string{"post the tweet"},
			Text:   "I will post it.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-shell", Name: contract.ToolShell, Input: json.RawMessage(`{"command":"echo posted"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants the tweet posted","doneWhen":["the tweet is posted"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		testkit.Step{
			Text:   "The command ran; I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-proof", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the tweet is posted","done":true,"resultId":"r1"}]}`)}},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		testkit.Step{
			Text:   theChattyEndOfTheFirstTask,
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 10},
		},
		testkit.Step{
			Expect: []string{"write the summary for the user"},
			Text:   "The summary is written.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 5},
		},
		theReviewStep(),
	)
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: steps}
}

func TestAJobsTaskWithAProvenDoneListClosesWhateverQuestionEndsItsReply(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobWhoseFirstTaskEndsWithAQuestion)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskForTheJobToSteer})

	report := screen.waitForReplySaying(t, "Job "+theJobToSteer+", report j"+theJobToSteer+".1: 1 of 2 tasks done", 90*time.Second)
	if !strings.Contains(report.Text, theChattyEndOfTheFirstTask) {
		t.Errorf("the first task's report reads %q, want the reply it finished with, question and all", report.Text)
	}
	if strings.Contains(report.Text, "waiting on task") {
		t.Errorf("the first task's report reads %q, and a task whose done list is proven is finished, not waiting", report.Text)
	}
	screen.waitForReplySaying(t, "Job "+theJobToSteer+", report j"+theJobToSteer+".2: 2 of 2 tasks done", 90*time.Second)
	screen.waitForReplySaying(t, "Job "+theJobToSteer+" is finished", 90*time.Second)
}
