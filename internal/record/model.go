package record

import (
	"context"
	"fmt"
	"slices"

	"github.com/JaredTate/coeus/internal/contract"
)

// Update is everything the model may write into a record, and the only way it
// can write anything at all. The task tool and the job tool of wave 2 hand one
// of these over, in the same reply as the model's other tool calls, so that
// keeping the record costs no extra call.
//
// There is no field here for the ask, for a correction, for the header, or for
// the situation, because those belong to the user and to the harness. A list
// left at nothing is left alone; a list set to an empty one is cleared.
type Update struct {
	// Why is the one line on why the user wants this, which is set once.
	Why string
	// DoneWhen is the whole done list, each line with the result or the user's
	// reply that proves it, when it has one.
	DoneWhen []contract.DoneLine
	// StopWhen is the whole stop list.
	StopWhen []string
	// Plan is a task's plan, one line per step, numbered from one in the order
	// they are written.
	Plan []string
	// Tasks is a job's whole task list, in order.
	Tasks []NewJobTask
	// Decision is one choice to add, with the reason it must carry.
	Decision *NewDecision
	// Failure is one thing that went wrong to add, with the cause it must carry.
	Failure *NewFailure
}

// NewDecision is a choice the model made, with the reason it must carry so that
// the model does not argue with itself later.
type NewDecision struct {
	// Text is the choice.
	Text string
	// Reason is why it was made.
	Reason string
}

// NewFailure is something that went wrong, with the cause it must carry so that
// it is not repeated.
type NewFailure struct {
	// Text is what went wrong.
	Text string
	// Cause is why it went wrong.
	Cause string
}

// NewJobTask is one task on a job's list as the model writes it. The check mark
// and the report behind it are the harness's to write.
type NewJobTask struct {
	// TaskID is the task's label, such as "t31".
	TaskID string
	// Text says what the task does, with one clear done line behind it.
	Text string
	// DueAt is the date it must wait for, in plain words such as "today at
	// 14:00", and is empty when it may start as soon as the ones before it are
	// done.
	DueAt string
}

// nothingToWrite says whether an update asks for no change at all, which is what
// most rounds hand over.
func (update Update) nothingToWrite() bool {
	return update.Why == "" && update.DoneWhen == nil && update.StopWhen == nil &&
		update.Plan == nil && update.Tasks == nil && update.Decision == nil && update.Failure == nil
}

// Apply writes the model's half of the record. It is all or nothing: every rule
// is checked before anything changes, so a refused update leaves the record
// exactly as it was and the model gets back one line naming the rule it broke.
func (keeper *Keeper) Apply(ctx context.Context, update Update) error {
	if update.nothingToWrite() {
		return nil
	}
	return keeper.change(ctx, func(into *contract.Record) error {
		return applyUpdate(into, update)
	})
}

// change makes one change on a copy of the record and only lets the copy become
// the record once the checkpoint behind it is saved, so that the record and the
// log never disagree about what happened.
func (keeper *Keeper) change(ctx context.Context, write func(into *contract.Record) error) error {
	changing := cloneRecord(keeper.record)
	if err := write(&changing); err != nil {
		return err
	}
	if err := checkItReadsBack(changing); err != nil {
		return err
	}
	held := keeper.record
	keeper.record = changing
	if err := keeper.save(ctx); err != nil {
		keeper.record = held
		return err
	}
	return nil
}

// applyUpdate runs every part of an update through its rules and writes it.
func applyUpdate(into *contract.Record, update Update) error {
	for _, write := range []func(*contract.Record, Update) error{
		applyWhy, applyDoneWhen, applyStopWhen, applyPlan, applyTasks, applyDecision, applyFailure,
	} {
		if err := write(into, update); err != nil {
			return err
		}
	}
	return nil
}

// applyWhy holds the rule that the why is set once and then stands, because it is
// what lets the model act sensibly when the plan breaks.
func applyWhy(into *contract.Record, update Update) error {
	if update.Why == "" || update.Why == into.Goal.Why {
		return nil
	}
	if into.Goal.Why != "" {
		return fmt.Errorf("the why already reads %q, and it is written once: %w", into.Goal.Why, ErrWhyIsSet)
	}
	into.Goal.Why = update.Why
	return nil
}

// applyDoneWhen holds the rule that a done line marked done names the result that
// proves it, or the user's reply that stands for one.
func applyDoneWhen(into *contract.Record, update Update) error {
	if update.DoneWhen == nil {
		return nil
	}
	for _, line := range update.DoneWhen {
		if line.Text == "" {
			return fmt.Errorf("one of the %d done lines has no text, so say in one line what must be true", len(update.DoneWhen))
		}
		if line.Done && line.ResultID == "" && line.UserReply == "" {
			return fmt.Errorf("the done line %q is marked done and names nothing behind it: %w", line.Text, ErrDoneLineNeedsProof)
		}
		if line.ResultID != "" && line.UserReply != "" {
			return fmt.Errorf("the done line %q names both a result and a reply from the user, so name the one that proves it", line.Text)
		}
		if err := checkNamesAResult(into, line.ResultID, "the done line "+line.Text); err != nil {
			return err
		}
	}
	into.Goal.DoneWhen = keepOrDrop(slices.Clone(update.DoneWhen))
	return nil
}

// applyStopWhen writes the short list of things that stop the work at once.
func applyStopWhen(into *contract.Record, update Update) error {
	if update.StopWhen == nil {
		return nil
	}
	for _, stop := range update.StopWhen {
		if stop == "" {
			return fmt.Errorf("one of the %d stop lines is empty, so write it or leave it out", len(update.StopWhen))
		}
	}
	into.Rules.StopWhen = keepOrDrop(slices.Clone(update.StopWhen))
	return nil
}

// applyPlan writes a task's plan, numbering the steps from one, and keeps the
// mark and the result of any step whose words did not change, so that editing a
// plan never throws away the proof of the work already done.
func applyPlan(into *contract.Record, update Update) error {
	if update.Plan == nil {
		return nil
	}
	if into.Header.Kind != contract.RecordTask {
		return fmt.Errorf("a plan belongs to a task and this is a %s, which has a task list: %w", into.Header.Kind, ErrWrongKind)
	}
	steps := make([]contract.PlanStep, 0, len(update.Plan))
	for at, text := range update.Plan {
		if text == "" {
			return fmt.Errorf("the plan step numbered %d has no text, so say what the step does", at+1)
		}
		step := contract.PlanStep{Number: at + 1, Text: text}
		if before, found := planStepSaying(into.Work.Plan, text); found && before.Done {
			step.Done, step.ResultID = true, before.ResultID
		}
		steps = append(steps, step)
	}
	into.Work.Plan = keepOrDrop(steps)
	return nil
}

// applyTasks writes a job's task list under its three rules: every label is a
// task label, the labels count upwards, and a task that is finished stays.
func applyTasks(into *contract.Record, update Update) error {
	if update.Tasks == nil {
		return nil
	}
	if into.Header.Kind != contract.RecordJob {
		return fmt.Errorf("a task list belongs to a job and this is a %s, which has a plan: %w", into.Header.Kind, ErrWrongKind)
	}
	written, err := writtenTasks(into, update.Tasks)
	if err != nil {
		return err
	}
	for _, before := range into.Work.Tasks {
		if before.Done && !slices.ContainsFunc(written, func(task contract.JobTask) bool { return task.TaskID == before.TaskID }) {
			return fmt.Errorf("the task %s is finished, and a finished task is never taken off the list: %w",
				before.TaskID, ErrFinishedTaskRemoved)
		}
	}
	into.Work.Tasks = keepOrDrop(written)
	return nil
}

// writtenTasks turns the task list the model wrote into the record's own, keeping
// the check mark and the report of every task that is already finished.
func writtenTasks(into *contract.Record, tasks []NewJobTask) ([]contract.JobTask, error) {
	written := make([]contract.JobTask, 0, len(tasks))
	highest := 0
	for _, task := range tasks {
		number, isTaskID := contract.ParseTaskID(task.TaskID)
		if !isTaskID {
			return nil, fmt.Errorf("the task label %q is not one a job writes, so use one such as %q", task.TaskID, contract.TaskID(31))
		}
		if task.Text == "" {
			return nil, fmt.Errorf("the task %s has no text, so say what it does in one line", task.TaskID)
		}
		if number <= highest {
			return nil, fmt.Errorf("the task %s comes after a task numbered %d: %w", task.TaskID, highest, ErrTasksOutOfOrder)
		}
		highest = number
		entry := contract.JobTask{TaskID: task.TaskID, Text: task.Text, DueAt: task.DueAt}
		if before, found := jobTaskLabelled(into.Work.Tasks, task.TaskID); found && before.Done {
			entry.Done, entry.ReportID = true, before.ReportID
		}
		written = append(written, entry)
	}
	return written, nil
}

// applyDecision adds one choice with the reason it must carry.
func applyDecision(into *contract.Record, update Update) error {
	if update.Decision == nil {
		return nil
	}
	if update.Decision.Text == "" {
		return fmt.Errorf("this decision says nothing was chosen, so write the choice in one line")
	}
	if update.Decision.Reason == "" {
		return fmt.Errorf("the decision %q carries no reason: %w", update.Decision.Text, ErrDecisionNeedsReason)
	}
	into.Lessons.Decisions = append(into.Lessons.Decisions, contract.Decision{
		ID:     contract.DecisionID(len(into.Lessons.Decisions) + 1),
		Text:   update.Decision.Text,
		Reason: update.Decision.Reason,
	})
	return nil
}

// applyFailure adds one thing that went wrong with the cause it must carry.
func applyFailure(into *contract.Record, update Update) error {
	if update.Failure == nil {
		return nil
	}
	if update.Failure.Text == "" {
		return fmt.Errorf("this failure says nothing went wrong, so write what went wrong in one line")
	}
	if update.Failure.Cause == "" {
		return fmt.Errorf("the failure %q carries no cause: %w", update.Failure.Text, ErrFailureNeedsCause)
	}
	into.Lessons.Failures = append(into.Lessons.Failures, contract.Failure{
		ID:    contract.FailureID(len(into.Lessons.Failures) + 1),
		Text:  update.Failure.Text,
		Cause: update.Failure.Cause,
	})
	return nil
}

// checkNamesAResult says no to a label that names a result this record never
// wrote, because proof that points at nothing is no proof.
func checkNamesAResult(into *contract.Record, id string, what string) error {
	if id == "" || recordHoldsResult(into, id) {
		return nil
	}
	return fmt.Errorf("%s names %q, which this record never wrote: %w", what, id, ErrNoSuchResult)
}

// recordHoldsResult says whether the record holds a result under this label.
func recordHoldsResult(into *contract.Record, id string) bool {
	if id == "" {
		return false
	}
	return slices.ContainsFunc(into.Work.Results, func(result contract.ResultLine) bool { return result.ID == id })
}

// planStepSaying finds the plan step whose words are these, if there is one.
func planStepSaying(plan []contract.PlanStep, text string) (contract.PlanStep, bool) {
	for _, step := range plan {
		if step.Text == text {
			return step, true
		}
	}
	return contract.PlanStep{}, false
}

// jobTaskLabelled finds the task with this label, if the job has one.
func jobTaskLabelled(tasks []contract.JobTask, id string) (contract.JobTask, bool) {
	for _, task := range tasks {
		if task.TaskID == id {
			return task, true
		}
	}
	return contract.JobTask{}, false
}
