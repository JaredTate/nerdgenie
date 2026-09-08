// The tests for the pace of a task against its job. The flight simulator's
// sky task ran 443 rounds and ten hours where its neighbours took 84 to 173
// rounds, almost all of it on one done line, and nothing weighed one task
// against the job. A task is now told its cost against the median of the
// job's finished tasks at twice it, and closed honestly at three times.
package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theMedianTaskTime is how long each of the job's first two tasks takes: the
// one read in each moves the fake clock by this much, so the job's median is
// ten minutes and the paced task's minutes read as plain multiples of it.
const theMedianTaskTime = 10 * time.Minute

// aReadWhoseTimeTheTestSets is a read tool that moves the fake clock on by
// whatever span the test has set when it runs, answering with a different
// line each time so that every read is a look at something new. It is how
// one task is made to take twice or three times what its neighbours took
// without any test waiting on a real clock.
type aReadWhoseTimeTheTestSets struct {
	clock *testkit.FakeClock
	by    time.Duration
	reads int
}

// Spec is what the model is told about the read.
func (tool *aReadWhoseTimeTheTestSets) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: "read", Description: "A read that takes the span the test set, for the tests.", Classes: []contract.PermissionClass{contract.ClassRead}}
}

// Run moves the clock on and answers with the number of the read.
func (tool *aReadWhoseTimeTheTestSets) Run(context.Context, json.RawMessage) (contract.ToolOutput, error) {
	tool.reads++
	tool.clock.Advance(tool.by)
	return contract.ToolOutput{Text: fmt.Sprintf("the sky notes, part %d", tool.reads)}, nil
}

// aPacedHarness builds the harness over the clock-moving read and the job of
// three tasks, on one fake clock the loop and the job store share.
func aPacedHarness(t *testing.T, steps []testkit.Step) (*harness, *aReadWhoseTimeTheTestSets, string) {
	t.Helper()
	clock := testkit.NewFakeClock(theStartOfTime)
	tool := &aReadWhoseTimeTheTestSets{clock: clock, by: theMedianTaskTime}
	built := newHarness(t, steps, tool)
	built.clock = clock
	built.jobs = &fakeJobThatResumes{FakeJob: testkit.NewFakeJob(clock)}
	built.loop = mustBuild(t, built)
	return built, tool, aJobOfThreeTasks(t, built)
}

// twoFinishedTasks is the script of the job's first two tasks, each reading
// once and closing, with the architecture question every job task is asked
// at its end answered "none".
func twoFinishedTasks() []testkit.Step {
	steps := []testkit.Step{}
	for _, done := range []string{"the cockpit is built", "the terrain is drawn"} {
		steps = append(steps, closingScript(done)...)
		steps = append(steps, answerStep("none"))
	}
	return steps
}

// readsOfTheSky is the script of a task that opens with a read and its done
// list, reads the given number of further parts, then points the done line
// at the first read and answers.
func readsOfTheSky(doneWhen string, more int) []testkit.Step {
	steps := []testkit.Step{callStep("I will read the sky notes.",
		callFor("c1", "read", `{"path":"sky1.md"}`),
		taskCall("c1t", `{"why":"the sky must be drawn","doneWhen":["`+doneWhen+`"]}`))}
	for read := 2; read <= more+1; read++ {
		steps = append(steps, callStep(fmt.Sprintf("Reading part %d.", read),
			callFor(fmt.Sprintf("c%d", read), "read", fmt.Sprintf(`{"path":"sky%d.md"}`, read))))
	}
	return append(steps,
		callStep("I will point the done line at the result.",
			taskCall("cpt", `{"doneWhen":[{"text":"`+doneWhen+`","done":true,"resultId":"r1"}]}`)),
		answerStep("It is done. What changed: the sky. What I checked: the notes. What is left: nothing."))
}

// runTheNextTask runs one due task of the job and fails the test when there
// is none or it cannot run.
func runTheNextTask(t *testing.T, built *harness) {
	t.Helper()
	more, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil {
		t.Fatalf("the loop could not run the job's task: %v", err)
	}
	if !more {
		t.Fatal("the loop found nothing to run, and the job has a task waiting")
	}
}

// theCostLineAtTwice is what the model reads when a task with one open done
// line has run twenty minutes against a median of ten.
const theCostLineAtTwice = "this task has run 20 minutes against a median of 10 for this job's finished tasks; " +
	"0 of 1 done lines are proved; prove the open lines with what you hold, or write what stands in their way as a failure and close the task"

// TestATaskAtTwiceTheJobsMedianReadsItsCostLineOnce: the job's first two
// tasks take ten minutes each, and the third reads in four-minute parts. The
// round that carries it to twenty minutes, twice the median, puts the cost
// line in front of the model, once; the rounds after it, still under three
// times, read nothing more, and the task closes on its own proof.
func TestATaskAtTwiceTheJobsMedianReadsItsCostLineOnce(t *testing.T) {
	steps := append(twoFinishedTasks(), readsOfTheSky("the sky is drawn", 6)...)
	steps = append(steps, aReviewReply("Keep reading."), answerStep("none"), aReviewReply("Keep the tasks short."))
	built, tool, jobID := aPacedHarness(t, steps)
	runTheNextTask(t, built)
	runTheNextTask(t, built)
	tool.by = 4 * time.Minute

	runTheNextTask(t, built)

	first, _ := requestsCarrying(built, theCostLineAtTwice)
	// The first two tasks made five calls each, so the sky's sixth call,
	// the one after the fifth read moved the clock to twenty minutes, is
	// request fifteen counting from zero.
	if first != 15 {
		t.Errorf("the cost line first reached the model on request %d, want 15, the round after the task passed twice the median", first)
	}
	if count := strings.Count(wholeRequestText(built.model.Requests()[16]), theCostLineAtTwice); count != 1 {
		t.Errorf("the request after the cost line carries it %d times, want once: the line is said once", count)
	}
	if !sentSomethingLike(built.channel.Sent(), "Task t3 took 28m") {
		t.Errorf("the person was sent %v, want the sky task closed on its own proof after twenty-eight minutes", built.channel.Sent())
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone {
		t.Errorf("the job is %q, want it done: a task under three times the median is never closed by the harness", summary.State)
	}
}

// TestATaskAtThriceTheJobsMedianEndsFailedWithItsOpenLinesOnTheReport: the
// third task reads in ten-minute parts. At twenty minutes it reads the cost
// line with one of its two lines proved; at thirty, three times the median,
// the harness ends it as failed with the report naming what was proved and
// the open line's text, the job takes it as a failed task, and the record's
// open line stays open, because the done check is never bypassed.
func TestATaskAtThriceTheJobsMedianEndsFailedWithItsOpenLinesOnTheReport(t *testing.T) {
	steps := append(twoFinishedTasks(),
		callStep("I will read the sky notes.",
			callFor("c1", "read", `{"path":"sky1.md"}`),
			taskCall("c1t", `{"why":"the sky must be drawn","doneWhen":["the sky is drawn","the clouds move"]}`)),
		callStep("I will prove the first line.",
			callFor("c2", "read", `{"path":"sky2.md"}`),
			taskCall("c2t", `{"doneWhen":[{"text":"the sky is drawn","done":true,"resultId":"r1"},{"text":"the clouds move"}]}`)),
		callStep("Reading part 3.", callFor("c3", "read", `{"path":"sky3.md"}`)),
		aReviewReply("Close a task at three times the median."),
		answerStep("none"))
	built, _, jobID := aPacedHarness(t, steps)
	runTheNextTask(t, built)
	runTheNextTask(t, built)

	runTheNextTask(t, built)

	report := "paced out at three times the job's median: 1 of 2 done lines proved; not proved: the clouds move"
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, report) {
		t.Errorf("the person was sent %v, want the paced-out report with the open line on it", sent)
	}
	costLine := "this task has run 20 minutes against a median of 10 for this job's finished tasks; 1 of 2 done lines are proved"
	if first, _ := requestsCarrying(built, costLine); first != 12 {
		t.Errorf("the cost line first reached the model on request %d, want 12, the round after the task passed twice the median", first)
	}
	held, err := built.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 3 || !strings.HasPrefix(held.Work.Results[2].Summary, "failed: ") || !strings.Contains(held.Work.Results[2].Summary, report) {
		t.Errorf("the job's reports are %+v, want the third a failed report carrying the paced-out line", held.Work.Results)
	}
	if summary := theSummaryOf(t, built, jobID); summary.FailuresInARow != 1 || summary.State != contract.JobRunning {
		t.Errorf("the job reads %+v, want one failure counted and the job still running", summary)
	}
	record := built.held(t, "3")
	if record.Header.Status != contract.StatusFailed {
		t.Errorf("the sky task's record stands at %q, want %q", record.Header.Status, contract.StatusFailed)
	}
	if len(record.Goal.DoneWhen) != 2 || record.Goal.DoneWhen[1].Done {
		t.Errorf("the record's done list reads %+v, want the second line still open: the done check is not bypassed", record.Goal.DoneWhen)
	}
	if built.model.StepsLeft() != 0 {
		t.Errorf("the model has %d steps left, want none: the paced-out task is reviewed and asked nothing more", built.model.StepsLeft())
	}
}

// TestAJobWithFewerThanTwoFinishedTasksPacesNothing: with one task finished
// there is no median, so the second task runs four times as long as the
// first and reads no cost line.
func TestAJobWithFewerThanTwoFinishedTasksPacesNothing(t *testing.T) {
	steps := append(closingScript("the cockpit is built"), answerStep("none"))
	steps = append(steps, readsOfTheSky("the terrain is drawn", 3)...)
	steps = append(steps, aReviewReply("Keep reading."), answerStep("none"))
	built, _, jobID := aPacedHarness(t, steps)
	runTheNextTask(t, built)

	runTheNextTask(t, built)

	if _, count := requestsCarrying(built, "against a median of"); count != 0 {
		t.Errorf("the cost line reached the model in %d requests, want none: one finished task makes no median", count)
	}
	if !sentSomethingLike(built.channel.Sent(), "Task t2 took 40m") {
		t.Errorf("the person was sent %v, want the terrain task closed on its own proof after forty minutes", built.channel.Sent())
	}
	if summary := theSummaryOf(t, built, jobID); summary.TasksDone != 2 || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want two tasks done and no failure", summary)
	}
}

// TestAPlainTaskOutsideAJobIsNeverPaced: a job with two finished tasks has a
// median of ten minutes, and a person's own task beside it runs forty and
// reads no cost line, because a plain task has no job to be weighed against.
func TestAPlainTaskOutsideAJobIsNeverPaced(t *testing.T) {
	steps := append(twoFinishedTasks(), readsOfTheSky("the sky notes are read", 3)...)
	steps = append(steps, aReviewReply("Keep reading."))
	built, _, _ := aPacedHarness(t, steps)
	runTheNextTask(t, built)
	runTheNextTask(t, built)

	outcome := built.ask(t, "read every part of the sky notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the plain task ended %q, want done: nothing paces a task outside a job. %s", outcome.Status, outcome.Report)
	}
	if _, count := requestsCarrying(built, "against a median of"); count != 0 {
		t.Errorf("the cost line reached the model in %d requests, want none: a plain task is never paced", count)
	}
	if built.model.StepsLeft() != 0 {
		t.Errorf("the model has %d steps left, want none: the plain task ran every round of its script", built.model.StepsLeft())
	}
}
