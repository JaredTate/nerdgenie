// The tests for what opening the store does with a dead process's claim on a
// task whose run left a record. A restart in the middle of a job's task, with
// no stop before it, left two stories: the program marked the task's record
// interrupted and invited "continue", while the store released the claim and
// the driver ran the same task again from the top, so "continue" resumed the
// old run as a plain task beside it and the work was done twice. The claim
// becomes a put-down mark on open, so both read the same way. These tests run
// in the package, because taking a claim under another process's name is not
// something the store lets anyone outside it do.
package job

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theRunTheDeadProcessRanTheTaskUnder is the number the loop in the process
// that died ran the job's task under.
const theRunTheDeadProcessRanTheTaskUnder = "7"

// theDeadProcessRanTheTask writes into the log what the loop writes when it
// runs a job's task: the ask under the run's number, carrying the job and the
// task as its id, and, when the run got as far as a tool call, the record's
// first checkpoint under the same number.
func theDeadProcessRanTheTask(t *testing.T, jobs *Jobs, jobID string, taskID string, text string, madeARecord bool) {
	t.Helper()
	ctx := t.Context()
	asked, err := json.Marshal(contract.Inbound{ID: jobID + "." + taskID, Text: text, Channel: "terminal"})
	if err != nil {
		t.Fatalf("cannot write the ask as JSON: %v", err)
	}
	_, err = jobs.eventLog.Append(ctx, contract.Event{
		TaskID: theRunTheDeadProcessRanTheTaskUnder, Kind: contract.EventMessage, Body: asked,
	})
	if err != nil {
		t.Fatalf("cannot write the ask of the dead process's run into the log: %v", err)
	}
	if !madeARecord {
		return
	}
	_, err = record.New(ctx, jobs.eventLog, record.Start{
		Kind: contract.RecordTask, ID: theRunTheDeadProcessRanTheTaskUnder, Ask: text, NoRoundBudget: true, NoTimeBudget: true,
	})
	if err != nil {
		t.Fatalf("cannot write the record of the dead process's run into the log: %v", err)
	}
}

// TestOpeningTheStorePutsAJobDownOnATaskADeadProcessLeftARecordOf is the
// restart in the middle of a task with no stop before it: the dead process's
// claim becomes a put-down mark on the task, so that "continue" picks it up
// under the job from its record, and the driver does not run the same task
// again from the top beside it.
func TestOpeningTheStorePutsAJobDownOnATaskADeadProcessLeftARecordOf(t *testing.T) {
	home := testkit.NewTempHome(t)
	ctx := context.Background()
	jobs, jobID, taskID := aJobWithOneTaskClaimedBy(t, home, aProcessNumberNoProcessCanHave)
	theDeadProcessRanTheTask(t, jobs, jobID, taskID, "the task the old process was running", true)

	reopened := openInThePackage(t, home)

	mark, there, err := reopened.PutDownTask(ctx)
	if err != nil || !there {
		t.Fatalf("after the restart the store holds no put-down task (there %v, error %v), and the dead process left a record of the task", there, err)
	}
	wanted := contract.PutDownMark{
		Task:      contract.TaskToRun{JobID: jobID, TaskID: taskID, Text: "the task the old process was running"},
		Run:       theRunTheDeadProcessRanTheTaskUnder,
		HasRecord: true,
	}
	if mark != wanted {
		t.Errorf("after the restart the put-down task is %+v, want %+v: the run the dead process made, with its record, stopped rather than asking", mark, wanted)
	}
	if state := summaryInThePackage(t, reopened, jobID).State; state != contract.JobPaused {
		t.Errorf("after the restart the job is %q, want it %q on the task the dead process was running", state, contract.JobPaused)
	}
	if next, due, err := reopened.NextTask(ctx, theMomentTheseTestsStart.Add(time.Second)); err != nil || due {
		t.Errorf("the reopened store handed out %+v (due %v, error %v), and the task is the person's to pick up, not the driver's to run again", next, due, err)
	}
	if err := reopened.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot set the job running again: %v", err)
	}
	if next, due, err := reopened.NextTask(ctx, theMomentTheseTestsStart.Add(2*time.Second)); err != nil || !due || next.TaskID != taskID {
		t.Errorf("once the job runs again the next task is %+v (due %v, error %v), want %s at once", next, due, err, taskID)
	}
}

// TestOpeningTheStoreLeavesATaskADeadProcessLeftNoRecordOfToRunAgain pins the
// other half of the rule: a run that never got as far as a tool call left
// nothing to pick up, so the task is simply run again.
func TestOpeningTheStoreLeavesATaskADeadProcessLeftNoRecordOfToRunAgain(t *testing.T) {
	home := testkit.NewTempHome(t)
	ctx := context.Background()
	jobs, jobID, taskID := aJobWithOneTaskClaimedBy(t, home, aProcessNumberNoProcessCanHave)
	theDeadProcessRanTheTask(t, jobs, jobID, taskID, "the task the old process was running", false)

	reopened := openInThePackage(t, home)

	if _, there, err := reopened.PutDownTask(ctx); err != nil || there {
		t.Errorf("after the restart the store holds a put-down task (there %v, error %v), and the dead process left no record to pick up", there, err)
	}
	next, due, err := reopened.NextTask(ctx, theMomentTheseTestsStart.Add(time.Second))
	if err != nil || !due || next.TaskID != taskID {
		t.Errorf("after the restart the next task is %+v (due %v, error %v), want %s run again from the top", next, due, err, taskID)
	}
}

// TestOpeningTheStoreLeavesASchedulesTaskADeadProcessHeldToRunAgain pins the
// rule for a task a schedule made: nobody is there to say continue to it, so
// pausing the job on it would stop the schedule for good, and the task is run
// again instead, as it was before.
func TestOpeningTheStoreLeavesASchedulesTaskADeadProcessHeldToRunAgain(t *testing.T) {
	home := testkit.NewTempHome(t)
	ctx := context.Background()
	jobs := openInThePackage(t, home)
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask: "Post every hour.", Why: "because the user asked",
		Schedule: &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour}, TaskTemplate: "post this hour's message",
	})
	if err != nil {
		t.Fatalf("cannot create the scheduled job: %v", err)
	}
	jobs.owner = aProcessNumberNoProcessCanHave
	ticked, due, err := jobs.NextTask(ctx, theMomentTheseTestsStart.Add(time.Hour))
	if err != nil || !due {
		t.Fatalf("the schedule's tick made no task (due %v, error %v)", due, err)
	}
	theDeadProcessRanTheTask(t, jobs, jobID, ticked.TaskID, ticked.Text, true)

	reopened := openInThePackage(t, home)

	if _, there, err := reopened.PutDownTask(ctx); err != nil || there {
		t.Errorf("after the restart the store holds a put-down task (there %v, error %v), and a schedule's task has nobody to pick it up", there, err)
	}
	next, due, err := reopened.NextTask(ctx, theMomentTheseTestsStart.Add(time.Hour+time.Second))
	if err != nil || !due || next.TaskID != ticked.TaskID || !next.Unattended {
		t.Errorf("after the restart the next task is %+v (due %v, error %v), want the schedule's task %s run again", next, due, err, ticked.TaskID)
	}
}

// TestAProcessNobodyMaySignalIsReadAsRunning pins the conservative reading of
// the kernel's answer: process 1 is there and is not ours to signal, and a
// claim under it is left alone rather than taken as a dead process's.
func TestAProcessNobodyMaySignalIsReadAsRunning(t *testing.T) {
	if !processIsRunning(ownerPrefix + "1") {
		t.Error("process 1 was read as gone, and a process that refuses the signal is running")
	}
}
