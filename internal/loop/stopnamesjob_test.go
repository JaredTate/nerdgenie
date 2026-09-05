// The tests for the stopped report of a person's task that made a job. The
// report used to invite the person to say how to carry on, and the live run
// found what that costs: the task had made job 2, the driver started the
// job's first task the moment the task ended, and the person's "continue"
// went to that running task, so the invitation pointed at work that was over.
// A stopped task that made a job which is now running says so instead.
package loop_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theJobToolsAnswer is what the job tool says when it has made a job with one
// task, which is how the loop learns that the task made a job.
const theJobToolsAnswer = "created job 1 and started task t1\n"

// stopThePersonsTask runs a person's task in the background, stops it the
// moment the waiting model call is in flight, and hands back where it ended.
func stopThePersonsTask(t *testing.T, made *loop.Loop, built *harness, waiting *modelThatWaitsOnOneCall, said string) loop.Outcome {
	t.Helper()
	ended := make(chan loop.Outcome, 1)
	failed := make(chan error, 1)
	go func() {
		outcome, err := made.Run(context.Background(), built.task(said))
		if err != nil {
			failed <- err
			return
		}
		ended <- outcome
	}()
	<-waiting.calling
	made.Stop()
	select {
	case outcome := <-ended:
		return outcome
	case err := <-failed:
		t.Fatalf("the task did not stop cleanly: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the loop never came back after the stop")
	}
	return loop.Outcome{}
}

// TestAStoppedTaskThatMadeARunningJobNamesTheJobInsteadOfInvitingACarryOn
// proves the report: the job and its next task are named, the person is told
// how to stop that too, and nothing invites them to carry the stopped task on.
func TestAStoppedTaskThatMadeARunningJobNamesTheJobInsteadOfInvitingACarryOn(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("This needs two sittings, so I will make a job.",
			callFor("c1", contract.ToolJob, `{"action":"create","ask":"run the campaign","text":"post the tweet"}`)),
	}, scriptedTool(contract.ToolJob, theJobToolsAnswer))
	jobID := aJobOfTwoTasks(t, built)
	waiting := aModelThatWaitsOn(2, built.model)
	made, err := loop.New(built.optionsOver(waiting))
	if err != nil {
		t.Fatalf("cannot build a loop over a model that waits: %v", err)
	}

	outcome := stopThePersonsTask(t, made, built, waiting, "run the two-part campaign")

	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended as %+v, want it stopped", outcome)
	}
	for _, words := range []string{"I stopped this task", "Job " + jobID, "t1", "say stop to stop that too"} {
		if !strings.Contains(outcome.Report, words) {
			t.Errorf("the stopped report is %q, want it to say %q", outcome.Report, words)
		}
	}
	if strings.Contains(strings.ToLower(outcome.Report), "carry on") {
		t.Errorf("the stopped report is %q, and it invites a carry-on of a task whose work is now the job's", outcome.Report)
	}
}

// TestAStoppedTaskThatMadeNoJobStillInvitesACarryOn pins the other side: a
// task with nothing running on its behalf is picked up the way it always was.
func TestAStoppedTaskThatMadeNoJobStillInvitesACarryOn(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
	}, scriptedTool("read", "the notes"))
	waiting := aModelThatWaitsOn(2, built.model)
	made, err := loop.New(built.optionsOver(waiting))
	if err != nil {
		t.Fatalf("cannot build a loop over a model that waits: %v", err)
	}

	outcome := stopThePersonsTask(t, made, built, waiting, "read the notes")

	if !strings.Contains(outcome.Report, "Tell me how to carry on") {
		t.Errorf("the stopped report is %q, want the invitation to carry on, because nothing runs on this task's behalf", outcome.Report)
	}
}

// jobWhoseListCannotBeRead is the fake job store that cannot say where its
// jobs stand, so that the report of a task that made one falls back to the
// plain invitation rather than failing the stop.
type jobWhoseListCannotBeRead struct {
	contract.Job
}

// List always refuses.
func (jobs jobWhoseListCannotBeRead) List(_ context.Context) ([]contract.JobSummary, error) {
	return nil, errors.New("the job store cannot be read right now, so check the database file")
}

// TestAStoppedTaskWhoseJobsCannotBeReadStillStopsWithTheInvitation pins the
// failure path: the stop is not held up by a store that will not answer.
func TestAStoppedTaskWhoseJobsCannotBeReadStillStopsWithTheInvitation(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("This needs two sittings, so I will make a job.",
			callFor("c1", contract.ToolJob, `{"action":"create","ask":"run the campaign","text":"post the tweet"}`)),
	}, scriptedTool(contract.ToolJob, theJobToolsAnswer))
	aJobOfTwoTasks(t, built)
	waiting := aModelThatWaitsOn(2, built.model)
	options := built.optionsOver(waiting)
	options.Jobs = jobWhoseListCannotBeRead{Job: built.jobs}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the store that cannot be read: %v", err)
	}

	outcome := stopThePersonsTask(t, made, built, waiting, "run the two-part campaign")

	if outcome.Status != contract.StatusStopped || !strings.Contains(outcome.Report, "Tell me how to carry on") {
		t.Errorf("the task ended as %+v, want it stopped with the plain invitation when the jobs cannot be read", outcome)
	}
}
