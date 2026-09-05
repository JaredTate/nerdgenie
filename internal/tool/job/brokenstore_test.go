package job_test

// These tests are the create action over a store that fails partway: a create
// is all or nothing, so a job whose task the store refuses is switched off and
// the store's reason goes back to the model whole.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/job"
)

// errTheStoreRefused is the reason a broken store gives, which the model must
// read in full so that it does not try the same create again.
var errTheStoreRefused = errors.New("the job store cannot write the task because the disk is full")

// brokenJobs is the fake job store with one of its writes made to fail, so that
// a create can be watched leaving nothing behind when the store fails halfway.
type brokenJobs struct {
	*testkit.FakeJob
	// createFails makes every Create fail.
	createFails bool
	// addTaskFailsAt is which AddTask call fails, counting from one, or zero
	// for none.
	addTaskFailsAt int
	// switchOffFails makes every SwitchOff fail.
	switchOffFails bool
	// loadFails makes every Load fail.
	loadFails bool
	// addTaskCalls counts the AddTask calls so far.
	addTaskCalls int
}

// Load fails when told to, and otherwise returns the job's record.
func (jobs *brokenJobs) Load(ctx context.Context, jobID string) (contract.Record, error) {
	if jobs.loadFails {
		return contract.Record{}, errTheStoreRefused
	}
	return jobs.FakeJob.Load(ctx, jobID)
}

// Create fails when told to, and otherwise makes the job.
func (jobs *brokenJobs) Create(ctx context.Context, wanted contract.NewJob) (string, error) {
	if jobs.createFails {
		return "", errTheStoreRefused
	}
	return jobs.FakeJob.Create(ctx, wanted)
}

// AddTask fails on the call it was told to, and otherwise records the task.
func (jobs *brokenJobs) AddTask(ctx context.Context, wanted contract.NewTask) (string, error) {
	jobs.addTaskCalls++
	if jobs.addTaskCalls == jobs.addTaskFailsAt {
		return "", errTheStoreRefused
	}
	return jobs.FakeJob.AddTask(ctx, wanted)
}

// SwitchOff fails when told to, and otherwise switches the job off.
func (jobs *brokenJobs) SwitchOff(ctx context.Context, jobID string) error {
	if jobs.switchOffFails {
		return errors.New("the job store cannot change the job's state")
	}
	return jobs.FakeJob.SwitchOff(ctx, jobID)
}

// newToolOverBrokenJobs builds the job tool over a record and a broken store.
func newToolOverBrokenJobs(t *testing.T, broken *brokenJobs) *job.Tool {
	t.Helper()
	broken.FakeJob = testkit.NewFakeJob(testkit.NewFakeClock(theMoment))
	records := heldRecord{record: contract.Record{Goal: contract.Goal{Ask: "the ask"}}}
	return job.New(job.Settings{Jobs: broken, Records: records})
}

// runningJobs is how many of the listed jobs are running.
func runningJobs(t *testing.T, jobs contract.Job) int {
	t.Helper()
	summaries, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	running := 0
	for _, summary := range summaries {
		if summary.State == contract.JobRunning {
			running++
		}
	}
	return running
}

// TestACreateWhoseTaskListCannotBeRecordedSwitchesTheJobOffAndSaysWhy proves a
// create is all or nothing: when the store refuses a task after the job is made,
// the job is switched off so no empty running job is left for a retry to stand
// beside, and the refusal names the job and carries the store's reason in full.
// On the real store a retrying model left twenty-nine empty running jobs behind.
func TestACreateWhoseTaskListCannotBeRecordedSwitchesTheJobOffAndSaysWhy(t *testing.T) {
	broken := &brokenJobs{addTaskFailsAt: 2}
	tool := newToolOverBrokenJobs(t, broken)

	_, err := run(t, tool, map[string]any{
		"action": "create", "why": "the why", "tasks": []any{"the first task", "the second task", "the third task"},
	})
	if err == nil {
		t.Fatalf("a create whose second task the store refused was reported as done")
	}
	if !errors.Is(err, errTheStoreRefused) || !strings.Contains(err.Error(), errTheStoreRefused.Error()) {
		t.Errorf("the refusal reads %q and does not carry the store's reason in full", err)
	}
	for _, told := range []string{"job 1", "switched off"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
	if running := runningJobs(t, broken); running != 0 {
		t.Errorf("%d jobs are still running after the failed create, want none", running)
	}
	summaries, listErr := broken.List(context.Background())
	if listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	}
	if len(summaries) != 1 || summaries[0].State != contract.JobOff {
		t.Errorf("the store holds %v, want the one job switched off", summaries)
	}
}

// TestACreateWhoseJobCannotBeMadeLeavesNothing proves a store that refuses the
// job itself leaves nothing behind and its reason reaches the model in full.
func TestACreateWhoseJobCannotBeMadeLeavesNothing(t *testing.T) {
	broken := &brokenJobs{createFails: true}
	tool := newToolOverBrokenJobs(t, broken)

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": []any{"the first task"}})
	if err == nil {
		t.Fatalf("a create the store refused was reported as done")
	}
	if !errors.Is(err, errTheStoreRefused) || !strings.Contains(err.Error(), errTheStoreRefused.Error()) {
		t.Errorf("the refusal reads %q and does not carry the store's reason in full", err)
	}
	if summaries, listErr := broken.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused create still left %d jobs behind", len(summaries))
	}
}

// TestACreateThatCannotSwitchItsBrokenJobOffSaysSo proves that when the store
// refuses a task and then refuses to switch the job off, the refusal says the
// job may be running with half its list, so the user can be told.
func TestACreateThatCannotSwitchItsBrokenJobOffSaysSo(t *testing.T) {
	broken := &brokenJobs{addTaskFailsAt: 1, switchOffFails: true}
	tool := newToolOverBrokenJobs(t, broken)

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": []any{"the first task"}})
	if err == nil {
		t.Fatalf("a create whose task the store refused was reported as done")
	}
	if !errors.Is(err, errTheStoreRefused) {
		t.Errorf("the refusal reads %q and does not carry the store's reason", err)
	}
	for _, told := range []string{"job 1", "cannot change the job's state", "tell the user"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
}

// Resume is here so that brokenJobs keeps to the job contract, which gained it; this
// double never resumes a job.
func (jobs *brokenJobs) Resume(context.Context, string) error { return nil }
