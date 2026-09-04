package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// finishJobTask writes a finished task's report into its job, sends it to the
// user with the job's progress line on it, and hands back the next task the job
// wants run. A task that is waiting on the user is not finished at all, so the
// job is left where it is until the user answers. A task the person stopped is
// not finished either: it is put down where it is, for the person to pick up
// with the word that carries on. A stopped task nobody attended has nobody to
// pick it up, so it is finished as the failure it is, and the schedule's next
// tick brings its own task.
func (theLoop *Loop) finishJobTask(ctx context.Context, task Task, number string, outcome Outcome) (Task, bool, error) {
	if theLoop.options.Jobs == nil || outcome.Status == contract.StatusWaiting {
		return Task{}, false, nil
	}
	if outcome.Status == contract.StatusStopped && !task.Unattended {
		return Task{}, false, theLoop.putTheTaskDown(ctx, task, number, outcome)
	}
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	failed := outcome.Status != contract.StatusDone
	reportID, err := theLoop.options.Jobs.FinishTask(ctx, jobID, taskID, outcome.Report, failed)
	if err != nil {
		return Task{}, false, fmt.Errorf("cannot write the report of task %s into job %s: %w", taskID, jobID, err)
	}
	held, err := theLoop.options.Jobs.Load(ctx, jobID)
	if err != nil {
		return Task{}, false, fmt.Errorf("cannot read job %s after its task finished: %w", jobID, err)
	}
	if err := theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+progressLine(jobID, reportID, held)); err != nil {
		return Task{}, false, err
	}
	if everyTaskIsDone(held) {
		return Task{}, false, theLoop.closeTheJob(ctx, task.Channel, jobID, held, task.Unattended)
	}
	return theLoop.nextTaskOfAJob(ctx, task.Channel)
}

// progressLine is the line every job report carries: which job it was, which
// report it became, and how far the job has got.
func progressLine(jobID string, reportID string, held contract.Record) string {
	return fmt.Sprintf("Job %s, report %s: %d of %d tasks done.",
		jobID, reportID, held.Header.TasksDone, held.Header.TasksTotal)
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

// nextTaskOfAJob asks the job store what is due now and hands it back as a task
// to run, or says there is nothing to do.
func (theLoop *Loop) nextTaskOfAJob(ctx context.Context, where contract.Channel) (Task, bool, error) {
	due, there, err := theLoop.options.Jobs.NextTask(ctx, theLoop.options.Clock.Now())
	if err != nil {
		return Task{}, false, fmt.Errorf("cannot ask the jobs which task is due next: %w", err)
	}
	if !there {
		return Task{}, false, nil
	}
	return taskFromJob(due, where), true, nil
}

// closeTheJob runs the job's own done-check and review when its last task has
// finished, and sends the final report.
func (theLoop *Loop) closeTheJob(ctx context.Context, where contract.Channel, jobID string,
	held contract.Record, unattended bool) error {
	report := fmt.Sprintf("Job %s is finished: every one of its %d tasks is done.", jobID, held.Header.TasksTotal)
	if err := record.DoneCheck(held); err != nil {
		report = fmt.Sprintf("Job %s has run every task, and its done list is not proven yet. %s", jobID, err.Error())
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
