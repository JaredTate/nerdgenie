// The tests for a job setting a task aside after its second guard stop. On
// the night of 7 September 2026 the flight simulator's sky task stopped on
// the guard, the job picked it up once, it stopped again, and the job waited
// nine hours for a person to type "keep going", three times, while twelve
// tasks that needed nothing from the sky sat untouched. A stuck task now
// goes to the back of the line, and the job goes on.
package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aJobOfThreeTasks puts a job with three tasks into the fake job store: the
// cockpit, the terrain, and the sky and atmosphere, which is the fixture the
// deferral and the pace are proved on.
func aJobOfThreeTasks(t *testing.T, built *harness) string {
	t.Helper()
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{
		Ask: "build the flight simulator", Why: "the person wants to fly over their own town",
	})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	for _, text := range []string{"the cockpit", "the terrain", "the sky and atmosphere"} {
		if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatalf("cannot add a task to job %s: %v", jobID, err)
		}
	}
	return jobID
}

// theFirstLineWith is the index of the first line carrying the words, or -1.
func theFirstLineWith(lines []string, words string) int {
	for at, line := range lines {
		if strings.Contains(line, words) {
			return at
		}
	}
	return -1
}

// twoGuardStopsOnTheFirstTask is the script of the job's first task stopping
// on the guard, being picked up once by the job, and stopping the same way
// again, each stop with its review and the architecture question, together
// with what the read tool answers across both.
func twoGuardStopsOnTheFirstTask() ([]testkit.Step, []string, int) {
	first, answers, calls := stallsThatStopTheGuard(0)
	second, more, _ := stallsThatStopTheGuard(calls)
	steps := append(first, aReviewReply("Read a file once and move on."), answerStep("none"))
	steps = append(steps, second...)
	steps = append(steps, aReviewReply("Read a file once and move on."), answerStep("none"))
	return steps, append(answers, more...), 2 * calls
}

// TestASecondGuardStopSetsTheTaskAsideAndTheJobGoesOn: the guard stops the
// job's first task, the job picks it up once, and the guard stops it again.
// Instead of the job waiting for a person, the task is set aside, the person
// is told so in one line, its record is kept the way a put-down task's is,
// and the job goes on with its next task; once the other tasks are done the
// set-aside task comes back, afresh, and the job closes on all three.
func TestASecondGuardStopSetsTheTaskAsideAndTheJobGoesOn(t *testing.T) {
	steps, reads, _ := twoGuardStopsOnTheFirstTask()
	for _, done := range []string{"the terrain is drawn", "the sky is drawn", "the cockpit is built"} {
		steps = append(steps, closingScript(done)...)
		steps = append(steps, answerStep("none"))
		reads = append(reads, "the notes")
	}
	steps = append(steps, aReviewReply("Set a stuck task aside and come back to it."))
	built := newHarness(t, steps, scriptedTool("read", reads...))
	jobID := aJobOfThreeTasks(t, built)

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 4 {
		t.Errorf("the driver ran %d tasks, want 4: the cockpit twice stopped and set aside, the terrain, the sky, and the cockpit again", ran)
	}
	sent := built.channel.Sent()
	setAside := "Task t1 is set aside after stopping twice; job " + jobID + " goes on with the next task and comes back to t1 before its last task"
	if count := timesSent(sent, setAside); count != 1 {
		t.Errorf("the person was told the task is set aside %d times, want once; they were sent %v", count, sent)
	}
	if sentSomethingLike(sent, "Your next message picks this task up") {
		t.Errorf("the person was sent %v, and a task set aside does not wait for their message", sent)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it done with no failure counted: a task set aside is neither failed nor put down", summary)
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store holds a put-down task, and the job set its task aside instead")
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done || task.ReportID == "" {
			t.Errorf("the task %s reads %+v, want it done and pointing at its report", task.TaskID, task)
		}
	}
	lines := built.recordLines()
	stopped := theFirstLineWith(lines, "job "+jobID+" task 1 stopped")
	terrain := theFirstLineWith(lines, "job "+jobID+" task 2 started · the terrain")
	skyDone := theFirstLineWith(lines, "job "+jobID+" task 3 done")
	cockpitAgain := theFirstLineWith(lines, "job "+jobID+" task 4 started · the cockpit")
	if stopped < 0 || terrain < stopped || skyDone < terrain || cockpitAgain < skyDone {
		t.Errorf("the record lines are %v, want the cockpit stopped, then the terrain and the sky run, then the cockpit started again last", lines)
	}
	if standing := built.held(t, "1").Header.Status; standing != contract.StatusStopped {
		t.Errorf("the set-aside run's record stands at %q, want %q: it is kept the way a put-down task's is", standing, contract.StatusStopped)
	}
}

// TestATaskSetAsideTwiceWaitsForThePerson: the deferral is once per task.
// A task that comes back from the back of the line and stops on the guard
// again is put down for a person, the job paused on it with the mark, the
// way a second stop was before.
func TestATaskSetAsideTwiceWaitsForThePerson(t *testing.T) {
	steps, reads, calls := twoGuardStopsOnTheFirstTask()
	for _, done := range []string{"the terrain is drawn", "the sky is drawn"} {
		steps = append(steps, closingScript(done)...)
		steps = append(steps, answerStep("none"))
		reads = append(reads, "the notes")
	}
	third, evenMore, _ := stallsThatStopTheGuard(calls)
	steps = append(steps, third...)
	steps = append(steps, aReviewReply("Read a file once and move on."), answerStep("none"), answerStep("This reply is never played."))
	built := newHarness(t, steps, scriptedTool("read", append(reads, evenMore...)...))
	jobID := aJobOfThreeTasks(t, built)

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 4 {
		t.Errorf("the driver ran %d tasks, want 4: the job is paused once the set-aside task stops again", ran)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobPaused || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it paused on the task with no failure counted", summary)
	}
	mark, there, err := built.jobs.PutDownTask(t.Context())
	if err != nil || !there || mark.Task.TaskID != "t1" || mark.Run != "4" || !mark.HasRecord || mark.Waiting {
		t.Errorf("the store holds the put-down task as %+v (there %v, error %v), want task t1, run 4, with its record, stopped", mark, there, err)
	}
	sent := built.channel.Sent()
	if count := timesSent(sent, "is set aside after stopping twice"); count != 1 {
		t.Errorf("the job said it set the task aside %d times, want once: the deferral is tried once", count)
	}
	if !sentSomethingLike(sent, "Your next message picks this task up") {
		t.Errorf("the person was sent %v, want the job paused on the task for their message after the third stop", sent)
	}
	if built.model.StepsLeft() != 1 {
		t.Errorf("the model has %d steps left, want 1: nothing runs after the job is paused", built.model.StepsLeft())
	}
	if first := theJobsFirstTask(t, built, jobID); first.Done || first.ReportID != "" {
		t.Errorf("the task reads %+v, want it neither done nor reported", first)
	}
	if _, count := requestsCarrying(built, loop.TheJobPickUpLine); count == 0 {
		t.Error("the job never picked the task up itself before setting it aside")
	}
}
