package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// finishJobTask writes a finished task's report into its job and sends it to
// the user with the job's progress line on it. A task the person stopped, or
// one that stopped to ask them a question, is not finished at all: it is put
// down where it is, for the person to pick up with the word that carries on
// or with their answer. A person's stop puts down any job's task, a
// schedule's included, because unattended means a schedule made the task and
// not that nobody is watching: the person at the terminal stopped it on
// purpose. A schedule's task that the harness stopped, on a spent budget or a
// line of the stop list, or that asked a question, has nobody to pick it up,
// so it is finished as the failure it is, and the schedule's next tick brings
// its own task. A task the harness's own guard stopped, attended or not, is
// picked up once by the job itself first, on a fresh window and on the same
// turn, and only a second such stop is put down or failed; the outcome handed
// back is the one the job's task ended with, the pick-up's when there was one.
//
// The bookkeeping runs under a short context of the loop's own, the way the
// ending of a task does, because a turn cut off by its deadline has a
// cancelled context: the report and the release of the claim used to fail
// under it with "context canceled", and the cut-off task sat claimed for the
// hour of its budget.
func (theLoop *Loop) finishJobTask(ctx context.Context, task Task, number string, outcome Outcome) (Outcome, error) {
	if theLoop.options.Jobs == nil {
		return outcome, nil
	}
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	if outcome.ByTheGuard && outcome.TaskID != "" {
		again, err := theLoop.options.Jobs.PickUpOnce(ctx, jobID, taskID)
		if err != nil {
			return outcome, fmt.Errorf("cannot ask job %s whether it may pick its task %s up itself: %w", jobID, taskID, err)
		}
		if again {
			return theLoop.pickTheTaskUpItself(ctx, task, number, outcome)
		}
	}
	ctx, done := theLoop.timeToWrapUp(ctx)
	defer done()
	stopped := outcome.Status == contract.StatusStopped
	putDown := (stopped || outcome.Status == contract.StatusWaiting) && !task.Unattended
	if putDown || (stopped && outcome.ByThePerson) {
		return outcome, theLoop.putTheTaskDown(ctx, task, number, outcome)
	}
	failed := outcome.Status != contract.StatusDone
	reportID, err := theLoop.options.Jobs.FinishTask(ctx, jobID, taskID, outcome.Report, failed)
	if err != nil {
		return outcome, fmt.Errorf("cannot write the report of task %s into job %s: %w", taskID, jobID, err)
	}
	if err := theLoop.markTheJobsDoneLines(ctx, jobID, reportID, outcome.JobProof); err != nil {
		return outcome, err
	}
	held, err := theLoop.options.Jobs.Load(ctx, jobID)
	if err != nil {
		return outcome, fmt.Errorf("cannot read job %s after its task finished: %w", jobID, err)
	}
	if err := theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+progressLine(jobID, reportID, held)); err != nil {
		return outcome, err
	}
	if !everyTaskIsDone(held) {
		return outcome, nil
	}
	added, err := theLoop.giveTheJobAFixTask(ctx, jobID, held, outcome.JobProof)
	if err != nil || added {
		return outcome, err
	}
	return outcome, theLoop.closeTheJob(ctx, task.Channel, jobID, held, task.Unattended, whatStaysRed(outcome.JobProof, held))
}

// TheJobPickUpLine is the ask a job's task is picked up with when the job
// picks it up itself after the harness's guard stopped it. It stands where
// the person's word would, last in the fresh window.
const TheJobPickUpLine = "The harness stopped this task because it was going round in circles, and its job has picked it up again, once, on a fresh window. " +
	"Go on from where the record says the work stands, and not the way that stalled: the record's failures say what that was."

// pickTheTaskUpItself picks a job's task up the way the person's word does,
// once, right after the harness's guard stopped it: the person is sent the
// stopped report with a line saying the job picks the task up itself, and the
// run is resumed by its number under the same job, on a fresh window with the
// record's newest results in front of the model, on the same turn. Run ten's
// polish task was ended by the same-call guard at round 212 and the job
// waited for a person to type continue; the GLM run stalled the same way. A
// task that stops the same way again is put down for a person, because the
// store answers the second ask with no.
func (theLoop *Loop) pickTheTaskUpItself(ctx context.Context, task Task, number string, outcome Outcome) (Outcome, error) {
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	if err := theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+picksItUpLine(jobID, taskID)); err != nil {
		return outcome, err
	}
	pickedUp := Task{
		Message:    contract.Inbound{ID: task.Message.ID, Text: TheJobPickUpLine, Channel: task.Message.Channel},
		Channel:    task.Channel,
		FromJob:    task.FromJob,
		Unattended: task.Unattended,
		ResumeID:   number,
	}
	return theLoop.runTaskAndItsJob(ctx, pickedUp)
}

// picksItUpLine is the line under a guard-stopped task's report saying the
// job picks the task up itself, and what happens if that stops too.
func picksItUpLine(jobID string, taskID string) string {
	return fmt.Sprintf("Job %s picks task %s up itself, once, on a fresh window; if it stops the same way again, the job waits for you.", jobID, taskID)
}

// progressLine is the line every job report carries: which job it was, which
// report it became, and how far the job has got.
func progressLine(jobID string, reportID string, held contract.Record) string {
	line := fmt.Sprintf("Job %s, report %s: %d of %d tasks done.",
		jobID, reportID, held.Header.TasksDone, held.Header.TasksTotal)
	if proved := provedCountLine(held); proved != "" {
		line += " " + proved
	}
	return line
}

// everyTaskIsDone says whether the job has run out of tasks to do.
func everyTaskIsDone(held contract.Record) bool {
	if len(held.Work.Tasks) == 0 {
		return false
	}
	for _, task := range held.Work.Tasks {
		if !task.Done {
			return false
		}
	}
	return true
}

// closeTheJob runs the job's own done-check and review when its last task has
// finished, and sends the final report.
func (theLoop *Loop) closeTheJob(ctx context.Context, where contract.Channel, jobID string,
	held contract.Record, unattended bool, staysRed string) error {
	report := fmt.Sprintf("Job %s is finished: every one of its %d tasks is done.", jobID, held.Header.TasksTotal)
	if err := record.DoneCheck(held); err != nil {
		report = fmt.Sprintf("Job %s has run every task, and its done list is not proven yet. %s", jobID, err.Error())
		if staysRed != "" {
			report += "\n" + staysRed
		}
	}
	// An unattended job has nobody to show a skill offer to, so its lesson is
	// kept as a fact and nothing is offered. The report below still goes
	// wherever the job's reports go.
	offerTo := where
	if unattended {
		offerTo = nil
	}
	if answer := theLoop.askTheFourQuestions(ctx, string(record.Print(held))); answer != "" {
		if err := theLoop.keepTheLesson(ctx, offerTo, "job "+jobID, answer); err != nil {
			return err
		}
	}
	return theLoop.tell(ctx, where, report)
}

// tell writes a message into the log and then sends it, which is the order
// everything the user is told goes in.
func (theLoop *Loop) tell(ctx context.Context, where contract.Channel, text string) error {
	if where == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	if err := theLoop.logEvent(ctx, "", contract.EventReply, struct {
		Text string `json:"text"`
	}{Text: text}); err != nil {
		return err
	}
	if err := where.Send(ctx, text); err != nil {
		return fmt.Errorf("cannot send the job's report to the user: %w", err)
	}
	return nil
}
