// The tests for Resume, the one way of setting a put-down job running again
// that touches nothing else. The loop's carry-on used to go through RunNow,
// which takes the date off the first unfinished task and fires a schedule's
// tick at once, so a "continue" on the job's second task erased the date the
// person had put on its first, and a scheduled job ran one time more than its
// schedule said.
package job_test

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theRunThePersonStopped is the number the loop ran the put-down task under.
const theRunThePersonStopped = "5"

// putDownOn puts the job down on the task the store has just handed out, the
// way the loop does when the person stops it.
func (holding *opened) putDownOn(t *testing.T, handed contract.TaskToRun) contract.PutDownMark {
	t.Helper()
	mark := contract.PutDownMark{Task: handed, Run: theRunThePersonStopped, HasRecord: true}
	if err := holding.jobs.PutDown(t.Context(), mark); err != nil {
		t.Fatalf("cannot put job %s down on task %s: %v", handed.JobID, handed.TaskID, err)
	}
	return mark
}

func TestResumeKeepsTheDateOnATaskThatWaitsForOne(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Run the campaign this month.")
	holding.aTask(t, jobID, "post on the anniversary itself", theEpoch().Add(7*24*time.Hour))
	second := holding.aTask(t, jobID, "draft the blog piece", time.Time{})
	handed, due := holding.nextTask(t, theEpoch())
	if !due || handed.TaskID != second {
		t.Fatalf("the task handed out is %+v (due %v), want %s, the undated one behind the dated one", handed, due, second)
	}
	holding.putDownOn(t, handed)

	if err := holding.jobs.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot resume the job: %v", err)
	}

	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if held.Work.Tasks[0].DueAt == "" {
		t.Errorf("after the resume the dated task reads %+v, and its date is gone", held.Work.Tasks[0])
	}
	if summary := holding.summaryOf(t, jobID); summary.State != contract.JobRunning {
		t.Errorf("after the resume the job is %q, want %q", summary.State, contract.JobRunning)
	}
	if _, there, err := holding.jobs.PutDownTask(ctx); err != nil || there {
		t.Errorf("after the resume the mark is still there (there %v, error %v), and a job set running again forgets it", there, err)
	}
	next, due := holding.nextTask(t, theEpoch().Add(time.Second))
	if !due || next.TaskID != second {
		t.Errorf("after the resume the next task is %+v (due %v), want %s, the put-down task, with the dated one still waiting", next, due, second)
	}
}

func TestResumeFiresNoTickOfAScheduledJob(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aScheduledJob(t, "Post every hour.", contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	taskID := holding.aTask(t, jobID, "post the first message by hand", time.Time{})
	before := holding.summaryOf(t, jobID).NextRun
	if before.IsZero() {
		t.Fatal("a scheduled job was made with no next run")
	}
	handed, due := holding.nextTask(t, theEpoch())
	if !due || handed.TaskID != taskID {
		t.Fatalf("the task handed out is %+v (due %v), want %s", handed, due, taskID)
	}
	holding.putDownOn(t, handed)

	if err := holding.jobs.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot resume the job: %v", err)
	}

	if after := holding.summaryOf(t, jobID).NextRun; !after.Equal(before) {
		t.Errorf("after the resume the next run is %s, want %s, unchanged: a resume fires no tick", after, before)
	}
	next, due := holding.nextTask(t, theEpoch().Add(time.Second))
	if !due || next.TaskID != taskID {
		t.Errorf("after the resume the next task is %+v (due %v), want %s, the put-down task, and no task a tick made", next, due, taskID)
	}
	if total := holding.summaryOf(t, jobID).TasksTotal; total != 1 {
		t.Errorf("after the resume the job holds %d tasks, want the one it had: no tick fired", total)
	}
}

func TestResumeRefusesAJobThatIsNotThere(t *testing.T) {
	holding := newJobs(t)

	if err := holding.jobs.Resume(t.Context(), "99"); err == nil {
		t.Error("a job that is not there was resumed without an error naming it")
	}
}
