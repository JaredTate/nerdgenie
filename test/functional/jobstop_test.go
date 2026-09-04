// The whole-program test for stopping a job's task. The person's own words for
// what they need: when they interrupt work, it must "stay on the job and
// task". Escape or /stop on the task a job is running puts that task down
// rather than finishing it. The task is not marked done and not counted as a
// failure, the job stays on it and runs nothing on its own, and the one word
// "continue" picks the same task up under the same job. When the task then
// finishes, its report goes into the job and the next task starts as usual.
package functional

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// howLongTheFirstTaskWorks is how long the job's first task is busy in a tool,
// which is the window the stop lands in. It is a shell sleep rather than a
// stalled model call, so that the task has a record with a result in it when
// it is stopped, and "continue" has something to pick up rather than starting
// the task afresh.
const howLongTheFirstTaskWorks = 5 * time.Second

// theWordThatCarriesOn is what the person types to pick the stopped task up.
const theWordThatCarriesOn = "continue"

// theTaskNumberInARecordLine reads the task's number out of a record line such
// as "job 2 task 4 started · post the tweet".
var theTaskNumberInARecordLine = regexp.MustCompile(`^job ` + theJobTheModelMakes + ` task (\d+) `)

// aJobWhoseFirstTaskIsStopped is the job of jobpanel_test.go made the same
// way, then its first task busy in a tool when the stop lands, the same task
// picked up again by "continue" and finished in one call, the second task, and
// the job's review.
func aJobWhoseFirstTaskIsStopped(_ string) testkit.Script {
	making := aJobOfTwoTasksMadeByTheModel("").Steps[:3]
	seconds := strconv.Itoa(int(howLongTheFirstTaskWorks / time.Second))
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: slices.Concat(making, []testkit.Step{
		{
			Expect: []string{"post the tweet"},
			Text:   "I will give the site a moment before posting.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-wait", Name: contract.ToolShell, Input: json.RawMessage(
				`{"command":"sleep ` + seconds + `"}`)}},
			Usage: contract.Usage{InputTokens: 300, OutputTokens: 10},
		},
		{
			Expect: []string{theWordThatCarriesOn, "post the tweet"},
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
			Text:   "1. Two tasks were to run.\n2. The first was stopped and picked up, then both ran.\n3. The stop.\n4. Keep a stopped task where it was.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 700, OutputTokens: 30},
		},
	})}
}

func TestAStopOnAJobsTaskKeepsTheJobOnThatTaskAndContinuePicksItUp(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobWhoseFirstTaskIsStopped)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatIsAJob})

	// A job made wakes the job driver, so the first task starts within a few
	// seconds, and the stop lands while that task is busy in its tool.
	first := screen.waitForStatusWhere(t, theTimeAJobsFirstTaskIsGiven, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobTheModelMakes &&
			fields[contract.StatusFieldJobTask] == "t1" &&
			strings.Contains(fields[contract.StatusFieldToolLine], contract.ToolShell)
	})
	number := theTaskNumberOf(t, first.Fields[contract.StatusFieldRecordLine])

	// The screen's Escape key sends exactly this, as stop_test.go proves.
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "stop"})
	stopped := screen.waitForReplySaying(t, "I stopped this task", howLongTheFirstTaskWorks+30*time.Second)

	if !strings.Contains(stopped.Text, "Job "+theJobTheModelMakes) || !strings.Contains(stopped.Text, "t1") {
		t.Errorf("the stopped report is %q, want it to say which job and which task it stays on", stopped.Text)
	}
	if strings.Contains(stopped.Text, "tasks done") || strings.Contains(stopped.Text, "report j") {
		t.Errorf("the stopped report is %q, and a stopped task is not finished, so it has no report and no progress line", stopped.Text)
	}
	theJobStaysOnTheStoppedTask(t, screen, number)

	// The one word picks the same task up under the same job, with the same
	// number, and the job's list is still where it was.
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theWordThatCarriesOn})
	again := screen.waitForStatusWhere(t, 60*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobTheModelMakes && fields[contract.StatusFieldJobTask] == "t1"
	})
	if picked := theTaskNumberOf(t, again.Fields[contract.StatusFieldRecordLine]); picked != number {
		t.Errorf("continue started task %s of the job, want task %s picked up again", picked, number)
	}
	if listed := again.Fields[contract.StatusFieldJobTasks]; listed != "[ ] t1 post the tweet\n[ ] t2 write the summary for the user" {
		t.Errorf("while the task is picked up the status carries the task list:\n%s\nwant both tasks, neither done", listed)
	}

	// When it finishes, its report goes into the job and the next task starts.
	screen.waitForReplySaying(t, "Job "+theJobTheModelMakes+", report j"+theJobTheModelMakes+".1: 1 of 2 tasks done", 60*time.Second)
	second := screen.waitForStatusWhere(t, 90*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobTheModelMakes && fields[contract.StatusFieldJobTask] == "t2"
	})
	if listed := second.Fields[contract.StatusFieldJobTasks]; !strings.HasPrefix(listed, "[x] t1 post the tweet\n[ ] t2 ") {
		t.Errorf("while the second task runs the status carries the task list:\n%s\nwant the first task marked done", listed)
	}
	screen.waitForReplySaying(t, theWordsOfAFinishedJob, 90*time.Second)
}

// theJobStaysOnTheStoppedTask asks the job store, through the two listing
// commands, where the job stands after the stop: still listed, none of its
// tasks done, not switched off, no failure and no report written for the task
// that was only stopped, and nothing of it started on its own. The number is
// the stopped task's own, so that its own start can be told from a new one.
func theJobStaysOnTheStoppedTask(t *testing.T, screen *attachedScreen, number string) {
	t.Helper()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "/jobs"})
	listing := waitForTheReplyWhileNoJobTaskStarts(t, screen, number, "Jobs, oldest first", 30*time.Second)
	line := theLineAbout(t, listing.Text, theJobTheModelMakes)
	if !strings.Contains(line, "0 of 2 tasks done") {
		t.Errorf("after the stop the job lists as %q, want none of its two tasks done", line)
	}
	if strings.Contains(line, string(contract.JobOff)) || strings.Contains(line, "failed") {
		t.Errorf("after the stop the job lists as %q, and a stop is neither a failure nor the end of the job", line)
	}
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "/jobs " + theJobTheModelMakes})
	held := waitForTheReplyWhileNoJobTaskStarts(t, screen, number, "# job "+theJobTheModelMakes, 30*time.Second)
	if !strings.Contains(held.Text, "[ ] t1 post the tweet") {
		t.Errorf("the job's record reads:\n%s\nwant its first task still there and not done", held.Text)
	}
	if strings.Contains(held.Text, "Failures:") || strings.Contains(held.Text, "j"+theJobTheModelMakes+".1") {
		t.Errorf("the job's record reads:\n%s\nwant no failure and no report written for a task that was only stopped", held.Text)
	}
}

// theTaskNumberOf reads the number of the job's task out of a record line, and
// fails the test when the line is not about a task of the job.
func theTaskNumberOf(t *testing.T, recordLine string) string {
	t.Helper()
	found := theTaskNumberInARecordLine.FindStringSubmatch(recordLine)
	if found == nil {
		t.Fatalf("the record line is %q, want one about a task of job %s", recordLine, theJobTheModelMakes)
	}
	return found[1]
}

// waitForTheReplyWhileNoJobTaskStarts reads envelopes until the reply with the
// words in it arrives, and fails the test if on the way a status says a job's
// task started under a number other than the stopped task's own, or a reply
// says a job's task reported or failed, because nothing of a stopped job runs
// on its own. A status built while the stopped task was still running, which
// names its own start, may still be on its way, and is not a new start.
func waitForTheReplyWhileNoJobTaskStarts(t *testing.T, screen *attachedScreen, number string, words string, bound time.Duration) contract.SocketEnvelope {
	t.Helper()
	itsOwnStart := "job " + theJobTheModelMakes + " task " + number + " "
	return screen.waitUntil(t, bound, "a reply saying "+words, func(envelope contract.SocketEnvelope) bool {
		line := envelope.Fields[contract.StatusFieldRecordLine]
		if envelope.Type == contract.SocketStatus && strings.HasPrefix(line, "job ") &&
			strings.Contains(line, " started") && !strings.HasPrefix(line, itsOwnStart) {
			t.Fatalf("the status says %q after the stop, and nothing of a stopped job runs until the person carries it on", line)
		}
		if envelope.Type == contract.SocketReply && (strings.Contains(envelope.Text, "failed in a row") || strings.Contains(envelope.Text, "report j")) {
			t.Fatalf("the agent said %q after the stop, and a stopped task neither reports nor fails", envelope.Text)
		}
		return envelope.Type == contract.SocketReply && strings.Contains(envelope.Text, words)
	})
}

// theLineAbout is the line of a jobs listing about one job, which begins with
// the job's number in the first column.
func theLineAbout(t *testing.T, listing string, jobID string) string {
	t.Helper()
	for _, line := range strings.Split(listing, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), jobID+" ") {
			return line
		}
	}
	t.Fatalf("the listing does not mention job %s:\n%s", jobID, listing)
	return ""
}
