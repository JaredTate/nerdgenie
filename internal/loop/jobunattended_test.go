// The tests for a schedule's task, which nobody attends, when it stops: a
// person's stop puts it down like any job's task, and a stop the harness made
// is the failure it is, because nobody is there to pick it up.
package loop_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// jobThatHandsOutOneUnattendedTask is the fake job store with two changes: the
// one task it hands out is a schedule's, so nobody is there to carry it on,
// and it hands out nothing afterwards, so that the loop's own question after
// the task ends finds nothing to run.
type jobThatHandsOutOneUnattendedTask struct {
	contract.Job
	guard     sync.Mutex
	handedOut bool
	finished  []bool
}

// NextTask hands the first task out once, as a schedule's.
func (jobs *jobThatHandsOutOneUnattendedTask) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	if jobs.handedOut {
		return contract.TaskToRun{}, false, nil
	}
	jobs.handedOut = true
	due, there, err := jobs.Job.NextTask(ctx, now)
	due.Unattended = true
	return due, there, err
}

// FinishTask writes down whether the report was a failure, then passes it on.
func (jobs *jobThatHandsOutOneUnattendedTask) FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	jobs.guard.Lock()
	jobs.finished = append(jobs.finished, failed)
	jobs.guard.Unlock()
	return jobs.Job.FinishTask(ctx, jobID, taskID, report, failed)
}

// TestAPersonsStopPutsAScheduledJobsTaskDownToo is the review's third
// finding. A schedule's task is unattended, which means a schedule made it,
// not that nobody is watching: the person at the terminal who presses Escape
// on it stopped it on purpose, and it used to be written into the job as a
// failure and counted toward the three that pause a job. A person's stop puts
// down any job's task.
func TestAPersonsStopPutsAScheduledJobsTaskDownToo(t *testing.T) {
	built := newHarness(t, nil)
	jobID := aJobOfTwoTasks(t, built)
	unattended := &jobThatHandsOutOneUnattendedTask{Job: built.jobs}
	waiting := aModelThatWaitsOn(1, built.model)
	options := built.optionsOver(waiting)
	options.Jobs = unattended
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the unattended job: %v", err)
	}

	stopTheJobsTask(t, made, built, waiting)

	if len(unattended.finished) != 0 {
		t.Errorf("the job store was told %v, want nothing: a task the person stopped is put down, not finished", unattended.finished)
	}
	if summary := theSummaryOf(t, built, jobID); summary.FailuresInARow != 0 || summary.State != contract.JobPaused {
		t.Errorf("the job reads %+v, want no failure counted and the job paused on the task the person stopped", summary)
	}
	mark, there, err := built.jobs.PutDownTask(t.Context())
	if err != nil || !there || mark.Task.TaskID != "t1" || !mark.Task.Unattended {
		t.Errorf("the store holds the put-down task as %+v (there %v, error %v), want the schedule's task t1", mark, there, err)
	}
	if !sentSomethingLike(built.channel.Sent(), "Your next message picks this task up") {
		t.Errorf("the person was sent %v, want the stopped report with the word that picks the task up", built.channel.Sent())
	}
}

// capsWithRoundsPerTask is the shipped caps with a round budget on every task,
// which is how a test makes the harness itself stop a task.
func capsWithRoundsPerTask(rounds int) contract.Caps {
	caps := contract.DefaultConfig().Caps
	caps.RoundsPerTask = rounds
	return caps
}

// TestAStopTheHarnessMadeOnAScheduledJobsTaskIsTheFailureItIs pins the other
// side of the rule: a schedule's task that the harness stopped, here because
// its budget ran out, has nobody to pick it up, so it is finished as failed,
// and the next tick brings its own task.
func TestAStopTheHarnessMadeOnAScheduledJobsTaskIsTheFailureItIs(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("What I did: I read the notes. What is left: the post."),
	}, scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	unattended := &jobThatHandsOutOneUnattendedTask{Job: built.jobs}
	options := built.options()
	options.Jobs = unattended
	options.Caps = capsWithRoundsPerTask(1)
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the unattended job: %v", err)
	}

	if _, err := made.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the schedule's task: %v", err)
	}

	if len(unattended.finished) != 1 || !unattended.finished[0] {
		t.Errorf("the job store was told %v, want the one task the budget stopped finished as failed", unattended.finished)
	}
	if summary := theSummaryOf(t, built, jobID); summary.FailuresInARow != 1 || summary.State != contract.JobRunning {
		t.Errorf("the job reads %+v, want one failure counted and the job still running for its next tick", summary)
	}
	if !sentSomethingLike(built.channel.Sent(), "0 of 2 tasks done") {
		t.Errorf("the person was sent %v, want the failure report with the job's progress on it", built.channel.Sent())
	}
}

// TestAPickedUpSchedulesTaskIsAttendedSoAnAskMeFirstCallAsks pins what
// picking a schedule's task up means: the person who says continue is
// attending it now, so a call on the ask-me-first list shows them a preview
// rather than stopping the task as it would for a schedule running alone.
func TestAPickedUpSchedulesTaskIsAttendedSoAnAskMeFirstCallAsks(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	}, scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	built.rulings.Rule("read", contract.PermissionDecision{
		Ruling: contract.RulingAsk, Reason: "reading the notes is on the ask-me-first list", PreviewText: "read notes.md",
	})
	unattended := &jobThatHandsOutOneUnattendedTask{Job: built.jobs}
	waiting := aModelThatWaitsOn(1, built.model)
	options := built.optionsOver(waiting)
	options.Jobs = unattended
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the unattended job: %v", err)
	}
	stopTheJobsTask(t, made, built, waiting)

	outcome, err := made.Run(t.Context(), built.task("continue"))
	if err != nil {
		t.Fatalf("carrying the schedule's task on failed: %v", err)
	}

	// The rulebook turns an ask into a stop when the request says nobody is
	// attending, so the flag the loop sends is the whole of the difference.
	for _, asked := range built.rulings.Requests() {
		if asked.ToolName == "read" && asked.Unattended {
			t.Errorf("the loop asked the rulebook about %s as unattended, and the person who picked the task up is attending it", asked.ToolName)
		}
	}
	if previews := built.channel.Previews(); len(previews) != 1 {
		t.Errorf("the person was shown %d previews, want the one for the call on the ask-me-first list: they picked the task up, so they are there to answer", len(previews))
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the picked-up task ended as %+v, want it finished after the person answered the preview", outcome)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the task reads %+v, want it done under the job", first)
	}
	if len(unattended.finished) != 1 || unattended.finished[0] {
		t.Errorf("the job store was told %v, want the one task finished and not failed", unattended.finished)
	}
}
