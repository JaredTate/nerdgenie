package testkit

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// CheckJobProvesDoneLines asserts the part of the job contract that a work
// order needs: a job made with a done list keeps it, unmarked, and does not
// close on its last task while a line is unproved; the harness marks one line
// at a time with the report that proves it, or unmarks it with no result; a
// line or a job that is not there is an error naming it; and the job closes
// once every line is marked and every task is done. It makes one job of its
// own.
func CheckJobProvesDoneLines(ctx context.Context, jobs contract.Job) error {
	lines := []string{"Every test passes. [tests pass: npm test]", "The page shows the board."}
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "build the game and prove it", Name: "the proved job", Why: "to check the contract", DoneWhen: lines})
	if err != nil {
		return fmt.Errorf("creating a job with a done list failed: %w", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "build the game"})
	if err != nil {
		return fmt.Errorf("adding the task failed: %w", err)
	}
	if err := checkTheListIsKeptUnmarked(ctx, jobs, jobID, lines); err != nil {
		return err
	}
	reportID, err := jobs.FinishTask(ctx, jobID, taskID, "the game is built", false)
	if err != nil {
		return fmt.Errorf("finishing the task failed: %w", err)
	}
	held, err := jobs.Load(ctx, jobID)
	if err != nil {
		return err
	}
	if held.Header.Status == contract.StatusDone || len(held.Goal.DoneWhen) != 2 {
		return fmt.Errorf("after its last task the job reads %q with %d done lines, want it still running with its two unproved lines kept", held.Header.Status, len(held.Goal.DoneWhen))
	}
	return checkTheMarks(ctx, jobs, jobID, reportID)
}

// checkTheListIsKeptUnmarked reads the new job back and holds its done list
// to the lines it was given, none of them marked.
func checkTheListIsKeptUnmarked(ctx context.Context, jobs contract.Job, jobID string, lines []string) error {
	held, err := jobs.Load(ctx, jobID)
	if err != nil {
		return fmt.Errorf("loading the job failed: %w", err)
	}
	if len(held.Goal.DoneWhen) != len(lines) {
		return fmt.Errorf("the job holds %d done lines, want the %d it was made with", len(held.Goal.DoneWhen), len(lines))
	}
	for at, line := range held.Goal.DoneWhen {
		if line.Text != lines[at] || line.Done || line.ResultID != "" {
			return fmt.Errorf("done line %d reads %+v, want %q unmarked", at+1, line, lines[at])
		}
	}
	return nil
}

// checkTheMarks marks, unmarks, refuses, and closes.
func checkTheMarks(ctx context.Context, jobs contract.Job, jobID string, reportID string) error {
	if err := jobs.ProveDoneLine(ctx, "no-such-job", 1, reportID); err == nil {
		return errors.New("marking a line of a job that is not there returned no error")
	}
	if err := jobs.ProveDoneLine(ctx, jobID, 3, reportID); err == nil {
		return errors.New("marking done line 3 of a two-line list returned no error")
	}
	if err := jobs.ProveDoneLine(ctx, jobID, 1, reportID); err != nil {
		return fmt.Errorf("marking done line 1 with %s failed: %w", reportID, err)
	}
	held, err := jobs.Load(ctx, jobID)
	if err != nil {
		return err
	}
	if first := held.Goal.DoneWhen[0]; !first.Done || first.ResultID != reportID {
		return fmt.Errorf("done line 1 reads %+v after the mark, want it done and pointing at %s", first, reportID)
	}
	if held.Header.Status == contract.StatusDone {
		return errors.New("the job closed with its second done line unproved")
	}
	if err := jobs.ProveDoneLine(ctx, jobID, 1, ""); err != nil {
		return fmt.Errorf("unmarking done line 1 failed: %w", err)
	}
	if held, _ = jobs.Load(ctx, jobID); held.Goal.DoneWhen[0].Done {
		return errors.New("done line 1 is still marked after it was unmarked")
	}
	for number := 1; number <= 2; number++ {
		if err := jobs.ProveDoneLine(ctx, jobID, number, reportID); err != nil {
			return fmt.Errorf("marking done line %d failed: %w", number, err)
		}
	}
	if held, _ = jobs.Load(ctx, jobID); held.Header.Status != contract.StatusDone {
		return fmt.Errorf("the job reads %q with every line proved and every task done, want done", held.Header.Status)
	}
	return nil
}
