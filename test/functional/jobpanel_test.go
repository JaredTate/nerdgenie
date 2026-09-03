// The whole-program test for the job a screen is watching: a job the model
// makes runs its tasks one at a time, and while each runs the status carries
// the job's number, its ask, its task list with a mark on every task that is
// done, and which task is running, which is what the side panel draws. The
// Tetris trial expected to see its job there and saw nothing.
package functional

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theAskThatIsAJob is what the person types, and what the job's ask field has
// to carry back.
const theAskThatIsAJob = "run the two-part campaign: post the tweet, then write me the summary"

// theJobTheModelMakes is the number the model's job takes: the nightly
// self-check is registered first on every fresh home, so the model's is the
// second.
const theJobTheModelMakes = "2"

// aJobOfTwoTasksMadeByTheModel is one task that makes the job, then the job's
// two tasks each answered in one call, then the job's review.
func aJobOfTwoTasksMadeByTheModel(_ string) testkit.Script {
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Expect: []string{theAskThatIsAJob},
			Text:   "This needs two sittings, so I will make a job.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-job", Name: contract.ToolJob, Input: json.RawMessage(
				`{"action":"create","ask":"` + theAskThatIsAJob + `","why":"the user wants the campaign run and summed up"}`)}},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Expect: []string{"created job " + theJobTheModelMakes},
			Text:   "I will give the job its two tasks.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-task-one", Name: contract.ToolJob, Input: json.RawMessage(
					`{"action":"add_task","job_id":"` + theJobTheModelMakes + `","text":"post the tweet"}`)},
				{ID: "call-task-two", Name: contract.ToolJob, Input: json.RawMessage(
					`{"action":"add_task","job_id":"` + theJobTheModelMakes + `","text":"write the summary for the user"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants the campaign run and summed up",` +
						`"done_when":[{"text":"the job is made with one task per piece of work","done":true,"result":"reply"}]}`)},
			},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 30},
		},
		{
			Text:   "I made job " + theJobTheModelMakes + " with two tasks. It runs them one at a time.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
		{
			Expect: []string{"post the tweet"},
			Text:   "The tweet is posted.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 5},
		},
		{
			Expect: []string{"write the summary for the user"},
			Text:   "The summary is written.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 5},
		},
		{
			Text:   "1. Two tasks were to run.\n2. They ran.\n3. There was no difference.\n4. Keep one task per piece of work.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 700, OutputTokens: 30},
		},
	}}
}

func TestTheStatusCarriesTheJobAndItsTaskListWhileItsTasksRun(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobOfTwoTasksMadeByTheModel)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatIsAJob})

	first := screen.waitForStatusWhere(t, 90*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobTheModelMakes && fields[contract.StatusFieldJobTask] == "t1"
	})
	if first.Fields[contract.StatusFieldJobAsk] != theAskThatIsAJob {
		t.Errorf("the status carries the job's ask as %q, want the person's own words", first.Fields[contract.StatusFieldJobAsk])
	}
	if listed := first.Fields[contract.StatusFieldJobTasks]; listed != "[ ] t1 post the tweet\n[ ] t2 write the summary for the user" {
		t.Errorf("while the first task runs the status carries the task list:\n%s\nwant both tasks, neither done", listed)
	}

	second := screen.waitForStatusWhere(t, 90*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobTheModelMakes && fields[contract.StatusFieldJobTask] == "t2"
	})
	if listed := second.Fields[contract.StatusFieldJobTasks]; !strings.HasPrefix(listed, "[x] t1 post the tweet\n[ ] t2 ") {
		t.Errorf("while the second task runs the status carries the task list:\n%s\nwant the first task marked done", listed)
	}

	screen.waitForReplySaying(t, "has run every task", 90*time.Second)
	screen.waitForStatusWhere(t, 30*time.Second, func(fields map[string]string) bool {
		_, sent := fields[contract.StatusFieldJob]
		return sent && fields[contract.StatusFieldJob] == ""
	})
}
