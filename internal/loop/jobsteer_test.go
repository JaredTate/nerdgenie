// The tests for what a person's words do when they pick a job's stopped task
// up: a store error on the way leaves the task put down for the next word,
// and a message that begins with the word and goes on, such as "continue, but
// post at noon", picks the task up and steers it.
package loop_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// jobWhoseResumeFailsOnce is the fake job store with one store error in it:
// the first time it is asked to set a job running again, it cannot.
type jobWhoseResumeFailsOnce struct {
	contract.Job
	guard  sync.Mutex
	failed bool
}

// Resume fails once and then passes the call on.
func (jobs *jobWhoseResumeFailsOnce) Resume(ctx context.Context, jobID string) error {
	jobs.guard.Lock()
	first := !jobs.failed
	jobs.failed = true
	jobs.guard.Unlock()
	if first {
		return errors.New("the job store cannot write right now, so try again")
	}
	return jobs.Job.Resume(ctx, jobID)
}

// TestAStoreErrorOnContinueLeavesTheTaskPutDownForTheNextContinue is the
// review's second finding: the loop used to forget the put-down task before
// the store had set the job running, so a store error on "continue" lost the
// job's task for good, and the next "continue" ran it as a plain task. The
// mark lives in the store now and is forgotten only when the store itself
// sets the job running, so the next "continue" picks the same task up under
// the job.
func TestAStoreErrorOnContinueLeavesTheTaskPutDownForTheNextContinue(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	}, scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	flaky := &jobWhoseResumeFailsOnce{Job: built.jobs}
	waiting := aModelThatWaitsOn(2, built.model)
	options := built.optionsOver(waiting)
	options.Jobs = flaky
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the flaky store: %v", err)
	}
	stopTheJobsTask(t, made, built, waiting)

	if _, err := made.Run(t.Context(), built.task("continue")); err == nil {
		t.Fatal("the first continue was answered with no error, and the store refused to set the job running")
	}
	outcome, err := made.Run(t.Context(), built.task("continue"))
	if err != nil {
		t.Fatalf("the second continue failed: %v", err)
	}

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("the second continue ended as %+v, want task 1, the job's put-down task, picked up under the job and finished", outcome)
	}
	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the picked-up task's report in the job", built.channel.Sent())
	}
}

// TestAMessageThatBeginsWithContinuePicksThePutDownTaskUpAndCarriesTheRest
// is the review's fourth finding: a person who says "continue, but post at
// noon" is carrying the task on and steering it, not starting a new one. The
// task is picked up under the job, and the whole message is written into its
// record as a correction, so the steer survives however long the task runs.
// TestAnyMessageAfterAStopPicksThePutDownTaskUpAndSteersIt holds the job half
// of the live game build's second failure: the person's plain words after a
// stop, "the start game button wont start game", pick the put-down task up
// under its job and go into its record as a correction, where they used to
// start a fresh task with an empty record.
func TestAnyMessageAfterAStopPicksThePutDownTaskUpAndSteersIt(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The start button is wired now."),
		aReviewReply("Check the script path first."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	})
	stopTheJobsTask(t, made, built, waiting)

	outcome, err := made.Run(t.Context(), built.task("the start game button wont start game"))
	if err != nil {
		t.Fatalf("carrying the task on with plain words failed: %v", err)
	}
	runTheJobToTheEnd(t, made, built.channel)

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("the plain words after a stop ended as %+v, want task 1, the put-down task, picked up under the job and finished", outcome)
	}
	held := built.held(t, "1")
	if len(held.Rules.Corrections) != 1 || held.Rules.Corrections[0].Text != "the start game button wont start game" {
		t.Errorf("the record's corrections are %+v, want the person's words, so that they outlive the conversation", held.Rules.Corrections)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the job's task reads %+v, want it done under the job", first)
	}
}

func TestAMessageThatBeginsWithContinuePicksThePutDownTaskUpAndCarriesTheRest(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up, at noon."),
		aReviewReply("Post at noon."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	})
	stopTheJobsTask(t, made, built, waiting)

	outcome, err := made.Run(t.Context(), built.task("continue, but post at noon"))
	if err != nil {
		t.Fatalf("carrying the task on with a steer failed: %v", err)
	}
	runTheJobToTheEnd(t, made, built.channel)

	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("the steered continue ended as %+v, want task 1, the put-down task, picked up under the job and finished", outcome)
	}
	held := built.held(t, "1")
	if len(held.Rules.Corrections) != 1 || held.Rules.Corrections[0].Text != "continue, but post at noon" {
		t.Errorf("the record's corrections are %+v, want the person's whole message, word for word", held.Rules.Corrections)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "post at noon") {
		t.Error("the steer never reached the model")
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the job's task reads %+v, want it done under the job", first)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone {
		t.Errorf("the job is %q, want it done after both tasks ran", summary.State)
	}
}
