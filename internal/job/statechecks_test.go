// The tests for the state a job must be in for two of the store's operations.
// A put-down used to pause a job whatever state it was in, so "/cron off 2"
// while its task ran, followed by Escape, turned the switched-off job into a
// paused one with a mark that "continue" then resumed; and a job closed as
// done the moment its last task finished whether or not the job was running,
// so a job the person had switched off, or that had paused itself on
// failures, closed under them.
package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestPutDownRefusesAJobThatIsSwitchedOff(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	holding.aTask(t, jobID, "the task that was running", time.Time{})
	handed, due := holding.nextTask(t, theEpoch())
	if !due {
		t.Fatal("the task was not handed out to begin with")
	}
	if err := holding.jobs.SwitchOff(ctx, jobID); err != nil {
		t.Fatalf("cannot switch the job off: %v", err)
	}

	err := holding.jobs.PutDown(ctx, contract.PutDownMark{Task: handed, Run: theRunThePersonStopped, HasRecord: true})

	if err == nil || !strings.Contains(err.Error(), "switched off") {
		t.Errorf("putting down a task of a switched-off job gave %v, want a refusal naming the job as switched off", err)
	}
	if state := holding.summaryOf(t, jobID).State; state != contract.JobOff {
		t.Errorf("after the refused put-down the job is %q, want it still %q", state, contract.JobOff)
	}
	if _, there, _ := holding.jobs.PutDownTask(ctx); there {
		t.Error("a refused put-down left a mark behind, and continue would resume a job the person switched off")
	}
}

func TestPutDownRefusesATaskThatIsFinished(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do two things.")
	holding.aTask(t, jobID, "the task that finished", time.Time{})
	holding.aTask(t, jobID, "the task after it", time.Time{})
	handed, due := holding.nextTask(t, theEpoch())
	if !due {
		t.Fatal("the task was not handed out to begin with")
	}
	holding.finish(t, jobID, handed.TaskID, "it is done", false)

	err := holding.jobs.PutDown(ctx, contract.PutDownMark{Task: handed, Run: theRunThePersonStopped, HasRecord: true})

	if err == nil || !strings.Contains(err.Error(), "finished") {
		t.Errorf("putting down a finished task gave %v, want a refusal naming the task as finished", err)
	}
	if state := holding.summaryOf(t, jobID).State; state != contract.JobRunning {
		t.Errorf("after the refused put-down the job is %q, want it still %q", state, contract.JobRunning)
	}
}

func TestAJobThatIsNotRunningDoesNotCloseWhenItsLastTaskFinishes(t *testing.T) {
	ctx := t.Context()
	for _, stopping := range []struct {
		name   string
		stop   func(*opened, string) error
		state  contract.JobState
		status contract.RecordStatus
	}{
		{"switched off", func(holding *opened, jobID string) error { return holding.jobs.SwitchOff(ctx, jobID) },
			contract.JobOff, contract.StatusStopped},
		{"paused", func(holding *opened, jobID string) error { return holding.jobs.Pause(ctx, jobID) },
			contract.JobPaused, contract.StatusWaiting},
	} {
		t.Run(stopping.name, func(t *testing.T) {
			holding := newJobs(t)
			jobID := holding.aJob(t, "Do one thing.")
			taskID := holding.aTask(t, jobID, "the one task", time.Time{})
			if _, due := holding.nextTask(t, theEpoch()); !due {
				t.Fatal("the task was not handed out to begin with")
			}
			if err := stopping.stop(holding, jobID); err != nil {
				t.Fatalf("cannot stop the job: %v", err)
			}

			reportID := holding.finish(t, jobID, taskID, "the one thing is done", false)

			held, err := holding.jobs.Load(ctx, jobID)
			if err != nil {
				t.Fatalf("cannot load the job: %v", err)
			}
			if !held.Work.Tasks[0].Done || held.Work.Tasks[0].ReportID != reportID {
				t.Errorf("the finished task reads %+v, want it done with report %s behind it", held.Work.Tasks[0], reportID)
			}
			if len(held.Work.Results) != 1 {
				t.Errorf("the job holds %d reports, want the one the task wrote", len(held.Work.Results))
			}
			if state := holding.summaryOf(t, jobID).State; state != stopping.state {
				t.Errorf("a %s job whose last task finished is %q, want it still %q: a job closes only while it is running", stopping.name, state, stopping.state)
			}
			if held.Header.Status != stopping.status {
				t.Errorf("the %s job's record stands at %q, want %q", stopping.name, held.Header.Status, stopping.status)
			}
		})
	}
}

func TestAPausedJobSetRunningAgainWithEveryTaskDoneClosesThen(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do one thing.")
	taskID := holding.aTask(t, jobID, "the one task", time.Time{})
	if _, due := holding.nextTask(t, theEpoch()); !due {
		t.Fatal("the task was not handed out to begin with")
	}
	if err := holding.jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job: %v", err)
	}
	holding.finish(t, jobID, taskID, "the one thing is done", false)

	if err := holding.jobs.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot resume the job: %v", err)
	}

	if state := holding.summaryOf(t, jobID).State; state != contract.JobDone {
		t.Errorf("a job set running again with every task done is %q, want %q, because there is nothing left for it to run", state, contract.JobDone)
	}
}
