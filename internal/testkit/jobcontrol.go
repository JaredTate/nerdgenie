package testkit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The fake job store's controls: what a person does to a job through the
// commands, which is to run it now, pause it, resume it, put it down on a
// task, or switch it off. The store itself, and how it hands out and finishes
// tasks, is in job.go.

// RunNow starts the job's next task without waiting for its date: the job runs
// again, every claim its tasks held is let go, the way the real store lets go
// of the claims a paused run held, and the next unfinished task loses its due
// date, so that NextTask hands it out at once. A run-now that only changed the
// state would leave the task waiting for the very date the user just overrode,
// or skip it as still running. A job with a schedule ticks at once as well:
// its next run is moved to now, as the real store moves it.
func (jobs *FakeJob) RunNow(_ context.Context, jobID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}

	entry.summary.State = contract.JobRunning
	entry.putDown = nil
	if entry.schedule != nil {
		entry.summary.NextRun = jobs.clock.Now()
	}
	for index := range entry.tasks {
		entry.tasks[index].running = false
	}
	for index := range entry.tasks {
		task := &entry.tasks[index]
		if task.task.Done {
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

// Resume sets a paused job running again and lets go of the running marks its
// tasks held, so that the task it was paused on is handed out first rather than
// skipped as running. It touches no date and no tick: a task waiting for a day
// goes on waiting, and a schedule fires when it was going to. Like the real
// store's resume it forgets the put-down mark and the failures that stopped the
// job, so that the next failure starts the count afresh.
func (jobs *FakeJob) Resume(_ context.Context, jobID string) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}
	entry.summary.State = contract.JobRunning
	entry.summary.FailuresInARow = 0
	entry.putDown = nil
	for index := range entry.tasks {
		entry.tasks[index].running = false
	}
	return nil
}

// PickUpOnce says whether the job may pick the task up itself after the
// harness's guard stopped it: yes the first time and no from then on, the way
// the real store answers. A task that is finished or not there, and a job that
// is not there, are refused with an error naming them.
func (jobs *FakeJob) PickUpOnce(_ context.Context, jobID string, taskID string) (bool, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[jobID]
	if !held {
		return false, fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", jobID)
	}
	task := jobs.findTask(entry, taskID)
	if task == nil {
		return false, fmt.Errorf("the job %s has no task %q, so list its tasks to see what there is", jobID, taskID)
	}
	if task.task.Done {
		return false, fmt.Errorf("task %s of job %s is already finished and its report is %s, so there is nothing to pick up", taskID, jobID, task.task.ReportID)
	}
	if task.pickedUp {
		return false, nil
	}
	task.pickedUp = true
	return true, nil
}

// PutDown pauses a job on one of its tasks and keeps the mark, which RunNow
// forgets again. A task that has finished cannot be put down, because nothing
// could pick it up, and a job that is off cannot be either, because the person
// ended it; each is refused with the task or the job named, the way the real
// store refuses them.
func (jobs *FakeJob) PutDown(_ context.Context, mark contract.PutDownMark) error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	entry, held := jobs.entries[mark.Task.JobID]
	if !held {
		return fmt.Errorf("there is no job numbered %q, so list the jobs to see what there is", mark.Task.JobID)
	}
	task := jobs.findTask(entry, mark.Task.TaskID)
	if task == nil {
		return fmt.Errorf("the job %s has no task %q, so list its tasks to see what there is", mark.Task.JobID, mark.Task.TaskID)
	}
	if task.task.Done {
		return fmt.Errorf("task %s of job %s is already finished and its report is %s, so there is nothing to put down", mark.Task.TaskID, mark.Task.JobID, task.task.ReportID)
	}
	if entry.summary.State == contract.JobOff {
		return fmt.Errorf("job %s is switched off, so nothing of it is running to put down; run it again with /cron run %s first", mark.Task.JobID, mark.Task.JobID)
	}
	entry.summary.State = contract.JobPaused
	entry.putDown = &mark
	return nil
}

// PutDownTask hands back the mark of the paused job whose run is newest, and
// false when no job is put down.
func (jobs *FakeJob) PutDownTask(_ context.Context) (contract.PutDownMark, bool, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	newest, there := contract.PutDownMark{}, false
	for _, jobID := range jobs.order {
		entry := jobs.entries[jobID]
		if entry.putDown == nil || entry.summary.State != contract.JobPaused {
			continue
		}
		if !there || runNumberOf(entry.putDown.Run) > runNumberOf(newest.Run) {
			newest, there = *entry.putDown, true
		}
	}
	return newest, there, nil
}

// runNumberOf reads a run's number, and is zero for one that is not a number.
func runNumberOf(run string) int {
	number, err := strconv.Atoi(run)
	if err != nil {
		return 0
	}
	return number
}

// SwitchOff stops the job for good.
func (jobs *FakeJob) SwitchOff(_ context.Context, jobID string) error {
	return jobs.setState(jobID, contract.JobOff)
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
