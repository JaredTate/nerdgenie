// The tests for a job picking up a task the harness's own guard stopped. Run
// ten's polish task was ended by the same-call guard at round 212, the job
// went to waiting, and nothing moved until a person typed continue, hours
// later; the same happened to the GLM run. The word a person types starts a
// fresh window on the same record, and a job can do that much itself, once.
package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/orientation"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// stallsThatStopTheGuard is the script of enough runs of the same call to
// make the same-call guard end the task, RewindsAllowed rethinks and one stall
// more, each stall on a file of its own because a rethink closes the call it
// was made over, with the answers the read tool gives across them, and the
// number of model calls the script takes. The call ids start after the number
// given so that two such scripts in a row do not share an id.
func stallsThatStopTheGuard(after int) ([]testkit.Step, []string, int) {
	steps := []testkit.Step{}
	answers := []string{}
	for stall := 0; stall <= loop.RewindsAllowed; stall++ {
		path := fmt.Sprintf("notes%d.md", stall)
		for call := 1; call <= 4; call++ {
			steps = append(steps, sameReadOf(fmt.Sprintf("c%d", after+stall*4+call), path))
		}
		answers = append(answers, "the notes", "the notes")
		if stall < loop.RewindsAllowed {
			steps = append(steps, theUsualRethink())
		}
	}
	return steps, answers, len(steps)
}

// TestAJobPicksAGuardStoppedTaskUpOnceItselfOnAFreshWindow: the guard stops
// the job's first task, and instead of the job waiting for a person the loop
// picks the task up at once, the way the person's word would, on a fresh
// window with the record's newest results in front of the model; the task
// then finishes, its report goes into the job, and the job runs on.
func TestAJobPicksAGuardStoppedTaskUpOnceItselfOnAFreshWindow(t *testing.T) {
	stalls, answers, calls := stallsThatStopTheGuard(0)
	steps := append(stalls,
		aReviewReply("Read a file once and move on."),
		answerStep("none"), // the fifth question, which a job's task is asked
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep posting at the same hour every day."))
	built := newHarness(t, steps, scriptedTool("read", answers...))
	jobID := aJobOfTwoTasks(t, built)

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 2 {
		t.Errorf("the driver ran %d tasks, want 2: the picked-up first task counts once, on the same turn", ran)
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done || task.ReportID == "" {
			t.Errorf("the task %s reads %+v, want it done and pointing at its report", task.TaskID, task)
		}
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it done with no failure counted: a guard stop picked up is not a failure", summary)
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store holds a put-down task, and the job picked its task up itself")
	}
	lines := built.recordLines()
	if !sentSomethingLike(lines, "job "+jobID+" task 1 stopped") {
		t.Errorf("the record lines are %v, want the first task stopped by the guard", lines)
	}
	if !sentSomethingLike(lines, "job "+jobID+" task 1 started · "+loop.TheJobPickUpLine[:40]) {
		t.Errorf("the record lines are %v, want task 1 picked up again as the job's task with the harness's own words", lines)
	}
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, "I stopped this task") || !sentSomethingLike(sent, "picks task t1 up itself") {
		t.Errorf("the person was sent %v, want the stopped report with the line saying the job picks the task up itself", sent)
	}
	if sentSomethingLike(sent, "Your next message picks this task up") {
		t.Errorf("the person was sent %v, and a task the job picks up itself does not wait for their message", sent)
	}
	first, _ := requestsCarrying(built, loop.TheJobPickUpLine)
	// The stop's review is two calls, the four questions and the fifth, so
	// the pick-up's first request is the third after the stalled calls.
	if first != calls+2 {
		t.Fatalf("the pick-up's first request is number %d, want %d: right after the stalled calls and the stop's review", first+1, calls+3)
	}
	opening := wholeRequestText(built.model.Requests()[first])
	if !strings.Contains(opening, orientation.TheResultsHeading) || !strings.Contains(opening, "the notes") {
		t.Errorf("the pick-up does not open with the record's newest results in full:\n%s", opening)
	}
	if strings.Contains(opening, "Do something different") {
		t.Errorf("the pick-up's window still carries the stalled rounds, want a fresh one:\n%s", opening)
	}
	// Run 23's pick-up went straight back to the polish that stalled it. The
	// fresh window's ask names the cause in the guard's own words and asks
	// for what is provable to be finished.
	if !strings.Contains(opening, "the model asked for the same thing over and over") || !strings.Contains(opening, loop.TheFinishWhatIsProvableLine) {
		t.Errorf("the pick-up's ask does not name the guard's cause and ask to finish what is provable:\n%s", opening)
	}
}

// TestAJobPutsATaskDownWhenItsOwnPickUpStopsOnTheGuardAgain: the pick-up is
// tried once. A task that stalls the same way on its fresh window is put down
// as before, the job paused on it with the mark, for a person to pick up.
func TestAJobPutsATaskDownWhenItsOwnPickUpStopsOnTheGuardAgain(t *testing.T) {
	first, answers, calls := stallsThatStopTheGuard(0)
	second, more, _ := stallsThatStopTheGuard(calls)
	steps := append(first, aReviewReply("Read a file once and move on."), answerStep("none"))
	steps = append(steps, second...)
	steps = append(steps, aReviewReply("Read a file once and move on."), answerStep("none"), answerStep("This reply is never played."))
	built := newHarness(t, steps, scriptedTool("read", append(answers, more...)...))
	jobID := aJobOfTwoTasks(t, built)

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 1 {
		t.Errorf("the driver ran %d tasks, want 1: the job is paused after the pick-up stopped again", ran)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobPaused || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it paused on the task with no failure counted", summary)
	}
	mark, there, err := built.jobs.PutDownTask(t.Context())
	if err != nil || !there || mark.Task.TaskID != "t1" || mark.Run != "1" || !mark.HasRecord || mark.Waiting {
		t.Errorf("the store holds the put-down task as %+v (there %v, error %v), want task t1, run 1, with its record, stopped", mark, there, err)
	}
	if task := theJobsFirstTask(t, built, jobID); task.Done || task.ReportID != "" {
		t.Errorf("the task reads %+v, want it neither done nor reported", task)
	}
	sent := built.channel.Sent()
	if count := timesSent(sent, "picks task t1 up itself"); count != 1 {
		t.Errorf("the job said it picks the task up itself %d times, want once: the pick-up is tried once", count)
	}
	if !sentSomethingLike(sent, "Your next message picks this task up") {
		t.Errorf("the person was sent %v, want the job paused on the task for their message after the second stop", sent)
	}
	if built.model.StepsLeft() != 1 {
		t.Errorf("the model has %d steps left, want 1: nothing runs after the second stop", built.model.StepsLeft())
	}
}

// timesSent counts the messages that carry the words.
func timesSent(sent []string, words string) int {
	count := 0
	for _, message := range sent {
		if strings.Contains(message, words) {
			count++
		}
	}
	return count
}

// TestAPersonsStopIsNeverPickedUpByTheJobItself keeps the person's stop what
// it was: a job never picks up a task the person stopped, on its own.
func TestAPersonsStopIsNeverPickedUpByTheJobItself(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("This reply is never played."),
	})

	stopTheJobsTask(t, made, built, waiting)

	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobPaused {
		t.Errorf("after the person's stop the job is %q, want it paused for them", summary.State)
	}
	if calls := waiting.callsMade(); calls != 2 {
		t.Errorf("the model was called %d times, want 2: the job did not pick the task up behind the person's back", calls)
	}
	if sentSomethingLike(built.channel.Sent(), "picks task t1 up itself") {
		t.Errorf("the person was sent %v, and their own stop is theirs to pick up", built.channel.Sent())
	}
}
