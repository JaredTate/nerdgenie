// The tests for how a job's tasks share the turn. The loop used to chain up
// to a hundred immediately-due tasks of a job inside one call, so the one
// turn deadline the driver set covered the whole chain rather than each task;
// and when that deadline cut a task off, the job's bookkeeping ran under the
// cancelled context, failed with "context canceled", and left the task
// claimed for an hour. Each call runs one task, and the bookkeeping runs
// under a short context of its own.
package loop_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

// TestEachOfAJobsTasksRunsInACallOfItsOwn proves the seam: one call runs one
// task and reports it, the next task waits for the next call, and the job
// closes on the call that runs its last task.
func TestEachOfAJobsTasksRunsInACallOfItsOwn(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	ran, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil || !ran {
		t.Fatalf("the first call ran nothing (ran %v, error %v), and the job has two tasks waiting", ran, err)
	}

	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the person was sent %v, want the first task's report", built.channel.Sent())
	}
	if sentSomethingLike(built.channel.Sent(), "Job "+jobID+" is finished") {
		t.Errorf("the person was sent %v after one call, and each call runs one task so that each runs under a turn of its own", built.channel.Sent())
	}
	if tasks := built.jobs.Tasks(jobID); len(tasks) != 2 || !tasks[0].Done || tasks[1].Done {
		t.Errorf("after one call the tasks read %+v, want the first done and the second still to run", tasks)
	}
	if ran := runTheJobToTheEnd(t, built.loop, built.channel); ran != 1 {
		t.Errorf("the rest of the job took %d calls, want one for its one task left", ran)
	}
	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+" is finished") {
		t.Errorf("the person was sent %v, want the job closed on the call that ran its last task", built.channel.Sent())
	}
}

// jobThatHonoursTheContext is the fake job store with the one thing the real
// one has and the fake does not: a call made under a cancelled context fails
// with that context's error, the way a database call does.
type jobThatHonoursTheContext struct {
	contract.Job
}

// FinishTask refuses under a cancelled context and passes the call on otherwise.
func (jobs jobThatHonoursTheContext) FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return jobs.Job.FinishTask(ctx, jobID, taskID, report, failed)
}

// Load refuses under a cancelled context and passes the call on otherwise.
func (jobs jobThatHonoursTheContext) Load(ctx context.Context, jobID string) (contract.Record, error) {
	if err := ctx.Err(); err != nil {
		return contract.Record{}, err
	}
	return jobs.Job.Load(ctx, jobID)
}

// TestATaskCutOffByTheTurnDeadlineIsPutDownWithItsClaimLetGo cuts a job's
// task off in the middle of its model call, the way the turn deadline and a
// shutdown do, and asks that the job's bookkeeping still land: the task is
// put down as the stop the program made, no failure is counted, the person is
// told the next message picks it up, and once the job runs again the task is
// handed out rather than sitting claimed for an hour. It used to be finished
// as a failure and run again from the top, which is how the 20:33 restart of
// the fifth game build abandoned ninety rounds of a play-test task's record.
func TestATaskCutOffByTheTurnDeadlineIsPutDownWithItsClaimLetGo(t *testing.T) {
	built := newHarness(t, nil)
	jobID := aJobOfTwoTasks(t, built)
	waiting := aModelThatWaitsOn(1, built.model)
	options := built.optionsOver(waiting)
	options.Jobs = jobThatHonoursTheContext{Job: built.jobs}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over the store that honours the context: %v", err)
	}
	turn, cutOff := context.WithCancel(context.Background())
	defer cutOff()
	ended := make(chan error, 1)
	go func() {
		_, err := made.RunNextJobTask(turn, built.channel)
		ended <- err
	}()
	<-waiting.calling

	cutOff()

	select {
	case err := <-ended:
		if err != nil {
			t.Fatalf("the cut-off task came back with an error, want it finished as failed with the bookkeeping done: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the loop never came back after the turn was cut off")
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobPaused || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it paused on the cut-off task with no failure counted", summary)
	}
	if putDown, there, err := built.jobs.PutDownTask(t.Context()); err != nil || !there || putDown.Task.TaskID != "t1" {
		t.Errorf("the put-down mark reads %+v (there %v, error %v), want the job's first task, so the next message picks it up", putDown, there, err)
	}
	if !sentSomethingLike(built.channel.Sent(), "Your next message picks this task up") {
		t.Errorf("the person was sent %v, want the stopped report with the line that says the next message picks the task up", built.channel.Sent())
	}
	if !strings.Contains(strings.Join(built.channel.Sent(), "\n"), "cut off") {
		t.Errorf("the person was sent %v, want the report to say the program cut the task off", built.channel.Sent())
	}
	if first := theJobsFirstTask(t, built, jobID); first.Done {
		t.Errorf("the task reads %+v, want it still to do", first)
	}
	if err := built.jobs.Resume(t.Context(), jobID); err != nil {
		t.Fatalf("cannot set the job running again: %v", err)
	}
	next, there, err := built.jobs.NextTask(t.Context(), built.clock.Now())
	if err != nil || !there || next.TaskID != "t1" {
		t.Errorf("the next task is %+v (there %v, error %v), want t1 handed out again with its claim let go once the job runs again", next, there, err)
	}
}
