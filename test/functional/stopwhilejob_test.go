// The whole-program test for stopping the task that made a job. The live run
// found the report misleading: the person's task made job 2, its stopped
// report said "tell me how to carry on", and the "continue" typed next went
// to the job's first task, which the driver had started the moment the task
// ended. A stopped task that made a job which is now running names the job
// and its next task and says how to stop that too, and invites no carry-on.
package functional

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theSecondsTheMakingTaskLingers is how long the task that made the job is
// busy in a tool after making it, which is the window the stop lands in.
const theSecondsTheMakingTaskLingers = 5

// aJobWhoseMakingTaskIsStopped is the job of midturnjob_test.go, with the
// task that makes it lingering in a shell command after the job is made, so
// that the person's stop lands on the task that made the job while the job
// is running; then the job's two tasks and its review.
func aJobWhoseMakingTaskIsStopped(_ string) testkit.Script {
	making := theStepsThatMakeTheJob()[:2]
	steps := append(making,
		testkit.Step{
			Text:   "The job is made; I will give it a moment before I sum up.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-linger", Name: contract.ToolShell, Input: json.RawMessage(
				fmt.Sprintf(`{"command":"sleep %d"}`, theSecondsTheMakingTaskLingers))}},
			Usage: contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
		testkit.Step{
			Expect: []string{"post the tweet"},
			Text:   "The tweet is posted.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 5},
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

func TestStoppingTheTaskThatMadeAJobNamesTheJobInsteadOfInvitingACarryOn(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobWhoseMakingTaskIsStopped)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskForTheJobToSteer})
	// The stop lands while the task that made the job is busy in its shell
	// command, which is a person's task, so the status names no job.
	screen.waitForStatusWhere(t, 30*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == "" &&
			strings.Contains(fields[contract.StatusFieldToolLine], contract.ToolShell)
	})
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "stop"})

	stopped := screen.waitForReplySaying(t, "I stopped this task", theSecondsTheMakingTaskLingers*time.Second+30*time.Second)

	for _, words := range []string{"Job " + theJobToSteer, "t1", "say stop to stop that too"} {
		if !strings.Contains(stopped.Text, words) {
			t.Errorf("the stopped report is %q, want it to say %q", stopped.Text, words)
		}
	}
	if strings.Contains(strings.ToLower(stopped.Text), "carry on") {
		t.Errorf("the stopped report is %q, and it invites a carry-on of a task whose work is now the job's", stopped.Text)
	}
	// The job then runs on its own, as the report said it would.
	screen.waitForReplySaying(t, "Job "+theJobToSteer+", report j"+theJobToSteer+".1: 1 of 2 tasks done", 60*time.Second)
	screen.waitForReplySaying(t, "Job "+theJobToSteer+" is finished", 90*time.Second)
}
