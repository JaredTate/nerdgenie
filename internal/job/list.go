package job

import (
	"context"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// List returns every job, oldest first, which is the order they are worked
// through and the order "/jobs" prints them in.
func (jobs *Jobs) List(ctx context.Context) ([]contract.JobSummary, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	listed := make([]contract.JobSummary, 0, len(jobs.order))
	for _, jobID := range jobs.order {
		summary, err := jobs.summaryOf(ctx, jobID, jobs.held[jobID])
		if err != nil {
			return nil, err
		}
		listed = append(listed, summary)
	}
	return listed, nil
}

// Load returns the job's record, which is what rides above the task record while
// one of the job's tasks runs and what "/jobs 4" prints.
func (jobs *Jobs) Load(_ context.Context, jobID string) (contract.Record, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return contract.Record{}, err
	}
	return held.keeper.Record(), nil
}

// summaryOf builds the one line "/jobs" prints about a job and the header that
// rides above a task record while one of the job's tasks runs. The caller holds
// the lock.
func (jobs *Jobs) summaryOf(ctx context.Context, jobID string, held *heldJob) (contract.JobSummary, error) {
	now := jobs.clock.Now()
	running, err := jobs.claimedTasks(ctx, jobID, now)
	if err != nil {
		return contract.JobSummary{}, err
	}
	listed := held.keeper.Record().Work.Tasks
	summary := contract.JobSummary{
		ID:             jobID,
		Title:          held.keeper.Record().Goal.Ask,
		State:          held.state.State,
		TasksTotal:     len(listed),
		LastRun:        held.state.LastRun,
		NextRun:        held.state.NextRun,
		FailuresInARow: held.state.FailuresInARow,
	}
	for _, task := range listed {
		if task.Done {
			summary.TasksDone++
			continue
		}
		if summary.NextTaskID != "" || running[task.TaskID] {
			continue
		}
		summary.NextTaskID = task.TaskID
		summary.NextDue = held.state.facts(task.TaskID).DueAt
	}
	return summary, nil
}

// writeProgress writes the header of a job's record: how many of its tasks are
// done and which one is due next. It is the harness's line, not the model's, so
// it is written here every time the list or the state changes. The caller holds
// the lock.
func (jobs *Jobs) writeProgress(ctx context.Context, jobID string, held *heldJob) error {
	listed := held.keeper.Record().Work.Tasks
	done := 0
	nextDue := ""
	for _, task := range listed {
		if task.Done {
			done++
			continue
		}
		if nextDue == "" {
			nextDue = nextDueLine(task)
		}
	}
	if err := held.keeper.SetProgress(ctx, done, len(listed), nextDue); err != nil {
		return fmt.Errorf("cannot write the progress of job %s: %w", jobID, err)
	}
	return nil
}

// nextDueLine says in plain words which task is next and when, for the job's
// header.
func nextDueLine(task contract.JobTask) string {
	if task.DueAt == "" {
		return "task " + task.TaskID + " now"
	}
	return "task " + task.TaskID + " " + task.DueAt
}
