package testkit

import (
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// The three assertions the forty-step fixture makes are here, so that wave 1 and
// wave 3 and the live suite all call the same code rather than each writing its
// own idea of what "works on any model" means.

// RecordAtTheEnd is the record the fixture says the harness should have built by
// the time the task finishes. A test compares the record the harness actually
// built against this one, or hands this one to the three checks below to prove
// the checks themselves work.
//
// It carries everything the model wrote through the task tool: the goal, the
// rules, the plan, the decisions and the failures. The situation is left empty,
// because those are the few facts the harness checks for itself and the fixture
// has nothing to say about them.
func (task FortyStepTask) RecordAtTheEnd() contract.Record {
	results := task.ToolResults()
	lines := make([]contract.ResultLine, 0, len(results))
	for _, result := range results {
		lines = append(lines, contract.ResultLine{ID: result.ID, Summary: result.Summary})
	}

	return contract.Record{
		Header: contract.Header{
			Kind:   contract.RecordTask,
			ID:     task.TaskID,
			Status: contract.StatusDone,
			Origin: task.Origin,
		},
		Goal: contract.Goal{Ask: task.Ask, Why: task.Why, DoneWhen: task.DoneLinesAtTheEnd()},
		Rules: contract.Rules{
			Corrections: []contract.Correction{{ID: contract.CorrectionID(1), Text: task.Correction}},
			StopWhen:    task.StopWhen,
		},
		Work: contract.Work{
			Plan:    task.PlanAtTheEnd(),
			Results: lines,
		},
		Lessons: contract.Lessons{
			Decisions: task.DecisionsAtTheEnd(),
			Failures:  task.FailuresAtTheEnd(),
		},
	}
}

// CheckAskAndCorrections is the first assertion: the ask and every correction
// are byte for byte what the user wrote, at the end of forty rounds.
func (task FortyStepTask) CheckAskAndCorrections(record contract.Record) error {
	if record.Goal.Ask != task.Ask {
		return fmt.Errorf("the ask is now %q, and the user wrote %q, so something edited it", record.Goal.Ask, task.Ask)
	}
	if len(record.Rules.Corrections) != 1 {
		return fmt.Errorf("the record holds %d corrections, want the one the user sent at round %d",
			len(record.Rules.Corrections), task.CorrectionRound)
	}
	if record.Rules.Corrections[0].Text != task.Correction {
		return fmt.Errorf("the correction is now %q, and the user wrote %q, so something rewrote it",
			record.Rules.Corrections[0].Text, task.Correction)
	}
	return nil
}

// CheckDoneList is the second assertion: the done-check passes, meaning every
// line of the done list points at a result or at a reply from the user.
func (task FortyStepTask) CheckDoneList(record contract.Record) error {
	if len(record.Goal.DoneWhen) != len(task.DoneWhen) {
		return fmt.Errorf("the done list has %d lines, want the %d the fixture wrote",
			len(record.Goal.DoneWhen), len(task.DoneWhen))
	}
	for _, line := range record.Goal.DoneWhen {
		if !line.Done {
			return fmt.Errorf("the done line %q is not marked done, so the task cannot close", line.Text)
		}
		if line.ResultID == "" && line.UserReply == "" {
			return fmt.Errorf("the done line %q has nothing behind it, so name the result that proves it", line.Text)
		}
	}
	return nil
}

// CheckResultsReadable is the third assertion: every result the task produced,
// from r1 upwards, can still be read back in full by its id, even though most of
// them left the working context long ago.
func (task FortyStepTask) CheckResultsReadable(read func(id string) (string, error)) error {
	if read == nil {
		return errors.New("no reader was given to the result check, so pass the function that reads a result by its id")
	}
	for _, result := range task.ToolResults() {
		text, err := read(result.ID)
		if err != nil {
			return fmt.Errorf("the result %s cannot be read back: %w", result.ID, err)
		}
		if text != result.Text {
			return fmt.Errorf("the result %s reads back as %q, and the fixture wrote %q", result.ID, text, result.Text)
		}
	}
	return nil
}
