// Pausing a job that has failed three times in a row and switching a scheduled
// one off after ten is OpenClaw's design, at
// ~/Code/openclaw/src/cron/service/auto-disable.ts, where a schedule is given
// far more rope than a piece of work somebody is watching, because a provider
// that was down for an afternoon is not a broken job. Waking the model only when
// a watched output has really changed is Hermes' design, at
// ~/Code/hermes-agent/cron/monitor.py. The Go here is written fresh.

package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// FinishTask writes a finished task's report into its job and returns the
// report's identifier. A task that failed counts towards the failures in a row:
// three pause a plain job, and ten switch a scheduled one off.
func (jobs *Jobs) FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return "", err
	}
	return jobs.finishTask(ctx, jobID, held, taskID, report, failed, jobs.clock.Now())
}

// finishTask is the whole of taking one task's report back. The caller holds the
// lock.
func (jobs *Jobs) finishTask(ctx context.Context, jobID string, held *heldJob, taskID string,
	report string, failed bool, now time.Time) (string, error) {
	task, err := unfinishedTask(held, jobID, taskID)
	if err != nil {
		return "", err
	}
	if err := jobs.release(ctx, jobID, taskID); err != nil {
		return "", err
	}

	reportID, quiet, err := jobs.writeReport(ctx, jobID, held, report, failed)
	if err != nil {
		return "", err
	}
	changed := held.state
	changed.LastRun = now
	if quiet {
		changed.LastOutputReportID = reportID
	} else if !failed && changed.Monitor {
		changed.LastOutputHash, changed.LastOutputReportID = hashOf(report), reportID
	}
	if failed {
		changed = jobs.countTheFailure(changed, report, now)
	} else {
		changed.FailuresInARow, changed.Backoff = 0, 0
	}
	if !failed || task.Unattended {
		if err := held.keeper.MarkJobTask(ctx, taskID, reportID); err != nil {
			return "", fmt.Errorf("cannot mark task %s of job %s done: %w", taskID, jobID, err)
		}
	}
	if err := jobs.saveState(ctx, jobID, held, changed); err != nil {
		return "", err
	}
	if err := jobs.afterOneTask(ctx, jobID, held, report, failed); err != nil {
		return "", err
	}
	// The loop runs one task of a job per call and the driver takes the next
	// on its next look, so a job still running after a task has work due at
	// once, and a task with no date makes no moment for the wait to sleep
	// until. Without this the second task of a job started a minute later.
	if held.state.State == contract.JobRunning {
		jobs.wake()
	}
	return reportID, jobs.writeProgress(ctx, jobID, held)
}

// unfinishedTask finds one task on a job's list and refuses a task that is not
// there or is already finished, because a finished task's report is written once.
func unfinishedTask(held *heldJob, jobID string, taskID string) (taskFacts, error) {
	for _, task := range held.keeper.Record().Work.Tasks {
		if task.TaskID != taskID {
			continue
		}
		if task.Done {
			return taskFacts{}, fmt.Errorf("task %s of job %s is already finished and its report is %s, so a second report for it is refused",
				taskID, jobID, task.ReportID)
		}
		return held.state.facts(taskID), nil
	}
	return taskFacts{}, fmt.Errorf("job %s has no task %q, so run /jobs %s to see its list", jobID, taskID, jobID)
}

// writeReport puts one line for the report into the job's record and the whole
// of its text into the log. A job watching for a change writes nothing at all
// when the report reads the same as the last one: the record the model sees does
// not move, which is what "wake me only when it changes" means, and the tick is
// marked done against the report that first carried that text.
func (jobs *Jobs) writeReport(ctx context.Context, jobID string, held *heldJob, report string, failed bool) (string, bool, error) {
	if !failed && held.state.Monitor && held.state.LastOutputHash == hashOf(report) &&
		recordHoldsReport(held, held.state.LastOutputReportID) {
		return held.state.LastOutputReportID, true, nil
	}
	summary := safeLine(report)
	if summary == "" {
		summary = "the task reported nothing"
	}
	if failed {
		summary = "failed: " + summary
	}
	reportID, err := held.keeper.AddReport(ctx, summary, report)
	if err != nil {
		return "", false, fmt.Errorf("cannot write the report of a task of job %s: %w", jobID, err)
	}
	return reportID, false, nil
}

// countTheFailure adds one to the failures in a row, remembers the incident, and
// pushes a schedule's next tick back.
func (jobs *Jobs) countTheFailure(changed jobState, report string, now time.Time) jobState {
	changed.FailuresInARow++
	changed.Incidents, _ = noteIncident(changed.Incidents, report, now)
	if changed.Schedule == nil {
		return changed
	}
	pushedBack, waited, err := afterAFailedTick(*changed.Schedule, now, changed.FailuresInARow)
	if err != nil {
		return changed
	}
	changed.NextRun, changed.Backoff = pushedBack, waited
	return changed
}

// afterOneTask applies the two rules that stop a job that keeps failing, and
// closes a job whose last task is done. The caller holds the lock.
func (jobs *Jobs) afterOneTask(ctx context.Context, jobID string, held *heldJob, report string, failed bool) error {
	if failed {
		return jobs.stopIfItKeepsFailing(ctx, jobID, held, report)
	}
	return jobs.closeIfEveryTaskIsDone(ctx, jobID, held)
}

// stopIfItKeepsFailing pauses a plain job at three failures in a row and
// switches a scheduled one off at ten. A scheduled job is given the longer rope
// because a schedule is meant to survive a bad afternoon, and pausing it at three
// would put the ten out of reach. A job told to keep running, whose work is to
// report what it finds, is stopped by neither rule.
func (jobs *Jobs) stopIfItKeepsFailing(ctx context.Context, jobID string, held *heldJob, report string) error {
	if held.state.KeepRunning {
		return nil
	}
	failures := held.state.FailuresInARow
	if held.state.Schedule != nil {
		if failures < FailuresThatSwitchOff {
			return nil
		}
		return jobs.stopTheJob(ctx, jobID, held, contract.JobOff,
			fmt.Sprintf("job %s was switched off after %d of its tasks failed in a row", jobID, failures),
			safeLine(report))
	}
	if failures < FailuresThatPause {
		return nil
	}
	return jobs.stopTheJob(ctx, jobID, held, contract.JobPaused,
		fmt.Sprintf("job %s was paused after %d of its tasks failed in a row", jobID, failures),
		safeLine(report))
}

// closeIfEveryTaskIsDone finishes a job whose last task is done. A job with a
// schedule is never finished, because its next tick will add another task. A job
// closes only while it is running: one the person switched off, or one that
// paused itself on failures, keeps the report and the mark on the task and
// stays where the person left it, so that "/cron off" means off. A job
// whose done list has nothing behind it stays running rather than closing, which
// is the same rule a task record keeps: nothing says it is done until something
// proves it.
//
// A job the model gave no done list is a different case: its tasks are what
// done looks like, each with one clear done line behind it, and each finished
// one has a report that proves it. So when the last task finishes, the harness
// writes one done line per task, in the task's own words and pointing at its
// report, before the check runs, the way it writes the one done line of a task
// whose answer is its own proof. Without that no job could ever close, because
// nothing lets the model write a job's done list.
func (jobs *Jobs) closeIfEveryTaskIsDone(ctx context.Context, jobID string, held *heldJob) error {
	if held.state.Schedule != nil || held.state.State != contract.JobRunning {
		return nil
	}
	for _, task := range held.keeper.Record().Work.Tasks {
		if !task.Done {
			return nil
		}
	}
	if err := jobs.theTasksAreTheDoneList(ctx, jobID, held); err != nil {
		return err
	}
	if err := record.DoneCheck(held.keeper.Record()); err != nil {
		return nil
	}
	if err := held.keeper.SetStatus(ctx, contract.StatusDone); err != nil {
		return fmt.Errorf("cannot close job %s: %w", jobID, err)
	}
	changed := held.state
	changed.State = contract.JobDone
	return jobs.saveState(ctx, jobID, held, changed)
}

// theTasksAreTheDoneList writes one done line per task into a job whose done
// list is empty, each pointing at the report of the task it stands for. A done
// list the model did write is left exactly as it is. The caller holds the lock
// and has checked that every task is done.
func (jobs *Jobs) theTasksAreTheDoneList(ctx context.Context, jobID string, held *heldJob) error {
	if len(held.keeper.Record().Goal.DoneWhen) > 0 {
		return nil
	}
	lines := []contract.DoneLine{}
	for _, task := range held.keeper.Record().Work.Tasks {
		lines = append(lines, contract.DoneLine{Text: task.Text, Done: true, ResultID: task.ReportID})
	}
	if err := held.keeper.Apply(ctx, record.Update{DoneWhen: lines}); err != nil {
		return fmt.Errorf("cannot write the done line per task that closes job %s: %w", jobID, err)
	}
	return nil
}

// stopTheJob puts a job into a state it will not work in, writes the reason into
// its record as a failure with its cause, so that the user reads why the next
// time they look, and moves the record's own status with it. The caller holds
// the lock.
func (jobs *Jobs) stopTheJob(ctx context.Context, jobID string, held *heldJob,
	state contract.JobState, why string, cause string) error {
	if cause == "" {
		cause = "no cause was given"
	}
	err := held.keeper.Apply(ctx, record.Update{Failure: &record.NewFailure{Text: safeLine(why), Cause: cause}})
	if err != nil {
		return fmt.Errorf("cannot write into job %s why it stopped: %w", jobID, err)
	}
	if err := held.keeper.SetStatus(ctx, contract.RecordStatusOfJob(state)); err != nil {
		return fmt.Errorf("cannot write the status of job %s: %w", jobID, err)
	}
	changed := held.state
	changed.State = state
	if err := jobs.saveState(ctx, jobID, held, changed); err != nil {
		return err
	}
	return jobs.sayTheJobStopped(ctx, jobID, why, cause)
}

// sayTheJobStopped puts in front of the user the fact that a job has stopped
// working: why it stopped, the failure behind it, and the two commands that show
// it and start it again. Brief 4.4 asks for this message twice, and without it a
// paused job is invisible until somebody happens to type /jobs.
//
// The message is sent while the store's own lock is held, so the function must
// not call back into the jobs. When it cannot reach the user the job has still
// stopped and the reason is still in its record, and the caller is told that
// nobody heard.
func (jobs *Jobs) sayTheJobStopped(ctx context.Context, jobID string, why string, cause string) error {
	if jobs.tellTheUser == nil {
		return nil
	}
	said := fmt.Sprintf("%s. The last failure was: %s. Run /jobs %s to see it, and /cron run %s to start it again.",
		why, cause, jobID, jobID)
	if err := jobs.tellTheUser(ctx, said); err != nil {
		return fmt.Errorf("job %s stopped and the user was not told why: %w", jobID, err)
	}
	return nil
}

// recordHoldsReport says whether a report with that identifier is on the job's
// list of reports, which is what a quiet tick is marked done against.
func recordHoldsReport(held *heldJob, reportID string) bool {
	if reportID == "" {
		return false
	}
	for _, line := range held.keeper.Record().Work.Results {
		if line.ID == reportID {
			return true
		}
	}
	return false
}

// hashOf is the fingerprint of a piece of output, which is what a job watching
// for a change compares from one tick to the next. Nothing is trimmed or tidied
// first, so a source that prints the time on every run counts as changed every
// run, which is the source's fault and not the agent's.
func hashOf(output string) string {
	sum := sha256.Sum256([]byte(output))
	return hex.EncodeToString(sum[:])
}

// safeLine folds a piece of text onto one line and takes out the three marks a
// record's own lines are built from, so that a report quoting one of them cannot
// make the record read back as something else.
func safeLine(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	line = strings.ReplaceAll(line, " -> ", " then ")
	line = strings.ReplaceAll(line, ". Reason: ", ". reason: ")
	return strings.ReplaceAll(line, ". Cause: ", ". cause: ")
}
