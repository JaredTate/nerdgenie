package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// A job made from a work order carries the person's done lines, and the
// harness proves them at the end of every task, not only at the finish: the
// task's own run holds the sandbox, the browser and the rulebook, so it runs
// the job's bracketed checks as it ends and hands what they showed to the
// job's bookkeeping in its outcome. A check that passed marks its line with
// the task's report; a check that failed unmarks it. When the last task ends
// with a check still red, the job gives itself one more task to make the line
// true, up to FixTasksPerLine times, and after that its done list stays
// unproved and the person is told which line, so a job never closes done on a
// check the person wrote and the harness saw fail. A line with no check is
// proved by the tasks at the close, the rule the store has always kept.

// FixTasksPerLine is how many tasks a job gives itself for one checked line
// that stays red before it stops trying and says so.
const FixTasksPerLine = 3

// theFixTaskOpening opens the task a job gives itself for a red check, and is
// what the job counts to hold itself to FixTasksPerLine.
const theFixTaskOpening = "Done line %d is not true yet"

// JobProof is what the harness proved of a job's done list as one of its
// tasks ended: the lines whose checks passed, the lines whose checks failed
// with what each showed, and the lines that carry no check.
type JobProof struct {
	// Proved is the numbers, from one, of the lines whose checks passed.
	Proved []int
	// Failed is what the check of each failed line showed, by line number.
	Failed map[int]string
	// Unchecked is the numbers of the lines the harness cannot check.
	Unchecked []int
}

// proveTheJobsDoneLines runs the job's bracketed checks as a task of the job
// ends, and is nil for a task that belongs to no job.
func (running *run) proveTheJobsDoneLines(ctx context.Context) (*JobProof, error) {
	if running.task.FromJob == nil || running.theLoop.options.Jobs == nil {
		return nil, nil
	}
	held, err := running.theLoop.options.Jobs.Load(ctx, running.task.FromJob.JobID)
	if err != nil {
		return nil, fmt.Errorf("cannot read job %s to prove its done list: %w", running.task.FromJob.JobID, err)
	}
	proof := &JobProof{Failed: map[int]string{}}
	for at, line := range held.Goal.DoneWhen {
		check, found := workorder.ReadCheck(line.Text)
		if !found {
			proof.Unchecked = append(proof.Unchecked, at+1)
			continue
		}
		shown, err := running.runTheCheck(ctx, check)
		if err != nil {
			return nil, err
		}
		if shown.passed {
			proof.Proved = append(proof.Proved, at+1)
			continue
		}
		problem := shown.said
		if tail := tailOf(shown.output); tail != "" {
			problem += "\n" + tail
		}
		proof.Failed[at+1] = problem
	}
	return proof, nil
}

// markTheJobsDoneLines writes what a task's ending proved into the job: each
// passed line marked with the task's report, each failed line unmarked, and,
// once every task is done, each unchecked line marked with the report too.
func (theLoop *Loop) markTheJobsDoneLines(ctx context.Context, jobID string, reportID string, proof *JobProof) error {
	if proof == nil {
		return nil
	}
	for _, number := range proof.Proved {
		if err := theLoop.options.Jobs.ProveDoneLine(ctx, jobID, number, reportID); err != nil {
			return fmt.Errorf("cannot mark done line %d of job %s: %w", number, jobID, err)
		}
	}
	for number := range proof.Failed {
		if err := theLoop.options.Jobs.ProveDoneLine(ctx, jobID, number, ""); err != nil {
			return fmt.Errorf("cannot unmark done line %d of job %s: %w", number, jobID, err)
		}
	}
	held, err := theLoop.options.Jobs.Load(ctx, jobID)
	if err != nil || !everyTaskIsDone(held) {
		return err
	}
	for _, number := range proof.Unchecked {
		if held.Goal.DoneWhen[number-1].Done {
			continue
		}
		if err := theLoop.options.Jobs.ProveDoneLine(ctx, jobID, number, reportID); err != nil {
			return fmt.Errorf("cannot mark done line %d of job %s on its tasks: %w", number, jobID, err)
		}
	}
	return nil
}

// giveTheJobAFixTask adds one task for the first red check that has not had
// FixTasksPerLine tasks yet, and says whether it added one.
func (theLoop *Loop) giveTheJobAFixTask(ctx context.Context, jobID string, held contract.Record, proof *JobProof) (bool, error) {
	if proof == nil {
		return false, nil
	}
	for number := 1; number <= len(held.Goal.DoneWhen); number++ {
		problem, failed := proof.Failed[number]
		if !failed || fixTasksFor(held, number) >= FixTasksPerLine {
			continue
		}
		line := held.Goal.DoneWhen[number-1]
		text := fmt.Sprintf(theFixTaskOpening+": %s. Make it true, then prove it with its check: %s",
			number, firstLineOf(problem), line.Text)
		if _, err := theLoop.options.Jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: text}); err != nil {
			return false, fmt.Errorf("cannot give job %s a task for its red check: %w", jobID, err)
		}
		return true, nil
	}
	return false, nil
}

// fixTasksFor counts the tasks the job has already given itself for one line.
func fixTasksFor(held contract.Record, number int) int {
	count := 0
	opening := fmt.Sprintf(theFixTaskOpening, number)
	for _, task := range held.Work.Tasks {
		if strings.HasPrefix(task.Text, opening) {
			count++
		}
	}
	return count
}

// whatStaysRed is the line under a job's closing report naming each check
// that failed and what it showed.
func whatStaysRed(proof *JobProof, held contract.Record) string {
	if proof == nil || len(proof.Failed) == 0 {
		return ""
	}
	parts := []string{}
	for number := 1; number <= len(held.Goal.DoneWhen); number++ {
		if problem, failed := proof.Failed[number]; failed {
			parts = append(parts, fmt.Sprintf("Done line %d, %q: %s", number, held.Goal.DoneWhen[number-1].Text, firstLineOf(problem)))
		}
	}
	return strings.Join(parts, "\n")
}

// firstLineOf is the first line of a text.
func firstLineOf(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return first
}

// provedCountLine is the line the person reads under a job task's report,
// when the job's done list carries checks.
func provedCountLine(held contract.Record) string {
	proved, checked := record.ProvedCheckedLines(held.Goal.DoneWhen)
	if checked == 0 {
		return ""
	}
	return fmt.Sprintf("Done lines proved: %d of %d.", proved, checked)
}
