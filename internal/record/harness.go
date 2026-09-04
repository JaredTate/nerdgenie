package record

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The methods in this file are the harness's half of a record: everything
// ordinary code can work out for itself, written with no model call and no
// tokens. The model's half goes through Apply, and there is no way from there to
// anything here, which is how rules seven and eight are kept: the model never
// writes the header, the situation, the corrections, or the results.

// SetBudget writes how much of a task's budget is left, on each of the two
// limits the task may have. A limit that is off is written as off, and the count
// on it is not kept.
func (keeper *Keeper) SetBudget(ctx context.Context, left Budget) error {
	if err := keeper.mustBe(contract.RecordTask, "a budget"); err != nil {
		return err
	}
	return keeper.change(ctx, func(into *contract.Record) error {
		if left.RoundsLeft < 0 || left.MinutesLeft < 0 {
			return fmt.Errorf("the budget left is %d rounds and %d minutes, and neither may be below zero", left.RoundsLeft, left.MinutesLeft)
		}
		left = left.kept()
		into.Header.RoundsLeft, into.Header.NoRoundBudget = left.RoundsLeft, left.NoRoundBudget
		into.Header.MinutesLeft, into.Header.NoTimeBudget = left.MinutesLeft, left.NoTimeBudget
		return nil
	})
}

// SetCost writes what the last turn cost. The record keeps the three counts to
// the tenth of a thousand tokens it prints, and nothing finer, so that the record
// and its text always say the same thing; the exact counts stay in the log.
func (keeper *Keeper) SetCost(ctx context.Context, cost contract.CostLine) error {
	if err := keeper.mustBe(contract.RecordTask, "a cost line"); err != nil {
		return err
	}
	return keeper.change(ctx, func(into *contract.Record) error {
		if cost.InputTokens < 0 || cost.CachedInputTokens < 0 || cost.OutputTokens < 0 {
			return fmt.Errorf("the turn cost %d tokens in and %d out, and no count of tokens may be below zero",
				cost.InputTokens, cost.OutputTokens)
		}
		into.Header.Cost = contract.CostLine{
			InputTokens:       roundToHundred(cost.InputTokens),
			CachedInputTokens: roundToHundred(cost.CachedInputTokens),
			OutputTokens:      roundToHundred(cost.OutputTokens),
		}
		return nil
	})
}

// SetProgress writes how many of a job's tasks are done and which one is due
// next.
func (keeper *Keeper) SetProgress(ctx context.Context, done int, total int, nextDue string) error {
	if err := keeper.mustBe(contract.RecordJob, "a progress line"); err != nil {
		return err
	}
	return keeper.change(ctx, func(into *contract.Record) error {
		if done < 0 || total < 0 || done > total {
			return fmt.Errorf("the progress is %d of %d tasks done, and that is not a count of finished tasks out of the whole", done, total)
		}
		into.Header.TasksDone, into.Header.TasksTotal = done, total
		into.Header.NextDue = nextDue
		return nil
	})
}

// SetSituation writes the few facts the harness can check on its own: the page
// the browser is on, the files changed, and the last command and how it went.
func (keeper *Keeper) SetSituation(ctx context.Context, facts []string) error {
	return keeper.change(ctx, func(into *contract.Record) error {
		for _, fact := range facts {
			if fact == "" {
				return fmt.Errorf("one of the %d lines of the situation is empty, so write it or leave it out", len(facts))
			}
		}
		into.Work.Situation = keepOrDrop(slices.Clone(facts))
		return nil
	})
}

// AddCorrection appends something the user said while the work was running, in
// the user's own words, with the next label. Nothing ever edits or removes one.
func (keeper *Keeper) AddCorrection(ctx context.Context, said string) (string, error) {
	if said == "" {
		return "", fmt.Errorf("a correction is something the user said, and this one is empty: %w", ErrCorrectionIsFixed)
	}
	id := ""
	err := keeper.change(ctx, func(into *contract.Record) error {
		id = contract.CorrectionID(len(into.Rules.Corrections) + 1)
		into.Rules.Corrections = append(into.Rules.Corrections, contract.Correction{ID: id, Text: said})
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// AddResult gives a task result the next label, keeps one line for it in the
// record, and writes its whole text to the log, which is what Read brings back
// after the text itself has left the model's window.
func (keeper *Keeper) AddResult(ctx context.Context, summary string, text string) (string, error) {
	if err := keeper.mustBe(contract.RecordTask, "a task result"); err != nil {
		return "", err
	}
	return keeper.addResultLine(ctx, summary, text)
}

// AddReport gives a finished task's report the next label inside its job, and
// keeps its whole text in the log the same way a task result is kept.
func (keeper *Keeper) AddReport(ctx context.Context, summary string, text string) (string, error) {
	if err := keeper.mustBe(contract.RecordJob, "a report"); err != nil {
		return "", err
	}
	return keeper.addResultLine(ctx, summary, text)
}

// addResultLine holds what a result and a report do alike.
func (keeper *Keeper) addResultLine(ctx context.Context, summary string, text string) (string, error) {
	if summary == "" {
		return "", fmt.Errorf("a result keeps one line in the record and this one is empty, so say in a few words what it was")
	}
	line := cutToOneLine(summary)
	id, err := keeper.nextResultLabel(ctx)
	if err != nil {
		return "", err
	}
	if err := keeper.storeResult(ctx, StoredResult{ID: id, Summary: line, Text: text}); err != nil {
		return "", err
	}
	written := keeper.change(ctx, func(into *contract.Record) error {
		into.Work.Results = append(into.Work.Results, contract.ResultLine{ID: id, Summary: line})
		return nil
	})
	if written != nil {
		return "", written
	}
	return id, nil
}

// nextResultLabel is the label the next result takes: one above the highest this
// record has ever written, read from the log rather than from the record in hand.
// It matters after a wind-back, where the record in hand has fewer results than
// the log remembers: handing out a label twice would leave the two paths' evidence
// under one name, and a replay of the abandoned one would read the wrong text.
func (keeper *Keeper) nextResultLabel(ctx context.Context) (string, error) {
	events, err := keeper.store.ByTask(ctx, keeper.LogKey())
	if err != nil {
		return "", fmt.Errorf("cannot read the log of %s %s to label the next result: %w", keeper.Kind(), keeper.ID(), err)
	}
	highest := keeper.highestResultNumber()
	for _, event := range events {
		if event.Kind != contract.EventToolResult {
			continue
		}
		stored := StoredResult{}
		if err := json.Unmarshal(event.Body, &stored); err != nil {
			continue
		}
		if number, valid := ResultNumber(keeper.record.Header, stored.ID); valid && number > highest {
			highest = number
		}
	}
	return nextResultID(keeper.record.Header, highest), nil
}

// storeResult writes the whole text of one result into the log before the
// record's own line is added, so that a crash between the two leaves the text
// findable rather than lost. That is the obligation-first idea from Hermes'
// delivery ledger at ~/Code/hermes-agent/gateway/delivery_ledger.py, written
// fresh here. If the checkpoint after it fails, the same label can be written
// twice, so Read takes the last one written, which is the retry.
func (keeper *Keeper) storeResult(ctx context.Context, result StoredResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("cannot write the result %s as JSON: %w", result.ID, err)
	}
	event := contract.Event{TaskID: keeper.LogKey(), Kind: contract.EventToolResult, Body: body}
	if _, err := keeper.store.Append(ctx, event); err != nil {
		return fmt.Errorf("cannot write the result %s to the log: %w", result.ID, err)
	}
	return nil
}

// Pin marks one result as evidence that is kept in front of the model word for
// word until it is let go of, or takes that mark off again. It is a mark and
// not a piece of writing, so the harness makes it and it costs no model call.
//
// It goes through the same change path as every other edit, which is what makes
// a pin survive a wait: the checkpoint behind it carries the mark, so a task put
// down for days is picked up with the same evidence in front of it.
func (keeper *Keeper) Pin(ctx context.Context, id string, pinned bool) error {
	return keeper.change(ctx, func(into *contract.Record) error {
		for at := range into.Work.Results {
			if into.Work.Results[at].ID != id {
				continue
			}
			into.Work.Results[at].Pinned = pinned
			return nil
		}
		return fmt.Errorf("%q would be pinned and this %s never wrote it, so name a result it holds: %w",
			id, keeper.Kind(), ErrNoSuchResult)
	})
}

// MarkPlanStep marks one step of a task's plan done and points it at the result
// that proves it. It is a check mark, so the harness writes it and it costs no
// model call.
func (keeper *Keeper) MarkPlanStep(ctx context.Context, number int, resultID string) error {
	if err := keeper.mustBe(contract.RecordTask, "a plan step"); err != nil {
		return err
	}
	return keeper.change(ctx, func(into *contract.Record) error {
		if number < 1 || number > len(into.Work.Plan) {
			return fmt.Errorf("there is no plan step numbered %d, and this plan has %d steps in it", number, len(into.Work.Plan))
		}
		if !recordHoldsResult(into, resultID) {
			return fmt.Errorf("the plan step numbered %d would be marked done by %q, which this record never wrote: %w",
				number, resultID, ErrPlanStepNeedsResult)
		}
		into.Work.Plan[number-1].Done = true
		into.Work.Plan[number-1].ResultID = resultID
		return nil
	})
}

// MarkJobTask marks one task on a job's list done and points it at the report it
// wrote, which is the same check mark on the job's side.
func (keeper *Keeper) MarkJobTask(ctx context.Context, taskID string, reportID string) error {
	if err := keeper.mustBe(contract.RecordJob, "a check mark on a task"); err != nil {
		return err
	}
	return keeper.change(ctx, func(into *contract.Record) error {
		if _, found := jobTaskLabelled(into.Work.Tasks, taskID); !found {
			return fmt.Errorf("the task %s is not on this job's list, which holds %d tasks", taskID, len(into.Work.Tasks))
		}
		if !recordHoldsResult(into, reportID) {
			return fmt.Errorf("the task %s would be marked done by %q, which this job never wrote: %w",
				taskID, reportID, ErrJobTaskNeedsReport)
		}
		markTaskDone(into, taskID, reportID)
		return nil
	})
}

// markTaskDone puts the check mark and the report on the task with this label.
func markTaskDone(into *contract.Record, taskID string, reportID string) {
	for at := range into.Work.Tasks {
		if into.Work.Tasks[at].TaskID == taskID {
			into.Work.Tasks[at].Done = true
			into.Work.Tasks[at].ReportID = reportID
			return
		}
	}
}

// Read brings back the whole text of a result by its label, which is the third
// tier of the design: the line stays in the record, the text stays in the log,
// and only the copy in the model's window ever leaves.
//
// AskLabel is read the same way and answered from the record itself rather than
// the log, because the ask is written once at the start and never changes. It is
// what the note beside a shortened ask tells the model to call.
func (keeper *Keeper) Read(ctx context.Context, id string) (string, error) {
	if id == AskLabel {
		return keeper.record.Goal.Ask, nil
	}
	if _, valid := ResultNumber(keeper.record.Header, id); !valid {
		return "", fmt.Errorf("%q is not a label this %s writes, so read one such as %q or %q: %w",
			id, keeper.Kind(), nextResultID(keeper.record.Header, 0), AskLabel, ErrNoSuchResult)
	}
	events, err := keeper.store.ByTask(ctx, keeper.LogKey())
	if err != nil {
		return "", fmt.Errorf("cannot read the log of %s %s to find %s: %w", keeper.Kind(), keeper.ID(), id, err)
	}
	text, found := "", false
	for _, event := range events {
		if event.Kind != contract.EventToolResult {
			continue
		}
		stored := StoredResult{}
		if err := json.Unmarshal(event.Body, &stored); err != nil {
			continue
		}
		if stored.ID == id {
			text, found = stored.Text, true
		}
	}
	if !found {
		return "", fmt.Errorf("the result %s is not in the log of %s %s: %w", id, keeper.Kind(), keeper.ID(), ErrNoSuchResult)
	}
	return text, nil
}

// highestResultNumber is the largest label number the results have reached, which
// is what the next one counts up from.
func (keeper *Keeper) highestResultNumber() int {
	highest := 0
	for _, result := range keeper.record.Work.Results {
		if number, valid := ResultNumber(keeper.record.Header, result.ID); valid && number > highest {
			highest = number
		}
	}
	return highest
}

// roundToHundred rounds a token count to the nearest hundred, which is the
// precision the cost line prints.
func roundToHundred(tokens int) int {
	if tokens < 0 {
		return 0
	}
	return (tokens + 50) / 100 * 100
}

// keepOrDrop returns nothing at all for an empty list, so that a list the caller
// cleared and a list that was never written print and read the same way.
func keepOrDrop[Item any](items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	return items
}
