package loop

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxJobTasksRemembered is how many of the job tasks this loop has run it keeps
// the job of, newest kept, so that one picked up again by its number, after a
// question or a stop, is picked up as the job's task it was. A task is picked
// up soon after it is put down, so a hundred is plenty; past that the oldest
// is forgotten and would be picked up as a plain task.
const MaxJobTasksRemembered = 100

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

// putTheTaskDown leaves a job's task where the person stopped it, or where it
// stopped to ask them something, rather than finishing it. The task is neither
// done nor failed and nothing is written into the job; the job is paused on it
// with the mark that says which task and which run, kept by the job store so
// that a restart reads it back, so that nothing of the job runs until the
// person picks it up, and so that its claim is not given up as a dead
// process's an hour later; and the person gets the task's own report, the
// stopped report or the question, with the job named under it where a finished
// task's report carries its progress line. The newest put-down task is the one
// picked up: it is the one the person was just looking at, and a job put down
// before it stays paused until they start it again by hand.
//
// The report goes out whatever the store says about the mark. A mark the
// store refuses used to skip the report, so the question was never sent and
// never logged and the program moved on past a question nobody saw; now the
// report carries a line saying the job could not be marked and what to do,
// and the store's error is still handed back once the report has gone.
func (theLoop *Loop) putTheTaskDown(ctx context.Context, task Task, number string, outcome Outcome) error {
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	waiting := outcome.Status == contract.StatusWaiting
	mark := contract.PutDownMark{Task: *task.FromJob, Run: number, HasRecord: outcome.TaskID != "", Waiting: waiting}
	under := pausedOnLine(jobID, taskID)
	if waiting {
		under = waitingOnLine(jobID, taskID)
	}
	marking := theLoop.options.Jobs.PutDown(ctx, mark)
	if marking != nil {
		marking = fmt.Errorf("cannot put job %s down on its task %s: %w", jobID, taskID, marking)
		under = couldNotMarkLine(jobID, taskID, waiting, marking)
	}
	if err := theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+under); err != nil {
		return err
	}
	return marking
}

// couldNotMarkLine is the line under a job task's report when the store would
// not write the mark: what could not be marked, why, and what to do, which for
// a stopped task is the word that tries again and for a question is the
// answer itself, because the program still names the task on the person's
// next message and the task is put down again when it ends.
func couldNotMarkLine(jobID string, taskID string, waiting bool, why error) string {
	if waiting {
		return fmt.Sprintf("I could not mark job %s as waiting on task %s (%v). Your answer still reaches the task, and I will try to mark it again when the task next stops.",
			jobID, taskID, why)
	}
	return fmt.Sprintf("I could not mark job %s as paused on task %s (%v). Say %s to try again.",
		jobID, taskID, why, contract.CarryOnWords[0])
}

// pausedOnLine is the line under a stopped job task's report: which job waits
// on which task, and the one word that picks it up. It is the only instruction
// the report gives, because a person told to say how to carry on and then to
// say continue did the first and started a plain task.
func pausedOnLine(jobID string, taskID string) string {
	return fmt.Sprintf("Job %s is paused on task %s. Say %s to pick this task up.", jobID, taskID, contract.CarryOnWords[0])
}

// waitingOnLine is the line under a job task's question: which job waits on
// which task, and that the next message answers it.
func waitingOnLine(jobID string, taskID string) string {
	return fmt.Sprintf("Job %s is waiting on task %s for your answer.", jobID, taskID)
}

// pickUpTheJobsTask hands a person's message that picks a job's task up the
// job it belongs to. A message that only says to carry on picks up the job's
// task the person stopped most recently, and any message at all answers the
// job's task that most recently asked a question, unless in either case the
// caller named a task of the person's own that stopped or asked after it; and
// a task picked up again by its number is picked up as the job's task it was,
// so that its report reaches the job however the task ended. A job's own task,
// and a message that is none of these, go through unchanged.
func (theLoop *Loop) pickUpTheJobsTask(ctx context.Context, task Task) (Task, error) {
	if task.FromJob != nil || theLoop.options.Jobs == nil {
		return task, nil
	}
	putDown, there, err := theLoop.thePutDownTaskFor(ctx, task)
	if err != nil {
		return Task{}, err
	}
	if there {
		return theLoop.carryOn(ctx, task, putDown)
	}
	if fromJob, known := theLoop.jobTaskBehind(task.ResumeID); known {
		task.FromJob, task.Unattended = &fromJob, fromJob.Unattended
	}
	return task, nil
}

// thePutDownTaskFor asks the job store for the job's task put down most
// recently and hands it over when this message picks it up, which for a task
// that asked a question is any message and for a stopped task is a message
// that begins with one of the words that carry on. The store is asked rather
// than this loop's memory so that a task put down before a restart is still
// picked up after it, and the store forgets the mark only once it has set the
// job running again, so a store error on the way leaves the task put down for
// the next word. The caller may have named a stopped or waiting task of the
// person's own; the one that stopped or asked later is the one the person
// means, and a task's number says which that is.
func (theLoop *Loop) thePutDownTaskFor(ctx context.Context, task Task) (contract.PutDownMark, bool, error) {
	putDown, there, err := theLoop.options.Jobs.PutDownTask(ctx)
	if err != nil {
		return contract.PutDownMark{}, false, fmt.Errorf("cannot ask the jobs which task was put down: %w", err)
	}
	if !there {
		return contract.PutDownMark{}, false, nil
	}
	if _, carriesOn := contract.CarryOn(task.Message.Text); !putDown.Waiting && !carriesOn {
		return contract.PutDownMark{}, false, nil
	}
	if task.ResumeID != "" && isNewer(task.ResumeID, putDown.Run) {
		return contract.PutDownMark{}, false, nil
	}
	return putDown, true, nil
}

// steerFromTheStart writes the words the person picked a stopped task up with
// into its record as a correction. The message itself is already in front of
// the model and in the log as the one that started this run, so only the
// record is written here, and the task is marked as corrected so that its
// ending is reviewed the way any corrected task's is.
func (running *run) steerFromTheStart(ctx context.Context, text string) error {
	running.hadCorrection = true
	if running.keeper == nil {
		return nil
	}
	if _, err := running.keeper.AddCorrection(ctx, text); err != nil {
		return fmt.Errorf("cannot write the words task %s was carried on with into its record: %w", running.keeper.ID(), err)
	}
	return nil
}

// isNewer says whether the first task number was handed out after the other.
func isNewer(number string, other string) bool {
	first, err := strconv.Atoi(number)
	if err != nil {
		return false
	}
	second, err := strconv.Atoi(other)
	if err != nil {
		return false
	}
	return first > second
}

// carryOn turns the person's word, or their answer, into the job's task it
// picks up. The job is set running again first, through Resume and never
// RunNow, because the person means "carry on where you were": Resume forgets
// the mark and lets go of the claim the stopped run held and touches nothing
// else, where RunNow would take the date off the first unfinished task, which
// may be one dated next week and not the one picked up, and fire a scheduled
// job's tick at once; then the run that was stopped or asked is
// picked up under the job when it made a record, and the task is started
// afresh under the job, with its own words as the ask, when the stop or the
// question came before its first tool call. What the person said beyond the
// word, the answer to the task's question or a steer such as "but post at
// noon", reaches the task too: a task picked up from its record has it written
// in as a correction, and a task started afresh is told it right after its own
// words, so the model is told both what to do and what the person said.
func (theLoop *Loop) carryOn(ctx context.Context, task Task, putDown contract.PutDownMark) (Task, error) {
	jobID, taskID := putDown.Task.JobID, putDown.Task.TaskID
	if err := theLoop.options.Jobs.Resume(ctx, jobID); err != nil {
		return Task{}, fmt.Errorf("cannot set job %s running again to carry on its task %s: %w", jobID, taskID, err)
	}
	rest, _ := contract.CarryOn(task.Message.Text)
	saidMore := putDown.Waiting || rest != ""
	if !putDown.HasRecord {
		fresh := taskFromJob(putDown.Task, task.Channel)
		if saidMore {
			fresh.Answer = task.Message
		}
		return fresh, nil
	}
	fromJob := putDown.Task
	task.FromJob, task.Unattended, task.ResumeID = &fromJob, fromJob.Unattended, putDown.Run
	if !putDown.Waiting && rest != "" {
		task.Correction = task.Message
	}
	return task, nil
}
