// The tests for RunNow on a job that is put down on a task. RunNow used to
// forget the mark and take the date off the first unfinished task, which on a
// job put down on its second task was the first, dated for next week: the
// dated task ran first and the put-down task ran after it as a plain task.
// RunNow on a put-down job runs the put-down task first, with its claim let go
// and the mark forgotten, and leaves every other date where it was.
package job_test

import (
	"testing"
	"time"
)

func TestRunNowOnAPutDownJobRunsThePutDownTaskFirstAndKeepsTheOtherDates(t *testing.T) {
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

	if err := holding.jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot run the job now: %v", err)
	}

	if _, there, err := holding.jobs.PutDownTask(ctx); err != nil || there {
		t.Errorf("after run now the mark is still there (there %v, error %v), and a job set running again forgets it", there, err)
	}
	next, due := holding.nextTask(t, theEpoch().Add(time.Second))
	if !due || next.TaskID != second {
		t.Errorf("after run now the next task is %+v (due %v), want %s, the put-down task, run first", next, due, second)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if held.Work.Tasks[0].DueAt == "" {
		t.Errorf("after run now the dated task reads %+v, and its date is gone: run now on a put-down job runs the put-down task, not the dated one", held.Work.Tasks[0])
	}
	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 0 {
		t.Errorf("the old claim was counted as %d failures, want none", failures)
	}
}
