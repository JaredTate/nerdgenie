// The whole-program test for a message typed while a job's task is running. The
// design promises that a message sent during a task reaches that task as soon as
// the tool call in flight has finished: a correction goes into the record and
// steers the work, and "stop" stops it. That was true of a task a person started
// and not of a task the job driver started, so a person watching a job being
// built could say "use TypeScript" or "stop" and not be heard until the whole
// job was over, when their words started a task of their own.
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

// theAskForTheJobToSteer is what the person types to make the job whose first
// task they then speak to.
const theAskForTheJobToSteer = "run the two-part campaign: post the tweet, then write me the summary"

// theJobToSteer is the number the model's job takes: the nightly self-check is
// registered first on every fresh home, so the model's is the second.
const theJobToSteer = "2"

// theCorrectionTypedMidJob is what the person types while the job's first task
// is running its command.
const theCorrectionTypedMidJob = "use TypeScript for every file"

// theSecondsTheToolCallHolds is how long the first task's one shell command
// sleeps, which is the window the person's message has to land in.
const theSecondsTheToolCallHolds = 4

// howLongTheToolCallHolds is that window as a length of time.
const howLongTheToolCallHolds = theSecondsTheToolCallHolds * time.Second

// howLongAStopMidJobMayTake is what the person is promised: a stop typed while
// a job's task runs is heard as soon as the tool call in flight has finished,
// not after the job has run every task it has.
const howLongAStopMidJobMayTake = howLongTheToolCallHolds + 8*time.Second

// theStepsThatMakeTheJob are the three model calls of the person's own task:
// make the job with its first task, add the second task and close the task's
// own done list, and answer.
func theStepsThatMakeTheJob() []testkit.Step {
	return []testkit.Step{
		{
			Expect: []string{theAskForTheJobToSteer},
			Text:   "This needs two sittings, so I will make a job.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-job", Name: contract.ToolJob, Input: json.RawMessage(
				`{"action":"create","ask":"` + theAskForTheJobToSteer + `","why":"the user wants the campaign run and summed up","text":"post the tweet"}`)}},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Expect: []string{"created job " + theJobToSteer},
			Text:   "The job made its first task; I will add the second.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-task-two", Name: contract.ToolJob, Input: json.RawMessage(
					`{"action":"add_task","job_id":"` + theJobToSteer + `","text":"write the summary for the user"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants the campaign run and summed up",` +
						`"done_when":[{"text":"the job is made with one task per piece of work","done":true,"result":"reply"}]}`)},
			},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 30},
		},
		{
			Text:   "I made job " + theJobToSteer + " with two tasks. It runs them one at a time.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
	}
}

// theStepThatStartsTheFirstTask is the first task's first model call: one shell
// command that takes a while, which is the tool call in flight when the person
// types, and the task's done list beside it.
func theStepThatStartsTheFirstTask() testkit.Step {
	return testkit.Step{
		Expect: []string{"post the tweet"},
		Text:   "I will post it.",
		Finish: contract.FinishToolCalls,
		ToolCalls: []contract.ToolCall{
			{ID: "call-shell", Name: contract.ToolShell, Input: json.RawMessage(
				fmt.Sprintf(`{"command":"sleep %d"}`, theSecondsTheToolCallHolds))},
			{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
				`{"why":"the user wants the tweet posted","doneWhen":["the tweet is posted"]}`)},
		},
		Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
	}
}

// theReviewStep answers the four review questions, which a task that had a
// correction and a job that has run every task are both asked.
func theReviewStep() testkit.Step {
	return testkit.Step{
		Text:   "1. Two tasks were to run.\n2. They ran.\n3. There was no difference.\n4. Keep one task per piece of work.",
		Finish: contract.FinishEnd,
		Usage:  contract.Usage{InputTokens: 700, OutputTokens: 30},
	}
}

// aJobWhoseFirstTaskHearsACorrection is the whole job: the person's task that
// makes it, the first task hearing the correction in the middle of its turn and
// finishing, the second task, and the job's review. The call after the shell
// command insists on the person's words, so a task that never heard them fails
// there rather than finishing as if nothing had been said.
func aJobWhoseFirstTaskHearsACorrection(_ string) testkit.Script {
	steps := append(theStepsThatMakeTheJob(),
		theStepThatStartsTheFirstTask(),
		testkit.Step{
			Expect: []string{theCorrectionTypedMidJob},
			Text:   "The person wants TypeScript; I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-proof", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the tweet is posted","done":true,"resultId":"r1"}]}`)}},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		testkit.Step{
			Text:   "The tweet is posted. What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
		theReviewStep(),
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

// aJobWhoseFirstTaskIsToldToStop is the same job up to the first task's shell
// command, and nothing after it: a stopped task is put down and its job pauses,
// as jobstop_test.go proves, so no model call follows the stop.
func aJobWhoseFirstTaskIsToldToStop(_ string) testkit.Script {
	steps := append(theStepsThatMakeTheJob(), theStepThatStartsTheFirstTask())
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: steps}
}

func TestAMessageTypedWhileAJobsTaskRunsReachesThatTaskAsACorrection(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobWhoseFirstTaskHearsACorrection)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskForTheJobToSteer})
	running := waitForTheFirstTasksCommand(t, screen)
	number := running.Fields[contract.StatusFieldTask]

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theCorrectionTypedMidJob})

	// The next reply is the first task's report into the job. A task that heard
	// the person is counted done; one that did not fails its next model call,
	// because the script insists on the words.
	report := screen.waitFor(t, contract.SocketReply, 60*time.Second)
	if !strings.Contains(report.Text, "1 of 2 tasks done") {
		t.Errorf("the first task reported:\n%s\nwant it finished with the person's words heard, and the job at 1 of 2 tasks done", report.Text)
	}
	screen.waitForReplySaying(t, "is finished", 90*time.Second)

	written := theReplyToTheCommand(t, screen, "tasks "+number)
	if !strings.Contains(written, `C1 "`+theCorrectionTypedMidJob+`"`) {
		t.Errorf("the record of task %s reads:\n%s\nwant the person's words under Corrections as C1", number, written)
	}
	listed := theReplyToTheCommand(t, screen, "tasks")
	if strings.Contains(listed, theCorrectionTypedMidJob) {
		t.Errorf("the task list reads:\n%s\nand a task of its own was started for the words typed while the job's task ran", listed)
	}
}

func TestAStopTypedWhileAJobsTaskRunsIsHeardWithinTheTurn(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobWhoseFirstTaskIsToldToStop)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskForTheJobToSteer})
	waitForTheFirstTasksCommand(t, screen)

	asked := time.Now()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "stop"})

	reply := screen.waitFor(t, contract.SocketReply, 60*time.Second)
	if !strings.Contains(reply.Text, "I stopped this task") {
		t.Errorf("the first reply after the stop was:\n%s\nwant the first task's stopped report, because the stop was typed while it ran", reply.Text)
	}
	if waited := time.Since(asked); waited > howLongAStopMidJobMayTake {
		t.Errorf("the stop was heard after %s, and it must be heard as soon as the tool call in flight has finished, inside %s",
			waited.Round(time.Millisecond), howLongAStopMidJobMayTake)
	}
}

// waitForTheFirstTasksCommand reads statuses until the job's first task is
// running its shell command, which is the window a message from the person
// lands in, and returns that status. The task's number is on it, because the
// record is made before the command runs.
func waitForTheFirstTasksCommand(t *testing.T, screen *attachedScreen) contract.SocketEnvelope {
	t.Helper()
	// The job driver notices a job made while it waits within the minute its
	// timer is clamped to, so the first task may be a minute in coming.
	return screen.waitForStatusWhere(t, 120*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobToSteer &&
			fields[contract.StatusFieldJobTask] == "t1" &&
			fields[contract.StatusFieldTask] != "" &&
			strings.Contains(fields[contract.StatusFieldToolLine], contract.ToolShell)
	})
}

// theReplyToTheCommand sends one command the way a screen does and returns
// what it answered.
func theReplyToTheCommand(t *testing.T, screen *attachedScreen, command string) string {
	t.Helper()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: command})
	return screen.waitFor(t, contract.SocketReply, 30*time.Second).Text
}
