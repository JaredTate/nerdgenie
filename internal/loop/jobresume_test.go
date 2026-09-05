// The tests for how the loop sets a put-down job running again when the
// person picks its task up. The word that carries on, and the answer to a
// question, must go through Resume, which sets the job running and nothing
// else: RunNow takes the date off the first unfinished task and fires a
// schedule's tick at once, so a "continue" that went through it erased the
// date on a task dated next week and ran a scheduled job an extra time.
package loop_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// fakeJobThatResumes is the fake job store with Resume on it, which the
// contract has and the fake in internal/testkit does not carry yet, because
// another brief is adding it there; the harness wraps every fake store in it.
// It writes down which of the two ways of setting a job running again the
// loop asked for, because the carry-on must use Resume and never RunNow. Its
// own Resume sets the job running through RunNow, the only way the fake
// offers; that Resume keeps every date and the schedule where they were is
// proved on the real store, in internal/job.
type fakeJobThatResumes struct {
	*testkit.FakeJob
	guard   sync.Mutex
	resumed []string
	ranNow  []string
}

// Resume writes the job down as resumed and sets it running.
func (jobs *fakeJobThatResumes) Resume(ctx context.Context, jobID string) error {
	jobs.guard.Lock()
	jobs.resumed = append(jobs.resumed, jobID)
	jobs.guard.Unlock()
	return jobs.FakeJob.RunNow(ctx, jobID)
}

// RunNow writes the job down as run now and passes the call on.
func (jobs *fakeJobThatResumes) RunNow(ctx context.Context, jobID string) error {
	jobs.guard.Lock()
	jobs.ranNow = append(jobs.ranNow, jobID)
	jobs.guard.Unlock()
	return jobs.FakeJob.RunNow(ctx, jobID)
}

// resumedJobs is every job the loop set running through Resume, in order.
func (jobs *fakeJobThatResumes) resumedJobs() []string {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	return append([]string(nil), jobs.resumed...)
}

// jobsRunNow is every job the loop set running through RunNow, in order.
func (jobs *fakeJobThatResumes) jobsRunNow() []string {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	return append([]string(nil), jobs.ranNow...)
}

// TestContinueSetsTheJobRunningThroughResumeAndNeverRunNow proves the word
// that carries on picks the job's stopped task up through Resume, so that a
// task dated next week keeps its date and a scheduled job fires no extra tick.
func TestContinueSetsTheJobRunningThroughResumeAndNeverRunNow(t *testing.T) {
	built, made, waiting, jobID := aJobWhoseTaskIsStoppedOn(t, 2, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Keep a stopped task where it was."),
	})
	stopTheJobsTask(t, made, built, waiting)

	if _, err := made.Run(t.Context(), built.task("continue")); err != nil {
		t.Fatalf("carrying the task on failed: %v", err)
	}

	if resumed := built.jobs.resumedJobs(); !slices.Equal(resumed, []string{jobID}) {
		t.Errorf("continue resumed the jobs %v, want job %s set running through Resume", resumed, jobID)
	}
	if ranNow := built.jobs.jobsRunNow(); len(ranNow) != 0 {
		t.Errorf("continue ran the jobs %v now, and RunNow erases the date of the next task and fires a schedule's tick", ranNow)
	}
}

// TestAnAnswerSetsTheJobRunningThroughResumeAndNeverRunNow proves the same of
// the answer to a question the job's task asked.
func TestAnAnswerSetsTheJobRunningThroughResumeAndNeverRunNow(t *testing.T) {
	built, jobID := aJobWhoseFirstTaskAsks(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep(theQuestionTheTaskAsks),
		answerStep("The post is up."),
		answerStep("The summary is written."),
		aReviewReply("Ask about the folder first."),
	})
	theJobHoldsOnItsFirstTask(t, built, jobID)

	if _, err := built.loop.Run(t.Context(), built.task(theAnswerThePersonGives)); err != nil {
		t.Fatalf("answering the question failed: %v", err)
	}

	if resumed := built.jobs.resumedJobs(); !slices.Equal(resumed, []string{jobID}) {
		t.Errorf("the answer resumed the jobs %v, want job %s set running through Resume", resumed, jobID)
	}
	if ranNow := built.jobs.jobsRunNow(); len(ranNow) != 0 {
		t.Errorf("the answer ran the jobs %v now, and RunNow erases the date of the next task and fires a schedule's tick", ranNow)
	}
}
