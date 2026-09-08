package job_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/job"
)

// manyJobs is a job store holding more jobs than one listing shows.
type manyJobs struct {
	held int
}

// Create is never called in these tests.
func (manyJobs) Create(context.Context, contract.NewJob) (string, error) { return "", nil }

// AddTask is never called in these tests.
func (manyJobs) AddTask(context.Context, contract.NewTask) (string, error) { return "", nil }

// List returns more jobs than one listing shows, one of which is waiting for a
// date and one of which has nothing to do next.
func (jobs manyJobs) List(context.Context) ([]contract.JobSummary, error) {
	found := []contract.JobSummary{}
	for at := range jobs.held {
		summary := contract.JobSummary{ID: fmt.Sprint(at + 1), State: contract.JobRunning, TasksTotal: 2}
		switch at % 3 {
		case 0:
			summary.NextTaskID = "t1"
			summary.NextDue = theMoment
		case 1:
			summary.NextTaskID = "t2"
		}
		found = append(found, summary)
	}
	return found, nil
}

// RunNow is never called in these tests.
func (manyJobs) RunNow(context.Context, string) error { return nil }

// Pause is never called in these tests.
func (manyJobs) Pause(context.Context, string) error { return nil }

// SwitchOff is never called in these tests.
func (manyJobs) SwitchOff(context.Context, string) error { return nil }

// NextTask is never called in these tests.
func (manyJobs) NextTask(context.Context, time.Time) (contract.TaskToRun, bool, error) {
	return contract.TaskToRun{}, false, nil
}

// FinishTask is never called in these tests.
func (manyJobs) FinishTask(context.Context, string, string, string, bool) (string, error) {
	return "", nil
}

// Load is never called in these tests.
func (manyJobs) Load(context.Context, string) (contract.Record, error) {
	return contract.Record{}, nil
}

func TestAListingStopsAtTheCapAndSaysWhatEachJobDoesNext(t *testing.T) {
	tool := job.New(job.Settings{Jobs: manyJobs{held: job.MaxListed + 4}})

	output, err := run(t, tool, map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("listing many jobs failed: %v", err)
	}
	if !strings.Contains(output.Text, "4 more jobs") {
		t.Errorf("the listing stopped without saying how many jobs it left out: %q", output.Text)
	}
	for _, wanted := range []string{"next t1 at", "next t2", "next nothing"} {
		if !strings.Contains(output.Text, wanted) {
			t.Errorf("the listing does not say %q anywhere: %q", wanted, output.Text)
		}
	}
}

func TestAScheduleInATimezoneNobodyKnowsIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"action": "create", "ask": "the ask", "why": "the why",
		"schedule": map[string]any{"kind": "every", "every": "24h", "timezone": "Mars/Olympus_Mons"},
	})
	if err == nil {
		t.Fatalf("a schedule in a timezone nobody knows was accepted")
	}
	if !strings.Contains(err.Error(), "timezone") {
		t.Errorf("the refusal reads %q and does not say what was wrong", err)
	}
}

func TestAScheduleOfTheKindAtWithNoMomentIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"action": "create", "ask": "the ask", "schedule": map[string]any{"kind": "at"},
	}); err == nil {
		t.Errorf("a schedule of the kind at with no moment was accepted")
	}
	if _, err := run(t, tool, map[string]any{
		"action": "create", "ask": "the ask", "schedule": map[string]any{"kind": "cron"},
	}); err == nil {
		t.Errorf("a schedule of the kind cron with no expression was accepted")
	}
}

func TestADateWrittenInTheOtherTwoShapesIsRead(t *testing.T) {
	tool, jobs := newTool(t)
	if _, err := run(t, tool, map[string]any{"action": "create", "ask": "the ask", "text": "the first task"}); err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}

	for _, written := range []string{"2026-03-01 14:00", "2026-03-01"} {
		if _, err := run(t, tool, map[string]any{
			"action": "add_task", "job_id": "1", "text": "the task", "due_at": written,
		}); err != nil {
			t.Errorf("a date written as %q was refused: %v", written, err)
		}
	}
	if tasks := jobs.Tasks("1"); len(tasks) != 3 {
		t.Errorf("the job holds %d tasks, want the first task and the two that were added", len(tasks))
	}
}

// PutDown is here so that manyJobs keeps to the job contract, which gained it
// after this double was written; the job tool never calls it.
func (manyJobs) PutDown(context.Context, contract.PutDownMark) error { return nil }

// PutDownTask is here for the same reason; nothing is ever put down here.
func (manyJobs) PutDownTask(context.Context) (contract.PutDownMark, bool, error) {
	return contract.PutDownMark{}, false, nil
}

// PickUpOnce is here for the same reason; no task is ever picked up here.
func (manyJobs) PickUpOnce(context.Context, string, string) (bool, error) { return false, nil }

// Defer is here for the same reason; no task is ever set aside here.
func (manyJobs) Defer(context.Context, string, string) (bool, error) { return false, nil }

// ProveDoneLine is here for the same reason; no done line is ever proved here.
func (manyJobs) ProveDoneLine(context.Context, string, int, string) error { return nil }

// SetProjectFolder is here for the same reason; no folder is ever set here.
func (manyJobs) SetProjectFolder(context.Context, string, string) error { return nil }

// Timing is here so that manyJobs keeps to the job contract, which gained it;
// this double keeps no moments.
func (manyJobs) Timing(context.Context, string) (contract.JobTiming, error) {
	return contract.JobTiming{}, nil
}

// Resume is here so that manyJobs keeps to the job contract, which gained it; this
// double never resumes a job.
func (manyJobs) Resume(context.Context, string) error { return nil }
