package testkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The three tasks the lifecycle steps of CheckJob add after the prose ones: one
// dated for a day the check never reaches, one undated behind it, and one added
// while the job is switched off.
const (
	theWaitingTasksText        = "post for day three"
	theTaskBehindTheWaitingOne = "draft the blog piece"
	theTaskAddedWhileOff       = "write the summary for the user"
)

// theTimeATaskWaits is how far ahead the waiting task is dated: two days, which
// is past every moment the check asks for work at, so that its date never
// comes on its own and only a run-now brings it forward.
const theTimeATaskWaits = 48 * time.Hour

// theRunTheContractCheckPutsDownAgain is the run number of the put-down the
// check expects to be refused, which is later than the one it was allowed.
const theRunTheContractCheckPutsDownAgain = "8"

// checkJobRefusesAPutDownOnAFinishedTask holds a store to the first half of
// the put-down rule: a task that has finished cannot be put down, the refusal
// names the task, and the job stands as it was. A store that takes the mark
// would pause a running job on a task nobody can pick up.
func checkJobRefusesAPutDownOnAFinishedTask(ctx context.Context, jobs contract.Job, under theJobUnderCheck) error {
	mark := contract.PutDownMark{Task: contract.TaskToRun{JobID: under.jobID, TaskID: under.plain}, Run: theRunTheContractCheckPutsDownAgain}
	err := jobs.PutDown(ctx, mark)
	if err == nil {
		return fmt.Errorf("putting the job down on task %s, which has finished, returned no error; a finished task cannot be put down, and the refusal must name it", under.plain)
	}
	if !strings.Contains(err.Error(), under.plain) {
		return fmt.Errorf("the refusal to put the job down on the finished task %s reads %q and does not name the task", under.plain, err)
	}
	if state, err := stateOfJob(ctx, jobs, under.jobID); err != nil || state != contract.JobRunning {
		return fmt.Errorf("after a put-down was refused the job is %q (error %v), want %q, because a refused put-down changes nothing", state, err, contract.JobRunning)
	}
	return nil
}

// addTheWaitingTasks puts two more tasks on the job's list while the prose
// tasks are still to run: one dated for a day the check never reaches, and one
// undated behind it. They go on before the prose tasks finish, because a job
// closes the moment its last task does, and they are what the next-task rule
// and the lifecycle steps after it are held on.
func addTheWaitingTasks(ctx context.Context, jobs contract.Job, under *theJobUnderCheck) error {
	waiting, err := jobs.AddTask(ctx, contract.NewTask{JobID: under.jobID, Text: theWaitingTasksText, DueAt: under.now.Add(theTimeATaskWaits)})
	if err != nil {
		return fmt.Errorf("adding the task dated two days on failed: %w", err)
	}
	behind, err := jobs.AddTask(ctx, contract.NewTask{JobID: under.jobID, Text: theTaskBehindTheWaitingOne})
	if err != nil {
		return fmt.Errorf("adding the task behind the one that waits failed: %w", err)
	}
	under.waiting, under.behind = waiting, behind
	return nil
}

// checkJobReadsPastATaskThatWaits holds a store to the next-task rule: with
// the prose tasks finished, the task dated for a day that has not come does not
// hold up the undated task behind it, which is handed out and finished. A
// store that stops at the dated task leaves a tick's task added today sitting
// behind a task dated for the end of the month.
func checkJobReadsPastATaskThatWaits(ctx context.Context, jobs contract.Job, under theJobUnderCheck) error {
	next, due, err := jobs.NextTask(ctx, under.afterThePutDownSteps())
	if err != nil {
		return fmt.Errorf("asking for the next task behind one that waits failed: %w", err)
	}
	if !due || next.TaskID != under.behind {
		return fmt.Errorf("with task %s dated two days on and task %s undated behind it, the next task is %+v (due %v), want %s: a task whose date has not come does not hold up the tasks behind it", under.waiting, under.behind, next, due, under.behind)
	}
	if _, err := jobs.FinishTask(ctx, under.jobID, under.behind, "the contract check finished it", false); err != nil {
		return fmt.Errorf("finishing the task %s failed: %w", under.behind, err)
	}
	return nil
}

// checkAJobThatIsNotRunningDoesNotClose holds a store to two rules at once on
// the job switched off: a job that is off cannot be put down, and the refusal
// names the job; and its last task finishing writes the report and marks the
// task done but does not close the job, which stays off. Then a task added
// while the job is off runs once the job is run now, so that the close step
// after this one still has a last task to close on. A store that closes a job
// that is not running says done on a job the person had stopped.
func checkAJobThatIsNotRunningDoesNotClose(ctx context.Context, jobs contract.Job, under theJobUnderCheck) error {
	if err := jobs.SwitchOff(ctx, under.jobID); err != nil {
		return fmt.Errorf("switching the job off failed: %w", err)
	}
	mark := contract.PutDownMark{Task: contract.TaskToRun{JobID: under.jobID, TaskID: under.waiting}, Run: theRunTheContractCheckPutsDownAgain}
	err := jobs.PutDown(ctx, mark)
	if err == nil {
		return fmt.Errorf("putting down job %s, which is switched off, returned no error; a job that is off cannot be put down, and the refusal must name the job", under.jobID)
	}
	if !strings.Contains(err.Error(), under.jobID) {
		return fmt.Errorf("the refusal to put down the switched-off job %s reads %q and does not name the job", under.jobID, err)
	}
	if err := checkTheLastTaskOfAJobThatIsOffFinishes(ctx, jobs, under); err != nil {
		return err
	}

	added, err := jobs.AddTask(ctx, contract.NewTask{JobID: under.jobID, Text: theTaskAddedWhileOff})
	if err != nil {
		return fmt.Errorf("adding a task to the switched-off job failed: %w", err)
	}
	if err := jobs.RunNow(ctx, under.jobID); err != nil {
		return fmt.Errorf("running the switched-off job now failed: %w", err)
	}
	if state, err := stateOfJob(ctx, jobs, under.jobID); err != nil || state != contract.JobRunning {
		return fmt.Errorf("after a run-now the switched-off job is %q (error %v), want %q, because a run-now starts a job again whatever stopped it", state, err, contract.JobRunning)
	}
	next, due, err := jobs.NextTask(ctx, under.afterThePutDownSteps())
	if err != nil {
		return fmt.Errorf("asking for the next task after the job was run now failed: %w", err)
	}
	if !due || next.TaskID != added || next.Text != theTaskAddedWhileOff {
		return fmt.Errorf("after the job was run now the next task is %+v (due %v), want %s, the task added while it was off, with its text", next, due, added)
	}
	if _, err := jobs.FinishTask(ctx, under.jobID, added, "the contract check finished it", false); err != nil {
		return fmt.Errorf("finishing the task %s failed: %w", added, err)
	}
	return nil
}

// checkTheLastTaskOfAJobThatIsOffFinishes is the middle of the step above: the
// switched-off job's one unfinished task finishes, and the store writes the
// report, marks the task done against it, and leaves the job off rather than
// closing it.
func checkTheLastTaskOfAJobThatIsOffFinishes(ctx context.Context, jobs contract.Job, under theJobUnderCheck) error {
	waiting := under.waiting
	reportID, err := jobs.FinishTask(ctx, under.jobID, waiting, "the contract check finished it while the job was off", false)
	if err != nil {
		return fmt.Errorf("finishing the last task of the switched-off job failed: %w", err)
	}
	if state, err := stateOfJob(ctx, jobs, under.jobID); err != nil || state != contract.JobOff {
		return fmt.Errorf("the job is %q (error %v) after its last task finished while it was off, want %q, because a job closes only while it is running", state, err, contract.JobOff)
	}
	record, err := jobs.Load(ctx, under.jobID)
	if err != nil {
		return fmt.Errorf("loading the switched-off job failed: %w", err)
	}
	if record.Header.Status == contract.StatusDone {
		return fmt.Errorf("the switched-off job's record stands at %q after its last task finished, want anything but done, because a job closes only while it is running", record.Header.Status)
	}
	task, found := jobTaskLabelled(record.Work.Tasks, waiting)
	if !found || !task.Done || task.ReportID != reportID {
		return fmt.Errorf("the task %s finished while the job was off and the record shows it as %+v (found %v), want it marked done against its report %s: a job that is not running still takes its running task's report", waiting, task, found, reportID)
	}
	for _, line := range record.Work.Results {
		if line.ID == reportID {
			return nil
		}
	}
	return fmt.Errorf("the report %s of the task finished while the job was off is not on the job's list of reports %+v", reportID, record.Work.Results)
}

// theNamelessJobsTemplate is the template of the job CheckJobWithoutAName makes,
// which has a schedule because CheckJob's job must close and a job with a
// schedule never does.
const theNamelessJobsTemplate = "write and post today's message"

// checkRunNowPullsTheTickForward holds a store with a scheduled job to the
// run-now rule: the next tick moves to now, which is earlier than where the
// schedule had put it, and asking for work at the moment the listing then names
// hands out a task made from the template, unattended. A store that leaves the
// tick where it was makes "/cron run" wait for the schedule it was meant to
// override.
func checkRunNowPullsTheTickForward(ctx context.Context, jobs contract.Job, jobID string, before time.Time) error {
	if before.IsZero() {
		return fmt.Errorf("the scheduled job %s lists no next run, and the cron listing needs one", jobID)
	}
	if err := jobs.RunNow(ctx, jobID); err != nil {
		return fmt.Errorf("running the scheduled job now failed: %w", err)
	}
	after, err := summaryOfJob(ctx, jobs, jobID)
	if err != nil {
		return err
	}
	if !after.NextRun.Before(before) {
		return fmt.Errorf("after a run-now the scheduled job's next run is %v, and it was %v before, want it pulled forward to now, because a run-now on a scheduled job ticks at once", after.NextRun, before)
	}
	next, due, err := jobs.NextTask(ctx, after.NextRun)
	if err != nil {
		return fmt.Errorf("asking for work at the moment the listing names as the next run failed: %w", err)
	}
	if !due || next.JobID != jobID || !next.Unattended || next.Text != theNamelessJobsTemplate {
		return fmt.Errorf("asking for work at the moment the listing names as the next run handed out %+v (due %v), want a task of job %s made from the template %q and marked unattended", next, due, jobID, theNamelessJobsTemplate)
	}
	return nil
}

// summaryOfJob reads one job's summary out of the listing.
func summaryOfJob(ctx context.Context, jobs contract.Job, jobID string) (contract.JobSummary, error) {
	listed, err := jobs.List(ctx)
	if err != nil {
		return contract.JobSummary{}, fmt.Errorf("listing the jobs failed: %w", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary, nil
		}
	}
	return contract.JobSummary{}, fmt.Errorf("the job %s is not in the listing", jobID)
}
