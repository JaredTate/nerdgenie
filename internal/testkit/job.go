package testkit

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// fakeTask is one task on a fake job's list, with the two things the contract's
// task type does not carry: the due time as a time, and whether a schedule made
// it.
type fakeTask struct {
	task       contract.JobTask
	dueAt      time.Time
	unattended bool
	running    bool
}

// fakeJobEntry is one job the fake holds.
type fakeJobEntry struct {
	summary  contract.JobSummary
	ask      string
	why      string
	schedule *contract.Schedule
	template string
	tasks    []fakeTask
	reports  []contract.ResultLine
}

// FakeJob holds jobs in memory: create one, add tasks to it, list them, change
// its state, hand out the next due task, and take a finished task's report.
type FakeJob struct {
	guard      sync.Mutex
	clock      contract.Clock
	order      []string
	entries    map[string]*fakeJobEntry
	nextJob    int
	nextTask   int
	defaultGap time.Duration
}

// The two failure counts from the design: three failures in a row pause a job,
// and ten switch a scheduled job off.
const (
	failuresThatPause     = 3
	failuresThatSwitchOff = 10
)

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
		ask:      wanted.Ask,
		why:      wanted.Why,
		schedule: wanted.Schedule,
		template: wanted.TaskTemplate,
	}
	if wanted.Schedule != nil {
		entry.summary.NextRun = jobs.clock.Now().Add(jobs.scheduleGap(wanted.Schedule))
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
	return jobs.appendTask(entry, wanted.Text, wanted.DueAt, false), nil
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

// RunNow starts the job's next task without waiting for its date: the job runs
// again and the next unfinished task loses its due date, so that NextTask hands
// it out at once. A run-now that only changed the state would leave the task
// waiting for the very date the user just overrode.
func (jobs *FakeJob) RunNow(_ context.Context, jobID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}

	entry.summary.State = contract.JobRunning
	for index := range entry.tasks {
		task := &entry.tasks[index]
		if task.task.Done || task.running {
			continue
		}
		task.dueAt = time.Time{}
		task.task.DueAt = ""
		entry.summary.NextDue = time.Time{}
		break
	}
	return nil
}

// Pause stops the job after the running task finishes.
func (jobs *FakeJob) Pause(_ context.Context, jobID string) error {
	return jobs.setState(jobID, contract.JobPaused)
}

// SwitchOff stops the job for good.
func (jobs *FakeJob) SwitchOff(_ context.Context, jobID string) error {
	return jobs.setState(jobID, contract.JobOff)
}

// NextTask hands out the first unfinished task whose due time has passed, from
// the oldest running job, making one from the template first when a schedule's
// tick has come.
func (jobs *FakeJob) NextTask(_ context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	for _, jobID := range jobs.order {
		entry := jobs.entries[jobID]
		if entry.summary.State != contract.JobRunning {
			continue
		}
		jobs.tickSchedule(entry, now)
		for index := range entry.tasks {
			task := &entry.tasks[index]
			if task.task.Done || task.running {
				continue
			}
			if task.dueAt.After(now) {
				break
			}
			task.running = true
			return contract.TaskToRun{JobID: jobID, TaskID: task.task.TaskID, Text: task.task.Text, Unattended: task.unattended}, true, nil
		}
	}
	return contract.TaskToRun{}, false, nil
}

// FinishTask writes a report into the job, marks the task done unless it failed,
// and applies the two failure counts.
func (jobs *FakeJob) FinishTask(_ context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return "", fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}
	task := jobs.findTask(entry, taskID)
	if task == nil {
		return "", fmt.Errorf("the job %s has no task %q, so list its tasks to see what there is", jobID, taskID)
	}
	reportID := contract.ReportID(jobID, len(entry.reports)+1)
	entry.reports = append(entry.reports, contract.ResultLine{ID: reportID, Summary: report})
	entry.summary.LastRun = jobs.clock.Now()
	task.running = false
	if failed {
		entry.summary.FailuresInARow++
		jobs.applyFailureCount(entry)
	} else {
		task.task.Done = true
		task.task.ReportID = reportID
		entry.summary.FailuresInARow = 0
		entry.summary.TasksDone++
	}
	entry.summary.NextTaskID = jobs.firstUnfinished(entry)
	if entry.summary.NextTaskID == "" && entry.schedule == nil && entry.summary.State == contract.JobRunning {
		entry.summary.State = contract.JobDone
	}
	return reportID, nil
}

// Load returns the job as a record: the same four parts as a task, with the
// task list in place of the plan and the reports in place of the results.
func (jobs *FakeJob) Load(_ context.Context, jobID string) (contract.Record, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return contract.Record{}, fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}
	tasks := make([]contract.JobTask, 0, len(entry.tasks))
	for _, task := range entry.tasks {
		tasks = append(tasks, task.task)
	}
	progress := fmt.Sprintf("%d of %d tasks done", entry.summary.TasksDone, entry.summary.TasksTotal)
	return contract.Record{
		Header: contract.Header{
			Kind:       contract.RecordJob,
			ID:         jobID,
			Status:     recordStatusOfJob(entry.summary.State),
			TasksDone:  entry.summary.TasksDone,
			TasksTotal: entry.summary.TasksTotal,
			NextDue:    jobs.nextDueLine(entry),
		},
		Goal:    contract.Goal{Ask: entry.ask, Why: entry.why},
		Work:    contract.Work{Situation: []string{progress}, Tasks: tasks, Results: append([]contract.ResultLine(nil), entry.reports...)},
		Lessons: contract.Lessons{},
	}, nil
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
	copied := make([]contract.JobTask, 0, len(entry.tasks))
	for _, task := range entry.tasks {
		copied = append(copied, task.task)
	}
	return copied
}

// appendTask adds one task to a job's list and returns its id. The caller holds
// the lock.
func (jobs *FakeJob) appendTask(entry *fakeJobEntry, text string, dueAt time.Time, unattended bool) string {
	taskID := contract.TaskID(jobs.nextTask)
	jobs.nextTask++
	dueText := ""
	if !dueAt.IsZero() {
		dueText = dueAt.Format(time.RFC3339)
	}
	entry.tasks = append(entry.tasks, fakeTask{
		task:       contract.JobTask{TaskID: taskID, Text: text, DueAt: dueText},
		dueAt:      dueAt,
		unattended: unattended,
	})
	entry.summary.TasksTotal = len(entry.tasks)
	entry.summary.NextTaskID = jobs.firstUnfinished(entry)
	entry.summary.NextDue = dueAt
	return taskID
}

// insertTaskInDueOrder adds one task ahead of every unfinished task that is due
// later than it is, and returns its id. The caller holds the lock.
func (jobs *FakeJob) insertTaskInDueOrder(entry *fakeJobEntry, text string, dueAt time.Time, unattended bool) string {
	taskID := jobs.appendTask(entry, text, dueAt, unattended)
	added := len(entry.tasks) - 1
	place := added
	for index := range entry.tasks[:added] {
		waiting := entry.tasks[index]
		if waiting.task.Done || waiting.running {
			continue
		}
		if waiting.dueAt.After(dueAt) {
			place = index
			break
		}
	}
	if place != added {
		moved := entry.tasks[added]
		copy(entry.tasks[place+1:], entry.tasks[place:added])
		entry.tasks[place] = moved
	}
	entry.summary.NextTaskID = jobs.firstUnfinished(entry)
	return taskID
}

// tickSchedule makes one task from the template when a scheduled job's tick has
// come, and moves the next run on. The task goes into the list in date order
// rather than at the end, because NextTask stops at the first task that is not
// due yet, and a tick behind a task dated next week would never run. The caller
// holds the lock.
func (jobs *FakeJob) tickSchedule(entry *fakeJobEntry, now time.Time) {
	if entry.schedule == nil || entry.summary.NextRun.IsZero() || now.Before(entry.summary.NextRun) {
		return
	}
	jobs.insertTaskInDueOrder(entry, entry.template, entry.summary.NextRun, true)
	if entry.schedule.Kind == contract.ScheduleAt {
		entry.summary.NextRun = time.Time{}
		return
	}
	entry.summary.NextRun = entry.summary.NextRun.Add(jobs.scheduleGap(entry.schedule))
}

// scheduleGap is how far apart the fake fires a schedule: the interval for
// "every", and one day for the other two kinds, because the fake reads no cron
// expressions.
func (jobs *FakeJob) scheduleGap(schedule *contract.Schedule) time.Duration {
	if schedule.Kind == contract.ScheduleEvery && schedule.Every > 0 {
		return schedule.Every
	}
	return jobs.defaultGap
}

// applyFailureCount stops a job that keeps failing. A plain job is paused at
// three failures in a row, because a person is there to look at it. A scheduled
// job keeps going until ten, because a schedule is meant to survive a bad
// afternoon, and then it is switched off for good. Pausing a scheduled job at
// three would make the ten-failure rule unreachable. The caller holds the lock.
func (jobs *FakeJob) applyFailureCount(entry *fakeJobEntry) {
	if entry.schedule != nil {
		if entry.summary.FailuresInARow >= failuresThatSwitchOff {
			entry.summary.State = contract.JobOff
		}
		return
	}
	if entry.summary.FailuresInARow >= failuresThatPause {
		entry.summary.State = contract.JobPaused
	}
}

// findTask returns the task with that id, or nil. The caller holds the lock.
func (jobs *FakeJob) findTask(entry *fakeJobEntry, taskID string) *fakeTask {
	for index := range entry.tasks {
		if entry.tasks[index].task.TaskID == taskID {
			return &entry.tasks[index]
		}
	}
	return nil
}

// nextDueLine says in plain words which task is next and when, for the job's
// header. The caller holds the lock.
func (jobs *FakeJob) nextDueLine(entry *fakeJobEntry) string {
	for _, task := range entry.tasks {
		if task.task.Done {
			continue
		}
		if task.dueAt.IsZero() {
			return "task " + task.task.TaskID + " now"
		}
		return "task " + task.task.TaskID + " at " + task.dueAt.Format(time.RFC3339)
	}
	return ""
}

// recordStatusOfJob maps a job's state onto the record statuses the header
// prints.
func recordStatusOfJob(state contract.JobState) contract.RecordStatus {
	switch state {
	case contract.JobPaused:
		return contract.StatusWaiting
	case contract.JobOff:
		return contract.StatusStopped
	case contract.JobDone:
		return contract.StatusDone
	default:
		return contract.StatusRunning
	}
}

// setState changes one job's state and reports plainly when there is no such
// job. It leaves LastRun alone: that is when a task last finished, and pausing a
// job finishes nothing.
func (jobs *FakeJob) setState(jobID string, state contract.JobState) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}
	entry.summary.State = state
	return nil
}

// firstUnfinished is the id of the next task to run, or an empty string when
// every task is done.
func (jobs *FakeJob) firstUnfinished(entry *fakeJobEntry) string {
	for _, task := range entry.tasks {
		if !task.task.Done {
			return task.task.TaskID
		}
	}
	return ""
}
