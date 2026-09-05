// The tests for a put-down that the job store refuses. The question, or the
// stopped report, used to be sent only after the mark was written, so a store
// that could not write the mark left the person with nothing: the question was
// never sent and never logged, the job stayed running with the task claimed,
// and the program moved on past a question nobody saw. The report goes out
// whatever the store says, with a line saying the mark failed and what to do,
// and the error is still surfaced.
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

// jobWhosePutDownFailsOnce is the fake job store with one store error in it:
// the first time it is asked to put a job down on a task, it cannot.
type jobWhosePutDownFailsOnce struct {
	contract.Job
	guard  sync.Mutex
	failed bool
}

// PutDown fails once and then passes the call on.
func (jobs *jobWhosePutDownFailsOnce) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	jobs.guard.Lock()
	first := !jobs.failed
	jobs.failed = true
	jobs.guard.Unlock()
	if first {
		return errors.New("the job store cannot write right now, so try again")
	}
	return jobs.Job.PutDown(ctx, mark)
}

// aLoopOverAStoreWhosePutDownFails builds a loop over the harness with the
// model given and a job store that refuses the first put-down.
func aLoopOverAStoreWhosePutDownFails(t *testing.T, built *harness, model contract.Model) *loop.Loop {
	t.Helper()
	options := built.optionsOver(model)
	options.Jobs = &jobWhosePutDownFailsOnce{Job: built.jobs}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the store whose put-down fails: %v", err)
	}
	return made
}

// theReportStillWentOut asserts the one report the person was sent carries
// the task's own words and the line saying the job could not be marked, that
// the same report is in the log, and that the job was left as it was.
func theReportStillWentOut(t *testing.T, built *harness, jobID string, words string) {
	t.Helper()
	sent := built.channel.Sent()
	if len(sent) != 1 {
		t.Fatalf("the person was sent %d messages, want the one report: %v", len(sent), sent)
	}
	for _, wanted := range []string{words, "could not mark job " + jobID, "t1"} {
		if !strings.Contains(sent[0], wanted) {
			t.Errorf("the person was sent %q, want it to say %q", sent[0], wanted)
		}
	}
	logged := false
	for _, event := range built.eventsOfKind(t, contract.EventReply) {
		if strings.Contains(string(event.Body), words) {
			logged = true
		}
	}
	if !logged {
		t.Errorf("the log holds no reply saying %q, and a report is written down before it is sent", words)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobRunning || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it left running with no failure counted, because the mark was refused", summary)
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store holds a mark, and the store refused to write one")
	}
}

// TestAQuestionStillReachesThePersonWhenTheJobCannotBeMarked is the review's
// finding on a question: the person is sent the question with a line saying
// the job could not be marked, the report is logged, the error is surfaced,
// and their answer, named for the task the program remembers, still reaches
// the task under its job.
func TestAQuestionStillReachesThePersonWhenTheJobCannotBeMarked(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep(theQuestionTheTaskAsks),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Ask about the folder first."),
	}, scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	made := aLoopOverAStoreWhosePutDownFails(t, built, built.model)

	if _, err := made.RunNextJobTask(t.Context(), built.channel); err == nil {
		t.Fatal("the store refused to mark the job and the loop said nothing was wrong")
	}

	theReportStillWentOut(t, built, jobID, theQuestionTheTaskAsks)
	answer := built.task(theAnswerThePersonGives)
	answer.ResumeID = "1"
	outcome, err := made.Run(t.Context(), answer)
	if err != nil {
		t.Fatalf("answering the question failed: %v", err)
	}
	if outcome.TaskID != "1" || outcome.Status != contract.StatusDone {
		t.Errorf("the answer ended as %+v, want task 1, the job's task, picked up under the job and finished", outcome)
	}
	if first := theJobsFirstTask(t, built, jobID); !first.Done {
		t.Errorf("the job's task reads %+v, want its report in the job once its question was answered", first)
	}
}

// TestAStoppedReportStillReachesThePersonWhenTheJobCannotBeMarked is the same
// for a stop: the stopped report goes out with the line saying the mark
// failed and the word that tries again.
func TestAStoppedReportStillReachesThePersonWhenTheJobCannotBeMarked(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
	}, scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	waiting := aModelThatWaitsOn(2, built.model)
	made := aLoopOverAStoreWhosePutDownFails(t, built, waiting)

	ended := make(chan error, 1)
	go func() {
		_, err := made.RunNextJobTask(context.Background(), built.channel)
		ended <- err
	}()
	<-waiting.calling
	made.Stop()
	if err := <-ended; err == nil {
		t.Fatal("the store refused to mark the job and the loop said nothing was wrong")
	}

	theReportStillWentOut(t, built, jobID, "I stopped this task")
	if sent := built.channel.Sent(); !strings.Contains(sent[0], "Say continue to try again") {
		t.Errorf("the person was sent %q, want it to say what to do: say continue to try again", sent[0])
	}
}
