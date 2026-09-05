package job_test

// These tests are the rule that a job's task does not make a job: a create from
// inside one is refused naming the job and add_task, told by the record's ask
// matching an unfinished task of a running job.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/job"
)

// aJobWithTwoTasks puts one running job with two tasks into the store, through
// the store itself, and returns its id.
func aJobWithTwoTasks(t *testing.T, jobs contract.Job, first string, second string) string {
	t.Helper()
	jobID, err := jobs.Create(context.Background(), contract.NewJob{Ask: "run the campaign", Name: "The campaign", Why: "the why"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	for _, text := range []string{first, second} {
		if _, err := jobs.AddTask(context.Background(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatalf("cannot add %q to job %s: %v", text, jobID, err)
		}
	}
	return jobID
}

// TestACreateFromInsideAJobsTaskIsRefusedAndMakesNoJob proves a job's task does
// not make a job: the loop starts a job's task with the task's own text as its
// ask, so a record whose ask is an unfinished task of a running job is that
// job's task, and create is refused naming the job and add_task, with no job
// made. Once that task is finished, the same words are a person's ask again.
func TestACreateFromInsideAJobsTaskIsRefusedAndMakesNoJob(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theMoment))
	jobID := aJobWithTwoTasks(t, jobs, "draft the post", "post it")
	records := heldRecord{record: contract.Record{Goal: contract.Goal{Ask: "draft the post"}}}
	tool := job.New(job.Settings{Jobs: jobs, Records: records})

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "make the poster"})
	if err == nil {
		t.Fatalf("a create from inside a job's task made a job inside the job")
	}
	for _, told := range []string{"this task belongs to job " + jobID, "add_task", "instead of making a job inside it"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 1 {
		t.Errorf("the store holds %d jobs after the refused create, want only the one whose task is running", len(summaries))
	}

	taken, there, err := jobs.NextTask(context.Background(), theMoment)
	if err != nil || !there || taken.Text != "draft the post" {
		t.Fatalf("the store handed out %+v (%v, %v), want the job's first task", taken, there, err)
	}
	if _, err := jobs.FinishTask(context.Background(), jobID, taken.TaskID, "posted", false); err != nil {
		t.Fatalf("cannot finish the task: %v", err)
	}
	if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "make the poster"}); err != nil {
		t.Errorf("a create whose ask is the text of a finished task was refused, and a finished task never runs again: %v", err)
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 2 {
		t.Errorf("the store holds %d jobs, want the first and the one just made", len(summaries))
	}
}

// TestACreateThatCannotReadTheJobsToTellWhoseTaskThisIsSaysSo proves a store
// that cannot be read refuses the create with its reason rather than making a
// job that might sit inside another.
func TestACreateThatCannotReadTheJobsToTellWhoseTaskThisIsSaysSo(t *testing.T) {
	broken := &brokenJobs{loadFails: true}
	tool := newToolOverBrokenJobs(t, broken)
	aJobWithTwoTasks(t, broken, "draft the post", "post it")

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "make the poster"})
	if err == nil {
		t.Fatalf("a create over a store that cannot be read made a job")
	}
	if !errors.Is(err, errTheStoreRefused) {
		t.Errorf("the refusal reads %q and does not carry the store's reason", err)
	}
	if summaries, listErr := broken.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 1 {
		t.Errorf("the store holds %d jobs after the refused create, want only the one already there", len(summaries))
	}
}
