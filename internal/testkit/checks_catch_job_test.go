package testkit_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// workingJobStore is the fake job store the broken ones below are built from.
func workingJobStore() *testkit.FakeJob {
	return testkit.NewFakeJob(testkit.NewFakeClock(zeroTime()))
}

// agreeableJobStore creates a job with no ask at all.
type agreeableJobStore struct{ *testkit.FakeJob }

// Create accepts a job with no ask.
func (agreeableJobStore) Create(context.Context, contract.NewJob) (string, error) { return "1", nil }

// forgivingJobStore pauses a job that does not exist.
type forgivingJobStore struct{ *testkit.FakeJob }

// Pause never says there is no such job.
func (forgivingJobStore) Pause(context.Context, string) error { return nil }

// oddlyNumberedJobStore gives its tasks identifiers of the wrong shape.
type oddlyNumberedJobStore struct{ *testkit.FakeJob }

// AddTask returns an identifier the design does not use.
func (oddlyNumberedJobStore) AddTask(context.Context, contract.NewTask) (string, error) {
	return "task-one", nil
}

// askListingJobStore lists every job by its ask, the way the store did before a
// job had a name, so a job made with a name is listed as if it had none.
type askListingJobStore struct{ *testkit.FakeJob }

// List puts the whole ask where the title goes, name or no name.
func (store askListingJobStore) List(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := store.FakeJob.List(ctx)
	for index, summary := range listed {
		record, loadErr := store.FakeJob.Load(ctx, summary.ID)
		if loadErr == nil {
			listed[index].Title = record.Goal.Ask
		}
	}
	return listed, err
}

// nameDroppingJobStore keeps a job's name for the listing and loses it from the
// record, so the job list and the side panel would disagree about the job.
type nameDroppingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with the name taken off.
func (store nameDroppingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	record.Goal.Name = ""
	return record, err
}

// openEndedJobStore never closes a job: its last task finishes and the listing
// goes on saying the job is running, which is what the real store did before a
// job could close on a done line per task.
type openEndedJobStore struct{ *testkit.FakeJob }

// List reports every job as running, done or not.
func (store openEndedJobStore) List(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := store.FakeJob.List(ctx)
	for index := range listed {
		listed[index].State = contract.JobRunning
	}
	return listed, err
}

// unprovedJobStore closes a job without writing the done list its tasks prove,
// so the record says done and nothing in it says why.
type unprovedJobStore struct{ *testkit.FakeJob }

// Load hands back the record with its done list taken off.
func (store unprovedJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	record.Goal.DoneWhen = nil
	return record, err
}

// forgetfulPutDownStore never remembers which task a job was put down on, the
// way the loop's own memory forgot it across a restart.
type forgetfulPutDownStore struct{ *testkit.FakeJob }

// PutDownTask never has a put-down task to hand back.
func (forgetfulPutDownStore) PutDownTask(context.Context) (contract.PutDownMark, bool, error) {
	return contract.PutDownMark{}, false, nil
}

// clingingPutDownStore keeps the put-down mark after the job is set running
// again, so a task already picked up would be picked up twice.
type clingingPutDownStore struct {
	*testkit.FakeJob
	mark *contract.PutDownMark
}

// PutDown keeps a copy of the mark of its own.
func (store *clingingPutDownStore) PutDown(ctx context.Context, mark contract.PutDownMark) error {
	store.mark = &mark
	return store.FakeJob.PutDown(ctx, mark)
}

// PutDownTask hands the copy back whether or not the job runs again.
func (store *clingingPutDownStore) PutDownTask(context.Context) (contract.PutDownMark, bool, error) {
	if store.mark == nil {
		return contract.PutDownMark{}, false, nil
	}
	return *store.mark, true, nil
}

// claimKeepingJobStore keeps the claim a paused run held when the job is set
// running again, so the task the job was paused on is never handed out again,
// which is what the fake did and the real store did not.
type claimKeepingJobStore struct {
	*testkit.FakeJob
	guard  sync.Mutex
	ranNow bool
}

// RunNow sets the job running again and keeps the claim.
func (store *claimKeepingJobStore) RunNow(ctx context.Context, jobID string) error {
	store.guard.Lock()
	store.ranNow = true
	store.guard.Unlock()
	return store.FakeJob.RunNow(ctx, jobID)
}

// NextTask hands nothing out once the job has been set running again, as a
// store whose task is still claimed would.
func (store *claimKeepingJobStore) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	store.guard.Lock()
	kept := store.ranNow
	store.guard.Unlock()
	if kept {
		return contract.TaskToRun{}, false, nil
	}
	return store.FakeJob.NextTask(ctx, now)
}

// oddlyReportedJobStore gives its reports identifiers of the wrong shape.
type oddlyReportedJobStore struct{ *testkit.FakeJob }

// FinishTask finishes the task and returns an identifier the design does not use.
func (store oddlyReportedJobStore) FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	_, err := store.FakeJob.FinishTask(ctx, jobID, taskID, report, failed)
	return "report-one", err
}

// reportLosingJobStore writes a report and then does not list it on the record.
type reportLosingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with its reports taken off.
func (store reportLosingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	record.Work.Results = nil
	return record, err
}

// progressLosingJobStore counts no task as done in the record's header,
// however many have finished.
type progressLosingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with the progress line saying nothing is done.
func (store progressLosingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	record.Header.TasksDone = 0
	return record, err
}

// generousPickUpStore lets a job pick a task up itself as often as it likes,
// where the promise is once.
type generousPickUpStore struct{ *testkit.FakeJob }

// PickUpOnce answers yes every time.
func (store generousPickUpStore) PickUpOnce(ctx context.Context, jobID string, taskID string) (bool, error) {
	_, err := store.FakeJob.PickUpOnce(ctx, jobID, taskID)
	return true, err
}

func TestTheJobCheckCatchesAStoreThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
	}{
		{"a store whose report identifiers are the wrong shape", oddlyReportedJobStore{workingJobStore()}},
		{"a store that writes a report and does not list it", reportLosingJobStore{workingJobStore()}},
		{"a store whose progress line counts no task as done", progressLosingJobStore{workingJobStore()}},
		{"a store that forgets which task a job was put down on", forgetfulPutDownStore{workingJobStore()}},
		{"a store that keeps the put-down mark after the job runs again", &clingingPutDownStore{FakeJob: workingJobStore()}},
		{"a store that keeps the claim a paused run held after the job runs again", &claimKeepingJobStore{FakeJob: workingJobStore()}},
		{"a store that lets a job pick a task up itself again and again", generousPickUpStore{workingJobStore()}},
		{"a store that creates a job with no ask", agreeableJobStore{workingJobStore()}},
		{"a store that pauses a job that is not there", forgivingJobStore{workingJobStore()}},
		{"a store whose task identifiers are the wrong shape", oddlyNumberedJobStore{workingJobStore()}},
		{"a store that lists a named job by its ask", askListingJobStore{workingJobStore()}},
		{"a store that loads a named job with its name dropped", nameDroppingJobStore{workingJobStore()}},
		{"a store that never closes a job whose last task is done", openEndedJobStore{workingJobStore()}},
		{"a store that closes a job with no done line behind it", unprovedJobStore{workingJobStore()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckJob(ctx, test.jobs); err == nil {
				t.Fatal("the job check passed, and it was given a store that breaks a promise")
			}
		})
	}
}

// commaRefusingJobStore refuses a task whose text carries a comma and a space,
// the way the real store did while a job task's due date was written after the
// last comma of its line: every comma in a task's text read back as a date, and
// the read-back check refused the record. The fake accepted anything, so the
// whole job path was proved only on the fake, and this is the store that would
// have shown the drift.
type commaRefusingJobStore struct{ *testkit.FakeJob }

// AddTask refuses a text with a comma and a space in it.
func (store commaRefusingJobStore) AddTask(ctx context.Context, wanted contract.NewTask) (string, error) {
	if strings.Contains(wanted.Text, ", ") {
		return "", fmt.Errorf("the task %q reads back as a task with a due date, so rephrase it without the comma", wanted.Text)
	}
	return store.FakeJob.AddTask(ctx, wanted)
}

// commaCuttingJobStore keeps only what comes before the first comma of a task's
// text, so the job lists a task that is not the one that was written.
type commaCuttingJobStore struct{ *testkit.FakeJob }

// AddTask drops everything from the comma on.
func (store commaCuttingJobStore) AddTask(ctx context.Context, wanted contract.NewTask) (string, error) {
	wanted.Text, _, _ = strings.Cut(wanted.Text, ", ")
	return store.FakeJob.AddTask(ctx, wanted)
}

// taskDroppingJobStore lists only the first task of a job, so a task that was
// added is not on the record it loads.
type taskDroppingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with every task but the first taken off.
func (store taskDroppingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	if len(record.Work.Tasks) > 1 {
		record.Work.Tasks = record.Work.Tasks[:1]
	}
	return record, err
}

// dateLosingJobStore loses the date a task was given, so a task that was to
// wait for a moment reads back as one that need not.
type dateLosingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with every date taken off.
func (store dateLosingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	for index := range record.Work.Tasks {
		record.Work.Tasks[index].DueAt = ""
	}
	return record, err
}

// dateInventingJobStore shows a date on a task that was given none, which is
// what a store that reads a date out of the task's own words does.
type dateInventingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with a date on every task that had none.
func (store dateInventingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	for index := range record.Work.Tasks {
		if record.Work.Tasks[index].DueAt == "" {
			record.Work.Tasks[index].DueAt = "today"
		}
	}
	return record, err
}

// doneMarkLosingJobStore counts a finished task in the progress line and shows
// it unfinished on the list, with no report behind it.
type doneMarkLosingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with every task's check mark and report taken off.
func (store doneMarkLosingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	for index := range record.Work.Tasks {
		record.Work.Tasks[index].Done, record.Work.Tasks[index].ReportID = false, ""
	}
	return record, err
}

// handoutCuttingJobStore keeps a task's text whole on the list and hands the
// task out to run with the text cut at its first comma.
type handoutCuttingJobStore struct{ *testkit.FakeJob }

// NextTask hands out the task with everything from the comma on dropped.
func (store handoutCuttingJobStore) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	next, due, err := store.FakeJob.NextTask(ctx, now)
	next.Text, _, _ = strings.Cut(next.Text, ", ")
	return next, due, err
}

// dateMisreadingJobStore reads past a task whose date has come as if it had
// not, and hands out the undated task behind it first.
type dateMisreadingJobStore struct{ *testkit.FakeJob }

// NextTask hands out the next task instead whenever the fake hands out one
// that has a date.
func (store dateMisreadingJobStore) NextTask(ctx context.Context, now time.Time) (contract.TaskToRun, bool, error) {
	next, due, err := store.FakeJob.NextTask(ctx, now)
	if err != nil || !due {
		return next, due, err
	}
	for _, task := range store.FakeJob.Tasks(next.JobID) {
		if task.TaskID == next.TaskID && task.DueAt != "" {
			return store.FakeJob.NextTask(ctx, now)
		}
	}
	return next, due, err
}

// dateRewritingJobStore shows a task's date differently on every read, so the
// date the person saw when they wrote the task is not the one they see later.
type dateRewritingJobStore struct {
	*testkit.FakeJob
	reads int
}

// Load hands back the record with the number of the read written into every
// date.
func (store *dateRewritingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	store.reads++
	for index := range record.Work.Tasks {
		if record.Work.Tasks[index].DueAt != "" {
			record.Work.Tasks[index].DueAt = fmt.Sprintf("%s (read %d)", record.Work.Tasks[index].DueAt, store.reads)
		}
	}
	return record, err
}

// wordlessDoneListJobStore closes a job on done lines that point at the right
// reports and say nothing, so the person cannot read what was proved.
type wordlessDoneListJobStore struct{ *testkit.FakeJob }

// Load hands back the record with the words taken off every done line.
func (store wordlessDoneListJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	for index := range record.Goal.DoneWhen {
		record.Goal.DoneWhen[index].Text = ""
	}
	return record, err
}

// TestTheJobCheckCatchesAStoreThatCannotKeepProse holds the check to the
// promise a live run found missing: a task's text is prose, and a store that
// refuses its punctuation, loses part of it, takes part of it for a date, or
// loses the date beside it fails the check with a message that says which and
// what to do.
func TestTheJobCheckCatchesAStoreThatCannotKeepProse(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
		says string
	}{
		{"a store that refuses a comma in a task's text", commaRefusingJobStore{workingJobStore()}, "punctuation"},
		{"a store that drops everything after the comma", commaCuttingJobStore{workingJobStore()}, "byte for byte"},
		{"a store that lists only the first task", taskDroppingJobStore{workingJobStore()}, "lists no task"},
		{"a store that loses a task's date", dateLosingJobStore{workingJobStore()}, "the date was lost"},
		{"a store that reads a date out of a task's words", dateInventingJobStore{workingJobStore()}, "taken for a date"},
		{"a store that shows a finished task unfinished", doneMarkLosingJobStore{workingJobStore()}, "marked done against its report"},
		{"a store that hands a task out with its text cut", handoutCuttingJobStore{workingJobStore()}, "handed out reading"},
		{"a store that reads past a task whose date has come", dateMisreadingJobStore{workingJobStore()}, "whose date has come"},
		{"a store that shows a date differently on every read", &dateRewritingJobStore{FakeJob: workingJobStore()}, "written once"},
		{"a store that closes a job on done lines with no words", wordlessDoneListJobStore{workingJobStore()}, "in the task's own words"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := testkit.CheckJob(ctx, test.jobs)
			if err == nil {
				t.Fatal("the job check passed, and it was given a store that cannot keep a task's text as it was written")
			}
			if !strings.Contains(err.Error(), test.says) {
				t.Errorf("the check failed saying %q, and it must say %q so the store's author knows what to do", err, test.says)
			}
		})
	}
}

// untitledJobStore lists a job made without a name under no title at all,
// instead of falling back to its ask.
type untitledJobStore struct{ *testkit.FakeJob }

// List leaves the title empty on every job that has no name.
func (store untitledJobStore) List(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := store.FakeJob.List(ctx)
	for index, summary := range listed {
		record, loadErr := store.FakeJob.Load(ctx, summary.ID)
		if loadErr == nil && record.Goal.Name == "" {
			listed[index].Title = ""
		}
	}
	return listed, err
}

// askNamingJobStore makes a name up for a job that was given none, by writing
// the ask into the record where the name goes.
type askNamingJobStore struct{ *testkit.FakeJob }

// Load hands back the record with the ask in place of the missing name.
func (store askNamingJobStore) Load(ctx context.Context, jobID string) (contract.Record, error) {
	record, err := store.FakeJob.Load(ctx, jobID)
	if record.Goal.Name == "" {
		record.Goal.Name = record.Goal.Ask
	}
	return record, err
}

func TestTheJobWithoutANameCheckCatchesAStoreThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
	}{
		{"a store that lists a job with no name under no title", untitledJobStore{workingJobStore()}},
		{"a store that makes a name up for a job given none", askNamingJobStore{workingJobStore()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckJobWithoutAName(ctx, test.jobs); err == nil {
				t.Fatal("the job-without-a-name check passed, and it was given a store that breaks a promise")
			}
		})
	}
}

// zeroTime is the moment every fake clock in these tests starts from.
func zeroTime() time.Time {
	return time.Unix(0, 0).UTC()
}
