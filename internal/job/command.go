package job

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// titleWidth is how much of a job's ask one line of a listing shows. A listing
// is read down the left-hand side, so a long ask is cut rather than wrapped.
const titleWidth = 100

// cutMark is what stands at the end of a piece of text that was too long for its
// column, so that a reader knows there is more of it.
const cutMark = "..."

// JobsCommand returns the "/jobs" slash command, which the orchestrator
// registers in serve.go. On its own it lists every job with its progress and
// what it does; with a number it prints that job's record in full.
func (jobs *Jobs) JobsCommand() contract.Command {
	return contract.Command{
		Name: "jobs",
		Help: "list every job and its progress, /jobs 4 to see one in full",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			jobID := strings.TrimSpace(arguments)
			if jobID == "" {
				return jobs.listEveryJob(ctx)
			}
			return jobs.printOneRecord(ctx, jobID)
		},
	}
}

// CronCommand returns the "/cron" slash command. On its own it lists the jobs
// that have a schedule; with a number it shows one; "run" starts one now and
// "off" switches one off.
func (jobs *Jobs) CronCommand() contract.Command {
	return contract.Command{
		Name: "cron",
		Help: "list the jobs that have a schedule, /cron 3, /cron run 3, /cron off 3",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			word, rest, _ := strings.Cut(strings.TrimSpace(arguments), " ")
			rest = strings.TrimSpace(rest)
			switch word {
			case "":
				return jobs.listTheScheduledJobs(ctx)
			case "run":
				return jobs.runOneNow(ctx, rest)
			case "off":
				return jobs.switchOneOff(ctx, rest)
			default:
				return jobs.showOneScheduledJob(ctx, word)
			}
		},
	}
}

// listEveryJob is what "/jobs" prints: one line per job with its number, where
// it stands, how far it has got, and what it does.
func (jobs *Jobs) listEveryJob(ctx context.Context) (string, error) {
	listed, err := jobs.List(ctx)
	if err != nil {
		return "", err
	}
	if len(listed) == 0 {
		return "There are no jobs. A job is made when a piece of work is too big for one sitting.", nil
	}
	lines := []string{"Jobs, oldest first. Run /jobs 4 to see one in full."}
	numberWidth, stateWidth, progressWidth := 0, 0, 0
	for _, summary := range listed {
		numberWidth = widest(numberWidth, summary.ID)
		stateWidth = widest(stateWidth, string(summary.State))
		progressWidth = widest(progressWidth, progressOf(summary))
	}
	for _, summary := range listed {
		lines = append(lines, fmt.Sprintf("  %-*s  %-*s  %-*s  %s",
			numberWidth, summary.ID, stateWidth, summary.State,
			progressWidth, progressOf(summary), cutTo(safeLine(summary.Title), titleWidth)))
	}
	return strings.Join(lines, "\n"), nil
}

// listTheScheduledJobs is what "/cron" prints: the jobs that have a schedule,
// each with its schedule in plain words, what it does, and when it last ran and
// runs next.
func (jobs *Jobs) listTheScheduledJobs(ctx context.Context) (string, error) {
	scheduled, err := jobs.scheduledSummaries(ctx)
	if err != nil {
		return "", err
	}
	if len(scheduled) == 0 {
		return "No job has a schedule. A job with one makes a task every time the clock says so.", nil
	}
	lines := []string{"Jobs with a schedule. Run /cron 3 to see one, /cron run 3 to run it now, /cron off 3 to switch it off."}
	for _, summary := range scheduled {
		lines = append(lines, jobs.scheduleLinesOf(summary)...)
	}
	return strings.Join(lines, "\n"), nil
}

// showOneScheduledJob is what "/cron 3" prints: the schedule in plain words, the
// failures it has met, and the job's record in full.
func (jobs *Jobs) showOneScheduledJob(ctx context.Context, jobID string) (string, error) {
	schedule, err := jobs.scheduleOf(jobID)
	if err != nil {
		return "", err
	}
	if schedule == nil {
		return "", fmt.Errorf("job %s has no schedule, so run /jobs %s to see it instead", jobID, jobID)
	}
	summary, err := jobs.summaryFor(ctx, jobID)
	if err != nil {
		return "", err
	}
	lines := jobs.scheduleLinesOf(summary)
	incidents, err := jobs.Incidents(ctx, jobID)
	if err != nil {
		return "", err
	}
	for _, incident := range incidents {
		lines = append(lines, fmt.Sprintf("     %s seen %d times, last at %s: %s",
			incident.Signature, incident.Count, momentInPlainWords(incident.LastSeen), cutTo(incident.Error, titleWidth)))
	}
	printed, err := jobs.printOneRecord(ctx, jobID)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n") + "\n\n" + printed, nil
}

// scheduleLinesOf is the three lines "/cron" prints about one job.
func (jobs *Jobs) scheduleLinesOf(summary contract.JobSummary) []string {
	schedule, err := jobs.scheduleOf(summary.ID)
	said := "on a schedule that can no longer be read"
	if err == nil && schedule != nil {
		said = InPlainWords(*schedule)
	}
	return []string{
		fmt.Sprintf("  %s  %s  %s", summary.ID, summary.State, said),
		"     " + cutTo(safeLine(summary.Title), titleWidth),
		fmt.Sprintf("     last run %s, next run %s", momentInPlainWords(summary.LastRun), momentInPlainWords(summary.NextRun)),
	}
}

// runOneNow is what "/cron run 3" does.
func (jobs *Jobs) runOneNow(ctx context.Context, jobID string) (string, error) {
	if err := jobs.RunNow(ctx, jobID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Job %s runs its next task now, without waiting for a date.", jobID), nil
}

// switchOneOff is what "/cron off 3" does.
func (jobs *Jobs) switchOneOff(ctx context.Context, jobID string) (string, error) {
	if err := jobs.SwitchOff(ctx, jobID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Job %s is switched off. Run /cron run %s to start it again.", jobID, jobID), nil
}

// printOneRecord prints one job's record, which is the same text the model reads
// while one of the job's tasks runs.
func (jobs *Jobs) printOneRecord(ctx context.Context, jobID string) (string, error) {
	held, err := jobs.Load(ctx, jobID)
	if err != nil {
		return "", err
	}
	return string(record.Print(held)), nil
}

// scheduledSummaries is the summary of every job that has a schedule.
func (jobs *Jobs) scheduledSummaries(ctx context.Context) ([]contract.JobSummary, error) {
	listed, err := jobs.List(ctx)
	if err != nil {
		return nil, err
	}
	scheduled := []contract.JobSummary{}
	for _, summary := range listed {
		schedule, err := jobs.scheduleOf(summary.ID)
		if err != nil || schedule == nil {
			continue
		}
		scheduled = append(scheduled, summary)
	}
	return scheduled, nil
}

// summaryFor is one job's summary, by its number.
func (jobs *Jobs) summaryFor(ctx context.Context, jobID string) (contract.JobSummary, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return contract.JobSummary{}, err
	}
	return jobs.summaryOf(ctx, jobID, held)
}

// scheduleOf is one job's schedule, or nil when it has none.
func (jobs *Jobs) scheduleOf(jobID string) (*contract.Schedule, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return nil, err
	}
	return held.state.Schedule, nil
}

// progressOf is the progress line a listing shows, such as "3 of 12 tasks done".
func progressOf(summary contract.JobSummary) string {
	return fmt.Sprintf("%d of %d tasks done", summary.TasksDone, summary.TasksTotal)
}

// widest is the greater of a width already found and the width of a piece of
// text, which is how a listing's columns are lined up.
func widest(sofar int, text string) int {
	if len(text) > sofar {
		return len(text)
	}
	return sofar
}

// cutTo shortens a piece of text to fit a column, saying that it was cut. It
// counts letters rather than bytes, because a title cut halfway through an
// accented letter is a title that is no longer text.
func cutTo(text string, width int) string {
	letters := []rune(text)
	if len(letters) <= width {
		return text
	}
	if width <= len(cutMark) {
		return string(letters[:max(width, 0)])
	}
	return string(letters[:width-len(cutMark)]) + cutMark
}
