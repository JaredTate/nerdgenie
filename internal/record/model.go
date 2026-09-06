package record

import (
	"context"
	"fmt"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
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
	// Name is a short name for the work, a few words, set once. The job tool
	// writes it when a job is made so the job list and the side panel can name
	// the job without the whole ask.
	Name string
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
	// StepDone marks one step of a task's plan done and points it at the
	// result that proves it, which is the check mark the side panel draws.
	StepDone *StepDone
}

// StepDone is one plan step to mark done: its number, counting from one, and
// the result that proves it.
type StepDone struct {
	// Number is which step, counting from one.
	Number int
	// ResultID is the result that proves the step, such as r7.
	ResultID string
}

// MaxDoneLines is how many lines a task's done list may hold. A task is one
// sitting of work, and five things that must be true at its end is what one
// sitting can prove; an ask that needs more is a job, with one task per line.
// The record refuses the sixth line rather than leaving the call to the model,
// because whether an ask is too big for one task was the model's judgment
// before, and a small model never said yes. A job's own done list is not held
// to this, because a job is made of many sittings.
const MaxDoneLines = 5

// MaxPlanSteps is how many steps a task's plan may hold. A task is one sitting
// of work, and ten steps is what one sitting works through: the forty-round
// tweet of the forty-step fixture, which is the design's own picture of one
// sitting, plans in ten. A plan longer than that is a job's task list in
// disguise, and the ask behind it is a job with one task per step. The record
// refuses the eleventh step rather than leaving the call to the model, because
// a small model never says on its own that an ask is too big for one task:
// given a whole game with its hazards, its animations, its tests and its
// play-testing, which section 4 of NERDGENIE.md says is a job, one kept the
// done list at five lines and hid the whole build in a twelve-step plan. A job
// is not held to this, because a job has a task list and no plan.
const MaxPlanSteps = 10

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
	return update.Name == "" && update.Why == "" && update.DoneWhen == nil && update.StopWhen == nil &&
		update.Plan == nil && update.Tasks == nil && update.Decision == nil && update.Failure == nil && update.StepDone == nil
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
//
// A keeper whose owner marks the rounds keeps the change in hand instead and
// waits for SaveTheRound, so that one round of work costs one checkpoint rather
// than one for the budget, one for the cost, one for every result and one for
// the situation. Nothing is lost by waiting: the whole text of every result is
// already in the log under its own event before the record's line for it is
// written, and closing a record saves whatever is still waiting.
func (keeper *Keeper) change(ctx context.Context, write func(into *contract.Record) error) error {
	return keeper.changeOrWait(ctx, write, keeper.oncePerRound)
}

// changeAndSave makes one change and saves the checkpoint behind it whatever
// the keeper's owner does about rounds, which is what closing a record does.
func (keeper *Keeper) changeAndSave(ctx context.Context, write func(into *contract.Record) error) error {
	return keeper.changeOrWait(ctx, write, false)
}

// changeOrWait is what both of those do: check the change against every rule on
// a copy, make the copy the record, and either save the checkpoint behind it now
// or mark it as waiting for the round. A save the log refuses leaves the record
// exactly as it was.
func (keeper *Keeper) changeOrWait(ctx context.Context, write func(into *contract.Record) error, wait bool) error {
	changing := cloneRecord(keeper.record)
	if err := write(&changing); err != nil {
		return err
	}
	if err := checkItReadsBack(changing); err != nil {
		return err
	}
	trimTheResultsToFit(&changing)
	if err := checkItStillFits(changing); err != nil {
		return err
	}
	held := keeper.record
	keeper.record = changing
	if wait {
		keeper.unsaved = true
		return nil
	}
	if err := keeper.save(ctx); err != nil {
		keeper.record = held
		return err
	}
	return nil
}

// applyUpdate runs every part of an update through its rules and writes it.
func applyUpdate(into *contract.Record, update Update) error {
	for _, write := range []func(*contract.Record, Update) error{
		applyName, applyWhy, applyDoneWhen, applyStopWhen, applyPlan, applyStepDone, applyTasks, applyDecision, applyFailure,
	} {
		if err := write(into, update); err != nil {
			return err
		}
	}
	return nil
}

// applyName holds the rule that a job's name is set once and then stands, the
// way the ask and the why above it do, so the thing a job is called does not
// drift out from under a task that is watching it.
func applyName(into *contract.Record, update Update) error {
	if update.Name == "" || update.Name == into.Goal.Name {
		return nil
	}
	if into.Goal.Name != "" {
		return fmt.Errorf("the name already reads %q, and it is written once: %w", into.Goal.Name, ErrNameIsSet)
	}
	into.Goal.Name = update.Name
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

// applyDoneWhen holds three rules: a task's done list holds at most
// MaxDoneLines lines, a done line marked done names the result that proves it
// or the user's reply that stands for one, and a line that names a result the
// record holds is done by that fact, mark or no mark. The last rule is there
// because a live job wrote all five of a task's done lines with their proof
// and every mark off, and the task waited on a person for work the record
// already held the proof of. The length of the ask decides nothing here: it
// used to refuse both the done list and the plan past 250 words and send the
// model to the job tool, and the model made the job and then kept working in
// the task with no list at all.
func applyDoneWhen(into *contract.Record, update Update) error {
	if update.DoneWhen == nil {
		return nil
	}
	if into.Header.Kind == contract.RecordTask && len(update.DoneWhen) > MaxDoneLines {
		return fmt.Errorf("this done list has %d lines and a task's done list holds at most %d, so %w",
			len(update.DoneWhen), MaxDoneLines, ErrDoneListTooLong)
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
	lines := slices.Clone(update.DoneWhen)
	for at := range lines {
		if lines[at].ResultID != "" {
			lines[at].Done = true
		}
	}
	into.Goal.DoneWhen = keepOrDrop(lines)
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

// applyPlan writes a task's plan under one rule, that a plan holds at most
// MaxPlanSteps steps, numbering the steps from one, and keeps the mark and the
// result of any step whose words did not change, so that editing a plan never
// throws away the proof of the work already done.
func applyPlan(into *contract.Record, update Update) error {
	if update.Plan == nil {
		return nil
	}
	if into.Header.Kind != contract.RecordTask {
		return fmt.Errorf("a plan belongs to a task and this is a %s, which has a task list: %w", into.Header.Kind, ErrWrongKind)
	}
	if len(update.Plan) > MaxPlanSteps {
		return fmt.Errorf("this plan has %d steps and a task's plan holds at most %d, so %w",
			len(update.Plan), MaxPlanSteps, ErrPlanTooLong)
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
	number := 1
	if last := len(into.Lessons.Decisions); last > 0 {
		number = numberAfter(into.Lessons.Decisions[last-1].ID)
	}
	into.Lessons.Decisions = theNewest(append(into.Lessons.Decisions, contract.Decision{
		ID:     contract.DecisionID(number),
		Text:   cutToALesson(update.Decision.Text),
		Reason: cutToALesson(update.Decision.Reason),
	}), MaxDecisionsKept)
	return nil
}

// applyStepDone marks one step of a task's plan done, through the same rules
// MarkPlanStep holds: the step must be one the plan has, and the result must
// be one the record wrote. It is the write behind the task tool's step_done,
// which a live build showed the record needed: its plan stood at "0 of 9 done"
// on the panel after the engine, the hazards and the UI were finished,
// because nothing ever marked a step.
func applyStepDone(into *contract.Record, update Update) error {
	if update.StepDone == nil {
		return nil
	}
	if into.Header.Kind != contract.RecordTask {
		return fmt.Errorf("a plan step belongs to a task and this is a %s, which has a task list: %w", into.Header.Kind, ErrWrongKind)
	}
	number := update.StepDone.Number
	if number >= 1 && number <= len(into.Work.Plan) && into.Work.Plan[number-1].Done {
		return fmt.Errorf("step %d is already done, by %s; %s: %w", number, into.Work.Plan[number-1].ResultID, theNextStepWaiting(into), ErrStepAlreadyDone)
	}
	return markStep(into, number, update.StepDone.ResultID)
}

// theNextStepWaiting names the first plan step not yet done, or says every
// step is done.
func theNextStepWaiting(into *contract.Record) string {
	for index, step := range into.Work.Plan {
		if !step.Done {
			return fmt.Sprintf("the next step waiting is step %d", index+1)
		}
	}
	return "every step is done"
}

// markStep is the check mark itself, shared by the model's step_done and the
// harness's MarkPlanStep.
func markStep(into *contract.Record, number int, resultID string) error {
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
	// The words compared are the words the record would keep, because a
	// whole paragraph shares few of its words with its own first line.
	kept := cutToALesson(update.Failure.Text)
	for _, held := range into.Lessons.Failures {
		if saysTheSame(kept, held.Text) {
			return fmt.Errorf("the failure %q says what %s already says: %w", kept, held.ID, ErrFailureAlreadyWritten)
		}
	}
	number := 1
	if last := len(into.Lessons.Failures); last > 0 {
		number = numberAfter(into.Lessons.Failures[last-1].ID)
	}
	into.Lessons.Failures = theNewest(append(into.Lessons.Failures, contract.Failure{
		ID:    contract.FailureID(number),
		Text:  cutToALesson(update.Failure.Text),
		Cause: cutToALesson(update.Failure.Cause),
	}), MaxFailuresKept)
	return nil
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
