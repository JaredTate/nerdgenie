package testkit

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// fakeJobEntry is one job the fake holds.
type fakeJobEntry struct {
	summary  contract.JobSummary
	schedule *contract.Schedule
	template string
	tasks    []contract.JobTask
}

// FakeJob holds jobs in memory: create one, add tasks to it, list them, and
// change its state the four ways the contract names.
type FakeJob struct {
	guard      sync.Mutex
	clock      contract.Clock
	order      []string
	entries    map[string]*fakeJobEntry
	nextJob    int
	nextTask   int
	defaultGap time.Duration
}

// NewFakeJob returns an empty job store that reads the time from the clock given.
func NewFakeJob(clock contract.Clock) *FakeJob {
	return &FakeJob{
		clock:      clock,
		entries:    map[string]*fakeJobEntry{},
		nextJob:    1,
		nextTask:   1,
		defaultGap: 24 * time.Hour,
	}
}

// Create starts a job and returns its id.
func (jobs *FakeJob) Create(_ context.Context, wanted contract.NewJob) (string, error) {
	if wanted.Ask == "" {
		return "", fmt.Errorf("a job needs the user's ask before it can be created, so pass the message word for word")
	}
	jobs.guard.Lock()
	defer jobs.guard.Unlock()

	jobID := strconv.Itoa(jobs.nextJob)
	jobs.nextJob++
	entry := &fakeJobEntry{
		summary:  contract.JobSummary{ID: jobID, Title: wanted.Ask, State: contract.JobRunning},
		schedule: wanted.Schedule,
		template: wanted.TaskTemplate,
	}
	if wanted.Schedule != nil {
		entry.summary.NextRun = jobs.clock.Now().Add(jobs.defaultGap)
	}
	jobs.order = append(jobs.order, jobID)
	jobs.entries[jobID] = entry
	return jobID, nil
}

// AddTask puts one more task on a job's list and returns its id.
func (jobs *FakeJob) AddTask(_ context.Context, wanted contract.NewTask) (string, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()

	entry, held := jobs.entries[wanted.JobID]
	if !held {
		return "", fmt.Errorf("there is no job numbered %q, so create the job before adding tasks to it", wanted.JobID)
	}
	taskID := contract.TaskID(jobs.nextTask)
	jobs.nextTask++

	dueAt := ""
	if !wanted.DueAt.IsZero() {
		dueAt = wanted.DueAt.Format(time.RFC3339)
	}
	entry.tasks = append(entry.tasks, contract.JobTask{TaskID: taskID, Text: wanted.Text, DueAt: dueAt})
	entry.summary.TasksTotal = len(entry.tasks)
	entry.summary.NextTaskID = jobs.firstUnfinished(entry)
	entry.summary.NextDue = wanted.DueAt
	return taskID, nil
}

// List returns every job, in the order they were created.
func (jobs *FakeJob) List(_ context.Context) ([]contract.JobSummary, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	listed := make([]contract.JobSummary, 0, len(jobs.order))
	for _, jobID := range jobs.order {
		listed = append(listed, jobs.entries[jobID].summary)
	}
	return listed, nil
}

// RunNow starts the job's next task without waiting for its date.
func (jobs *FakeJob) RunNow(_ context.Context, jobID string) error {
	return jobs.setState(jobID, contract.JobRunning)
}

// Pause stops the job after the running task finishes.
func (jobs *FakeJob) Pause(_ context.Context, jobID string) error {
	return jobs.setState(jobID, contract.JobPaused)
}

// SwitchOff stops the job for good.
func (jobs *FakeJob) SwitchOff(_ context.Context, jobID string) error {
	return jobs.setState(jobID, contract.JobOff)
}

// Tasks is one job's task list, which a test uses to check what the model
// planned.
func (jobs *FakeJob) Tasks(jobID string) []contract.JobTask {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return nil
	}
	copied := make([]contract.JobTask, len(entry.tasks))
	copy(copied, entry.tasks)
	return copied
}

// setState changes one job's state and reports plainly when there is no such
// job.
func (jobs *FakeJob) setState(jobID string, state contract.JobState) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}
	entry.summary.State = state
	entry.summary.LastRun = jobs.clock.Now()
	return nil
}

// firstUnfinished is the id of the next task to run, or an empty string when
// every task is done.
func (jobs *FakeJob) firstUnfinished(entry *fakeJobEntry) string {
	for _, task := range entry.tasks {
		if !task.Done {
			return task.TaskID
		}
	}
	return ""
}
