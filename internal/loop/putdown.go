package loop

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxJobTasksRemembered is how many of the job tasks this loop has run it keeps
// the job of, newest kept, so that one picked up again by its number, after a
// question or a stop, is picked up as the job's task it was. A task is picked
// up soon after it is put down, so a hundred is plenty; past that the oldest
// is forgotten and would be picked up as a plain task.
const MaxJobTasksRemembered = 100

// theWordsThatCarryOn is the short fixed list of whole messages that mean
// "pick up the task you put down". It is the list cmd/nerdgenie/resuming.go
// reads for a person's own stopped task, kept in step with it by hand, because
// only this package knows which job a stopped task belongs to and the program
// hands it a job's task back by nothing but these words.
var theWordsThatCarryOn = []string{"continue", "go on", "carry on", "keep going"}

// putDownTask is a job's task the person stopped and has not yet carried on:
// which job's task it is, the number of the run that was stopped, and whether
// that run made a record, which is what the word that carries on picks up.
type putDownTask struct {
	fromJob   contract.TaskToRun
	number    string
	hasRecord bool
}

// saysCarryOn says whether the whole message is one of the ways of asking for
// the stopped task back, whatever case it was typed in and whatever
// punctuation it ends with. Anything longer is a new ask, even when it begins
// with one of the words.
func saysCarryOn(said string) bool {
	plain := strings.TrimRight(strings.ToLower(strings.TrimSpace(said)), ".!? ")
	return slices.Contains(theWordsThatCarryOn, plain)
}

// rememberTheJobTask writes down which job's task a number was run for, and
// forgets the oldest past the cap. A person's own task belongs to no job and
// is not written down.
func (theLoop *Loop) rememberTheJobTask(number string, fromJob *contract.TaskToRun) {
	if fromJob == nil {
		return
	}
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if theLoop.jobTasks == nil {
		theLoop.jobTasks = map[string]contract.TaskToRun{}
	}
	if _, known := theLoop.jobTasks[number]; !known {
		theLoop.jobTaskNumbers = append(theLoop.jobTaskNumbers, number)
	}
	theLoop.jobTasks[number] = *fromJob
	for len(theLoop.jobTaskNumbers) > MaxJobTasksRemembered {
		delete(theLoop.jobTasks, theLoop.jobTaskNumbers[0])
		theLoop.jobTaskNumbers = theLoop.jobTaskNumbers[1:]
	}
}

// jobTaskBehind is the job's task a number was run for, and false for a
// person's own task or one this loop has forgotten.
func (theLoop *Loop) jobTaskBehind(number string) (contract.TaskToRun, bool) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	fromJob, known := theLoop.jobTasks[number]
	return fromJob, known
}

// putTheTaskDown leaves a job's task where the person stopped it rather than
// finishing it. The task is neither done nor failed and nothing is written
// into the job; the job is paused on it, so that nothing of the job runs until
// the person says to carry on; and the person gets the task's own stopped
// report, which ends by asking how to carry on, with the job named under it
// where a finished task's report carries its progress line. Only the newest
// put-down task is kept: it is the one the person was just looking at, and a
// job put down before it stays paused until they start it again by hand.
func (theLoop *Loop) putTheTaskDown(ctx context.Context, task Task, number string, outcome Outcome) error {
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	if err := theLoop.options.Jobs.Pause(ctx, jobID); err != nil {
		return fmt.Errorf("cannot pause job %s on its stopped task %s: %w", jobID, taskID, err)
	}
	theLoop.guard.Lock()
	theLoop.putDown = &putDownTask{fromJob: *task.FromJob, number: number, hasRecord: outcome.TaskID != ""}
	theLoop.guard.Unlock()
	return theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+pausedOnLine(jobID, taskID))
}

// pausedOnLine is the line under a stopped job task's report: which job waits
// on which task, and the word that carries it on.
func pausedOnLine(jobID string, taskID string) string {
	return fmt.Sprintf("Job %s is paused on task %s until you say %s.", jobID, taskID, theWordsThatCarryOn[0])
}

// pickUpTheJobsTask hands a person's message that picks a job's task up the
// job it belongs to. A message that only says to carry on picks up the job's
// task the person put down most recently, unless the caller named a stopped
// task of the person's own that stopped after it; and a task picked up again
// by its number is picked up as the job's task it was, so that its report
// reaches the job however the task ended. A job's own task, and a message that
// is neither, go through unchanged.
func (theLoop *Loop) pickUpTheJobsTask(ctx context.Context, task Task) (Task, error) {
	if task.FromJob != nil {
		return task, nil
	}
	if putDown, there := theLoop.takeThePutDownTaskFor(task); there {
		return theLoop.carryOn(ctx, task, putDown)
	}
	if fromJob, known := theLoop.jobTaskBehind(task.ResumeID); known {
		task.FromJob, task.Unattended = &fromJob, fromJob.Unattended
	}
	return task, nil
}

// takeThePutDownTaskFor hands over the job's task put down most recently when
// this message is the person asking for the stopped task back, and forgets
// it, because a task picked up is put down no longer. The caller may have
// named a stopped task of the person's own; the one that stopped later is the
// one the person means, and a task's number says which that is.
func (theLoop *Loop) takeThePutDownTaskFor(task Task) (putDownTask, bool) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if theLoop.putDown == nil || !saysCarryOn(task.Message.Text) {
		return putDownTask{}, false
	}
	if task.ResumeID != "" && isNewer(task.ResumeID, theLoop.putDown.number) {
		return putDownTask{}, false
	}
	putDown := *theLoop.putDown
	theLoop.putDown = nil
	return putDown, true
}

// isNewer says whether the first task number was handed out after the second.
func isNewer(number string, than string) bool {
	first, err := strconv.Atoi(number)
	if err != nil {
		return false
	}
	second, err := strconv.Atoi(than)
	if err != nil {
		return false
	}
	return first > second
}

// carryOn turns the person's word into the job's task it picks up. The job is
// set running again first, so that the next task starts as usual when this
// one finishes; then the run that was stopped is picked up under the job when
// it made a record, and the task is started afresh under the job, with its
// own words as the ask, when the stop landed before its first tool call.
func (theLoop *Loop) carryOn(ctx context.Context, task Task, putDown putDownTask) (Task, error) {
	jobID, taskID := putDown.fromJob.JobID, putDown.fromJob.TaskID
	if err := theLoop.options.Jobs.RunNow(ctx, jobID); err != nil {
		return Task{}, fmt.Errorf("cannot set job %s running again to carry on its task %s: %w", jobID, taskID, err)
	}
	if !putDown.hasRecord {
		return taskFromJob(putDown.fromJob, task.Channel), nil
	}
	fromJob := putDown.fromJob
	task.FromJob, task.Unattended, task.ResumeID = &fromJob, fromJob.Unattended, putDown.number
	return task, nil
}
