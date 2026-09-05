package testkit_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// finishedPutDownJobStore puts a job down on a task that has already finished,
// which the fake did before the put-down rule was pinned.
type finishedPutDownJobStore struct{ *testkit.FakeJob }

// PutDown takes the mark without a word when the task is finished.
func (store finishedPutDownJobStore) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	for _, task := range store.FakeJob.Tasks(mark.Task.JobID) {
		if task.TaskID == mark.Task.TaskID && task.Done {
			return nil
		}
	}
	return store.FakeJob.PutDown(ctx, mark)
}

// offPutDownJobStore puts down a job that was switched off.
type offPutDownJobStore struct{ *testkit.FakeJob }

// PutDown takes the mark without a word when the job is off.
func (store offPutDownJobStore) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	listed, err := store.FakeJob.List(ctx)
	if err != nil {
		return err
	}
	for _, summary := range listed {
		if summary.ID == mark.Task.JobID && summary.State == contract.JobOff {
			return nil
		}
	}
	return store.FakeJob.PutDown(ctx, mark)
}

// eagerClosingJobStore closes a job the moment its last task finishes, whether
// or not the job is running, which is what the real store did before a job
// closed only while running.
type eagerClosingJobStore struct{ *testkit.FakeJob }

// everyTaskIsDone says whether a job with at least one task has finished all
// of them.
func (store eagerClosingJobStore) everyTaskIsDone(jobID string) bool {
	tasks := store.FakeJob.Tasks(jobID)
	for _, task := range tasks {
		if !task.Done {
			return false
		}
	}
	return len(tasks) > 0
}

// List reports every job whose tasks are all done as done.
func (store eagerClosingJobStore) List(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := store.FakeJob.List(ctx)
	for index, summary := range listed {
		if store.everyTaskIsDone(summary.ID) {
			listed[index].State = contract.JobDone
		}
	}
	return listed, err
}

// Load hands back the record standing at done once every task is done.
func (store eagerClosingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	if store.everyTaskIsDone(jobID) {
		record.Header.Status = contract.StatusDone
	}
	return record, err
}

// offStayingJobStore leaves a switched-off job off when it is run now.
type offStayingJobStore struct{ *testkit.FakeJob }

// RunNow does nothing for a job that is off.
func (store offStayingJobStore) RunNow(ctx context.Context, jobID string) error {
	listed, err := store.FakeJob.List(ctx)
	if err != nil {
		return err
	}
	for _, summary := range listed {
		if summary.ID == jobID && summary.State == contract.JobOff {
			return nil
		}
	}
	return store.FakeJob.RunNow(ctx, jobID)
}

// dateStoppingJobStore stops at a task whose date has not come rather than
// reading past it, which is what the fake did before the next-task rule was
// pinned.
type dateStoppingJobStore struct{ *testkit.FakeJob }

// NextTask hands out nothing when an unfinished task with a date still to
// come sits ahead of the task the fake would hand out.
func (store dateStoppingJobStore) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	next, due, err := store.FakeJob.NextTask(ctx, now)
	if err != nil || !due {
		return next, due, err
	}
	for _, task := range store.FakeJob.Tasks(next.JobID) {
		if task.TaskID == next.TaskID {
			break
		}
		if task.Done || task.DueAt == "" {
			continue
		}
		if dueAt, parseErr := time.Parse(time.RFC3339, task.DueAt); parseErr == nil && dueAt.After(now) {
			return contract.TaskToRun{}, false, nil
		}
	}
	return next, due, err
}

// isOff says whether the fake lists that job as switched off.
func isOff(ctx context.Context, jobs *testkit.FakeJob, jobID string) bool {
	listed, err := jobs.List(ctx)
	if err != nil {
		return false
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary.State == contract.JobOff
		}
	}
	return false
}

// mutePutDownJobStore refuses a put-down on a finished task without saying
// which task.
type mutePutDownJobStore struct{ *testkit.FakeJob }

// PutDown refuses a finished task with a message that names nothing.
func (store mutePutDownJobStore) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	for _, task := range store.FakeJob.Tasks(mark.Task.JobID) {
		if task.TaskID == mark.Task.TaskID && task.Done {
			return errors.New("that cannot be put down")
		}
	}
	return store.FakeJob.PutDown(ctx, mark)
}

// pausingRefuserJobStore refuses a put-down on a finished task and pauses the
// job anyway, so the refusal is not the whole story.
type pausingRefuserJobStore struct{ *testkit.FakeJob }

// PutDown pauses the job, then refuses the finished task by name.
func (store pausingRefuserJobStore) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	for _, task := range store.FakeJob.Tasks(mark.Task.JobID) {
		if task.TaskID == mark.Task.TaskID && task.Done {
			_ = store.FakeJob.Pause(ctx, mark.Task.JobID)
			return fmt.Errorf("task %s is finished and cannot be put down", mark.Task.TaskID)
		}
	}
	return store.FakeJob.PutDown(ctx, mark)
}

// muteOffPutDownJobStore refuses a put-down on a job that is off without
// saying which job.
type muteOffPutDownJobStore struct{ *testkit.FakeJob }

// PutDown refuses an off job with a message that names nothing.
func (store muteOffPutDownJobStore) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	if isOff(ctx, store.FakeJob, mark.Task.JobID) {
		return errors.New("that cannot be put down")
	}
	return store.FakeJob.PutDown(ctx, mark)
}

// doneRecordJobStore lists a job honestly and hands back a record standing at
// done once every task is done, whether or not the job is running.
type doneRecordJobStore struct{ *testkit.FakeJob }

// Load hands back the record standing at done once every task is done.
func (store doneRecordJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	if eagerClosingJobStore(store).everyTaskIsDone(jobID) {
		record.Header.Status = contract.StatusDone
	}
	return record, err
}

// offForgettingJobStore shows every task of a switched-off job unfinished,
// as if a job that is off took no report.
type offForgettingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with the check marks taken off while the job is
// off.
func (store offForgettingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	if isOff(ctx, store.FakeJob, jobID) {
		for index := range record.Work.Tasks {
			record.Work.Tasks[index].Done, record.Work.Tasks[index].ReportID = false, ""
		}
	}
	return record, err
}

// offReportLosingJobStore marks a switched-off job's task done and lists no
// report for it.
type offReportLosingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with the reports taken off while the job is off.
func (store offReportLosingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	if isOff(ctx, store.FakeJob, jobID) {
		record.Work.Results = nil
	}
	return record, err
}

// TestTheJobCheckCatchesAStoreThatBreaksALifecycleRule holds the check to the
// four rules a review found the fake and the real store keeping differently: a
// put-down on a finished task or an off job is refused, a job closes only
// while it is running, a run-now starts a job again whatever stopped it, and a
// task whose date has not come does not hold up the tasks behind it.
func TestTheJobCheckCatchesAStoreThatBreaksALifecycleRule(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
		says string
	}{
		{"a store that puts a job down on a finished task", finishedPutDownJobStore{workingJobStore()}, "a finished task cannot be put down"},
		{"a store that refuses a finished task without naming it", mutePutDownJobStore{workingJobStore()}, "does not name the task"},
		{"a store that refuses a finished task and pauses the job anyway", pausingRefuserJobStore{workingJobStore()}, "a refused put-down changes nothing"},
		{"a store that puts down a job that is off", offPutDownJobStore{workingJobStore()}, "a job that is off cannot be put down"},
		{"a store that refuses an off job without naming it", muteOffPutDownJobStore{workingJobStore()}, "does not name the job"},
		{"a store that closes a job that is not running", eagerClosingJobStore{workingJobStore()}, "closes only while it is running"},
		{"a store whose record says done while its listing says off", doneRecordJobStore{workingJobStore()}, "want anything but done"},
		{"a store that takes no report while the job is off", offForgettingJobStore{workingJobStore()}, "still takes its running task's report"},
		{"a store that lists no report for a task finished while off", offReportLosingJobStore{workingJobStore()}, "not on the job's list of reports"},
		{"a store that leaves a switched-off job off when it is run now", offStayingJobStore{workingJobStore()}, "whatever stopped it"},
		{"a store that stops at a task whose date has not come", dateStoppingJobStore{workingJobStore()}, "does not hold up the tasks behind it"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := testkit.CheckJob(ctx, test.jobs)
			if err == nil {
				t.Fatal("the job check passed, and it was given a store that breaks a lifecycle rule")
			}
			if !strings.Contains(err.Error(), test.says) {
				t.Errorf("the check failed saying %q, and it must say %q so the store's author knows what to do", err, test.says)
			}
		})
	}
}

// tickKeepingJobStore leaves a scheduled job's next tick where the schedule
// put it when the job is run now, which is what the fake did before the
// run-now rule was pinned.
type tickKeepingJobStore struct {
	*testkit.FakeJob
	nextRuns map[string]time.Time
}

// Create makes the job and remembers where the schedule put its first tick.
func (store *tickKeepingJobStore) Create(ctx context.Context, wanted contract.NewJob) (string, error) {
	jobID, err := store.FakeJob.Create(ctx, wanted)
	if err != nil {
		return jobID, err
	}
	listed, err := store.FakeJob.List(ctx)
	for _, summary := range listed {
		if summary.ID == jobID {
			store.nextRuns[jobID] = summary.NextRun
		}
	}
	return jobID, err
}

// List reports the first tick as the next one, whatever a run-now did.
func (store *tickKeepingJobStore) List(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := store.FakeJob.List(ctx)
	for index, summary := range listed {
		if kept, remembered := store.nextRuns[summary.ID]; remembered {
			listed[index].NextRun = kept
		}
	}
	return listed, err
}

// tickHidingJobStore lists no next run for a job that has a schedule.
type tickHidingJobStore struct{ *testkit.FakeJob }

// List leaves the next run empty on every job.
func (store tickHidingJobStore) List(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := store.FakeJob.List(ctx)
	for index := range listed {
		listed[index].NextRun = time.Time{}
	}
	return listed, err
}

// tickIgnoringJobStore never makes a task from a schedule's template.
type tickIgnoringJobStore struct{ *testkit.FakeJob }

// NextTask hands out nothing in place of a task a schedule made.
func (store tickIgnoringJobStore) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	next, due, err := store.FakeJob.NextTask(ctx, now)
	if due && next.Unattended {
		return contract.TaskToRun{}, false, nil
	}
	return next, due, err
}

// TestTheJobWithoutANameCheckCatchesAStoreThatBreaksTheRunNowRule holds the
// scheduled half of the run-now rule, which lives in the check whose job has a
// schedule: a run-now pulls the next tick to now, and asking for work at that
// moment hands out a task made from the template.
func TestTheJobWithoutANameCheckCatchesAStoreThatBreaksTheRunNowRule(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
		says string
	}{
		{"a store that lists no next run for a scheduled job", tickHidingJobStore{workingJobStore()}, "lists no next run"},
		{"a store that leaves the next tick where the schedule put it", &tickKeepingJobStore{FakeJob: workingJobStore(), nextRuns: map[string]time.Time{}}, "pulled forward to now"},
		{"a store that never makes a task from the template", tickIgnoringJobStore{workingJobStore()}, "made from the template"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := testkit.CheckJobWithoutAName(ctx, test.jobs)
			if err == nil {
				t.Fatal("the job-without-a-name check passed, and it was given a store that breaks the run-now rule")
			}
			if !strings.Contains(err.Error(), test.says) {
				t.Errorf("the check failed saying %q, and it must say %q so the store's author knows what to do", err, test.says)
			}
		})
	}
}
