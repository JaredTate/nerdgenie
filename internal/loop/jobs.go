package loop

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// pickUpAGuardStoppedTask picks a job's task up once, on a fresh window, when
// the harness's guard stopped it, and says whether it did. A task the guard
// stopped a second time is not picked up again; it is left for the set-aside
// or the put-down below.
func (theLoop *Loop) pickUpAGuardStoppedTask(ctx context.Context, task Task, number string, outcome Outcome) (Outcome, bool, error) {
	if !outcome.ByTheGuard || outcome.TaskID == "" {
		return outcome, false, nil
	}
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	again, err := theLoop.options.Jobs.PickUpOnce(ctx, jobID, taskID)
	if err != nil {
		return outcome, false, fmt.Errorf("cannot ask job %s whether it may pick its task %s up itself: %w", jobID, taskID, err)
	}
	if !again {
		return outcome, false, nil
	}
	out, err := theLoop.pickTheTaskUpItself(ctx, task, number, outcome)
	return out, true, err
}

// setAPacedOutTaskAside sets a task the pacer ended aside so the job goes on
// and comes back to it, or puts it down for the person when it was set aside
// once already, and says whether it handled the outcome. A paced-out task ran
// past three times the job's median; setting it aside keeps one slow task from
// being retried until three failures pause the whole job.
func (theLoop *Loop) setAPacedOutTaskAside(turnCtx, wrapCtx context.Context, task Task, number string, outcome Outcome) (Outcome, bool, error) {
	if !outcome.PacedOut || task.Unattended {
		return outcome, false, nil
	}
	setAside, err := theLoop.setTheTaskAside(wrapCtx, task, outcome, theTooSlowReason)
	if err != nil || setAside {
		return outcome, true, err
	}
	if theLoop.keepGoing() {
		out, err := theLoop.reorientOrPutDown(turnCtx, wrapCtx, task, number, outcome)
		return out, true, err
	}
	return outcome, true, theLoop.putTheTaskDown(wrapCtx, task, number, outcome)
}

// handleAStoppedOrWaitingTask deals with a job's task that did not finish: one
// the person stopped, or one that stopped to ask a question or on the harness's
// guard. Under yolo it is reoriented and carried on; otherwise a guard's second
// stop sets it aside and every other such stop puts it down for the person. It
// runs under the wrap-up context, wrapCtx, for its bookkeeping, and reruns a
// reorient under turnCtx, the whole turn's own. It says whether it handled the
// task; a task that finished is left for finishJobTask to write into the job.
func (theLoop *Loop) handleAStoppedOrWaitingTask(turnCtx, wrapCtx context.Context, task Task, number string, outcome Outcome) (Outcome, bool, error) {
	stopped := outcome.Status == contract.StatusStopped
	putDown := (stopped || outcome.Status == contract.StatusWaiting) && !task.Unattended
	if !putDown && !(stopped && outcome.ByThePerson) {
		return outcome, false, nil
	}
	if putDown && outcome.ByTheGuard {
		setAside, err := theLoop.setTheTaskAside(wrapCtx, task, outcome, theTwiceStoppedReason)
		if err != nil || setAside {
			return outcome, true, err
		}
	}
	if putDown && !outcome.ByThePerson && theLoop.keepGoing() {
		out, err := theLoop.reorientOrPutDown(turnCtx, wrapCtx, task, number, outcome)
		return out, true, err
	}
	return outcome, true, theLoop.putTheTaskDown(wrapCtx, task, number, outcome)
}

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
// turn; a second such stop on an attended task sets it aside, once, so that
// the job goes on with its next task and comes back to it; and only a third
// is put down, or failed on a schedule's task; the outcome handed back is
// the one the job's task ended with, the pick-up's when there was one.
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
	if out, picked, err := theLoop.pickUpAGuardStoppedTask(ctx, task, number, outcome); picked || err != nil {
		return out, err
	}
	// turnCtx is the whole turn's own context; a reorient reruns the model under
	// it, not the ten-second wrap-up context below, which is only for the quick
	// bookkeeping at a task's end. Running a reorient under the wrap-up context
	// cut the model off after ten seconds, the cut-off read as stopped, and
	// stopped reoriented again: a ten-second spin that never let the model work.
	turnCtx := ctx
	ctx, done := theLoop.timeToWrapUp(ctx)
	defer done()
	if out, handled, err := theLoop.setAPacedOutTaskAside(turnCtx, ctx, task, number, outcome); handled {
		return out, err
	}
	if out, handled, err := theLoop.handleAStoppedOrWaitingTask(turnCtx, ctx, task, number, outcome); handled {
		return out, err
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
	held, err = theLoop.keepTheLearnedFolder(ctx, jobID, held, outcome.FilesChanged)
	if err != nil {
		return outcome, err
	}
	if !failed {
		theLoop.writeTheProjectDocuments(held)
	}
	progress := progressLine(jobID, reportID, held) + theFailedChecksLine(outcome.JobProof) + theTookLine(theLoop.theTimingOf(ctx, jobID), taskID)
	if err := theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+progress); err != nil {
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

// TheFinishWhatIsProvableLine ends the pick-up's ask. Run 23's picked-up QA
// task went straight back to the polish that had stalled it.
const TheFinishWhatIsProvableLine = "Finish what is provable rather than continue with what was going round."

// thePickUpAsk is the picked-up task's ask: the pick-up line, the guard's own
// words for why the task was stopped when it gave them, and the line that
// asks for what is provable to be finished.
func thePickUpAsk(cause string) string {
	ask := TheJobPickUpLine
	if first, _, _ := strings.Cut(strings.TrimSpace(cause), "\n"); first != "" {
		ask += " It was stopped because " + strings.TrimSuffix(first, ".") + "."
	}
	return ask + " " + TheFinishWhatIsProvableLine
}

// keepGoing says whether the agent should reorient a stuck job task and carry
// on rather than wait for a person, which yolo turns on for the unattended
// agent. Off, the harness puts the task down and waits as it did before.
func (theLoop *Loop) keepGoing() bool {
	return theLoop.options.KeepGoing != nil && theLoop.options.KeepGoing()
}

// TheBoldReorientAsk is what a stuck job task is handed on a fresh window when
// the agent keeps going instead of waiting for a person: it breaks the trance
// of a model going round in circles by telling it to read what it already
// tried, drop all of it, and take a bold, fundamentally different route.
const TheBoldReorientAsk = "The harness is handing this task back to you on a fresh window rather than waiting for anyone, because it never stops until the job is done. " +
	"You were going round in circles, so break out of it now. First read the record's failures: they are the full list of what you have already tried, and none of it worked, so do not repeat any of it. " +
	"Now look at the whole task with completely fresh eyes. Think outside the box. Be bold and creative, and choose a fundamentally different approach from the ones that failed. " +
	"Say in one line the boldest, most different thing that could still work, then make your very next move that."

// keepsGoingLine is the one line under a stuck task's report saying the job
// keeps working it itself rather than waiting.
func keepsGoingLine(jobID string, taskID string) string {
	return fmt.Sprintf("Job %s keeps working task %s itself on a fresh window rather than waiting for anyone; it will not stop until the job is done.", jobID, taskID)
}

// reorientOrPutDown carries a stuck job task on under yolo, rerunning the model
// under turnCtx — the whole turn's own context — so a reorient gets a full turn
// and not the ten seconds of the wrap-up context the bookkeeping around it uses.
// When turnCtx is already done — the turn's deadline ran out, or the serve is
// shutting down — the task is put down under wrapCtx instead of reoriented, so
// never-quit is bounded by the turn the way every loop must be, rather than
// spinning ten-second reruns that are cut off before the model can work.
func (theLoop *Loop) reorientOrPutDown(turnCtx, wrapCtx context.Context, task Task, number string, outcome Outcome) (Outcome, error) {
	if turnCtx.Err() != nil {
		return outcome, theLoop.putTheTaskDown(wrapCtx, task, number, outcome)
	}
	return theLoop.boldlyReorientAndKeepGoing(turnCtx, task, number, outcome)
}

// boldlyReorientAndKeepGoing hands a stuck job task back to the model on a
// fresh window with the bold reorient ask, and runs it again, so an unattended
// job never stops to wait for a person. It is the pick-up above without the
// one-time budget: the job keeps going as many fresh starts as it takes.
func (theLoop *Loop) boldlyReorientAndKeepGoing(ctx context.Context, task Task, number string, outcome Outcome) (Outcome, error) {
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	if err := theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+keepsGoingLine(jobID, taskID)); err != nil {
		return outcome, err
	}
	reoriented := Task{
		Message:    contract.Inbound{ID: task.Message.ID, Text: TheBoldReorientAsk, Channel: task.Message.Channel},
		Channel:    task.Channel,
		FromJob:    task.FromJob,
		Unattended: task.Unattended,
		ResumeID:   number,
	}
	return theLoop.runTaskAndItsJob(ctx, reoriented)
}

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
		Message:    contract.Inbound{ID: task.Message.ID, Text: thePickUpAsk(outcome.StopLine), Channel: task.Message.Channel},
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
	return fmt.Sprintf("Job %s picks task %s up itself, once, on a fresh window; if it stops the same way again, the job sets it aside and goes on.", jobID, taskID)
}

// setTheTaskAside asks the job to set a guard-stopped task aside, once the
// job's one pick-up is spent, and says whether it did. When it did, the
// person is told in one line under the stopped report, the task's record is
// left where it stands, the way a put-down task's is, nothing is written into
// the job, and the driver takes the job's next task on its next ask, the way
// it does after a finished one; the task comes back through the store once
// only deferred tasks remain, started afresh on its own words. When the store
// answers no, because the task was set aside once already, the task is left
// to be put down for the person. On the night of 7 September 2026 the flight
// simulator's sky task waited nine hours for a person, three times, while
// twelve tasks that needed nothing from it sat untouched.
func (theLoop *Loop) setTheTaskAside(ctx context.Context, task Task, outcome Outcome, reason string) (bool, error) {
	jobID, taskID := task.FromJob.JobID, task.FromJob.TaskID
	deferred, err := theLoop.options.Jobs.Defer(ctx, jobID, taskID)
	if err != nil {
		return false, fmt.Errorf("cannot ask job %s whether it may set its task %s aside: %w", jobID, taskID, err)
	}
	if !deferred {
		return false, nil
	}
	return true, theLoop.tell(ctx, task.Channel, outcome.Report+"\n"+setAsideLine(jobID, taskID, reason))
}

// Why a task was set aside, in the line the person reads: it stopped on the
// guard twice (theTwiceStoppedReason) or it ran past three times the job's
// median (theTooSlowReason).
const (
	theTwiceStoppedReason = "after stopping twice"
	theTooSlowReason      = "after running past three times the job's median"
)

// setAsideLine is the line under a set-aside task's report saying the job sets
// the task aside, goes on, and comes back to it, with the reason it was set
// aside.
func setAsideLine(jobID string, taskID string, reason string) string {
	return fmt.Sprintf("Task %s is set aside %s; job %s goes on with the next task and comes back to %s before its last task.",
		taskID, reason, jobID, taskID)
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

// MaxFailedChecksNamed is how many failed checks the progress line names, and
// MaxCheckReasonRunes how much of each check's own words it carries.
const (
	MaxFailedChecksNamed = 2
	MaxCheckReasonRunes  = 160
)

// theFailedChecksLine names the lines whose checks failed at this task's end
// and what each check showed, so that the next task knows what to put
// right. On run 24 the shows and looks checks failed after the board task
// because nothing answered on the port, and the next task saw two unproved
// lines and no reason. Nothing is added when every check passed.
func theFailedChecksLine(proof *JobProof) string {
	if proof == nil || len(proof.Failed) == 0 {
		return ""
	}
	numbers := make([]int, 0, len(proof.Failed))
	for number := range proof.Failed {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	var parts []string
	for _, number := range numbers {
		if len(parts) == MaxFailedChecksNamed {
			parts = append(parts, fmt.Sprintf("and %d more", len(numbers)-MaxFailedChecksNamed))
			break
		}
		reason := strings.Join(strings.Fields(proof.Failed[number]), " ")
		if runes := []rune(reason); len(runes) > MaxCheckReasonRunes {
			reason = string(runes[:MaxCheckReasonRunes-3]) + "..."
		}
		parts = append(parts, fmt.Sprintf("line %d, %s", number, reason))
	}
	return " Not proved: " + strings.Join(parts, "; ") + "."
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
	report := fmt.Sprintf("Job %s is finished: every one of its %d tasks is done%s.", jobID, held.Header.TasksTotal,
		theJobsSpan(theLoop.theTimingOf(ctx, jobID), theLoop.options.Clock.Now()))
	if err := record.DoneCheck(held); err != nil {
		report = fmt.Sprintf("Job %s has run every task, and its done list is not proven yet. %s", jobID, err.Error())
		if staysRed != "" {
			report += "\n" + staysRed
		}
	} else if staysRed == "" {
		theLoop.writeTheStandingOrder(held)
	}
	// An unattended job has nobody to show a skill offer to, so its lesson is
	// kept as a fact and nothing is offered. The report below still goes
	// wherever the job's reports go.
	offerTo := where
	if unattended {
		offerTo = nil
	}
	whole, answer := theLoop.askTheFourQuestions(ctx, string(record.Print(held)))
	outcome := "no lesson"
	if answer != "" {
		if err := theLoop.keepTheLesson(ctx, offerTo, "job "+jobID, answer); err != nil {
			return err
		}
		outcome = "kept the fourth answer as a lesson"
	}
	theLoop.logTheQuestion(ctx, contract.RecordLogKey(contract.RecordJob, jobID), "review", TheFourQuestions, whole, outcome)
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
