package record

import "errors"

// Every rule of a record has a named error here, so that a caller can tell which
// rule was broken with errors.Is and hand the model back a line it can act on.
// The message on each one says what to do; the error that wraps it says where.
var (
	// ErrDoneLineNeedsProof is the rule that a done line marked done must name
	// the result that proves it or the user's reply that stands for one.
	ErrDoneLineNeedsProof = errors.New("a done line marked done must name the result that proves it, or the user's reply")
	// ErrPlanStepNeedsResult is the rule that a plan step marked done must name
	// its result.
	ErrPlanStepNeedsResult = errors.New("a plan step marked done must name the result it produced")
	// ErrJobTaskNeedsReport is the rule that a task on a job marked done must
	// name the report it wrote.
	ErrJobTaskNeedsReport = errors.New("a task marked done must name the report it wrote, such as j4.2")
	// ErrDoneListTooLong is the rule that a task's done list holds at most
	// MaxDoneLines lines, because a task is one sitting and an ask that needs
	// more than that is a job. The harness decides this rather than the model,
	// because a small model never says an ask is too big for it. The message
	// says what to do instead, and it is the one line the model reads.
	ErrDoneListTooLong = errors.New("this ask is a job, not one task: create it with the job tool, one task per done line, each task with one clear done line, and then work the first task")
	// ErrPlanTooLong is the rule that a task's plan holds at most MaxPlanSteps
	// steps, because a task is one sitting and a plan that needs more than that
	// is a job's task list in disguise. It closes the door the done-list rule
	// left open: a model that kept its done list short and hid the work in a
	// long plan. The message gives the model the same instruction as the
	// done-list refusal, with one task per step, and it is the one line the
	// model reads.
	ErrPlanTooLong = errors.New("this ask is a job, not one task: create it with the job tool, one task per step, each task with one clear done line, and then work the first task")
	// ErrDecisionNeedsReason is the rule that every decision carries its reason,
	// so that the model does not argue with itself later.
	ErrDecisionNeedsReason = errors.New("a decision must carry the reason it was made, so add a reason to it")
	// ErrFailureNeedsCause is the rule that every failure carries its cause, so
	// that it is not repeated.
	ErrFailureNeedsCause = errors.New("a failure must carry the cause it had, so add a cause to it")
	// ErrTasksOutOfOrder is the rule that a job lists its tasks in order, so that
	// a later task can lean on the reports of the ones before it.
	ErrTasksOutOfOrder = errors.New("a job lists its tasks in order, so give this one a number above the one before it")
	// ErrIdentifiersOutOfOrder is the rule that the labels of a list only count
	// upwards, so that nothing a reader has already seen is renumbered.
	ErrIdentifiersOutOfOrder = errors.New("the labels of a list only count upwards, so give this one a number above the last")
	// ErrCorrectionIsFixed is the rule that a correction is the user's own words,
	// only ever appended and never edited or removed, so there is nothing to add
	// when the user said nothing.
	ErrCorrectionIsFixed = errors.New("a correction is the user's own words, added and never edited, so pass what the user said")
	// ErrWrongKind is the rule that a task and a job each have their own half of
	// the work section, and neither takes the other's.
	ErrWrongKind = errors.New("this belongs to the other kind of record, so use the call that matches this one")
	// ErrWhyIsSet is the rule that the one line on why the user wants this is
	// written once and then stands, like the ask above it.
	ErrWhyIsSet = errors.New("the why is written once and then stands, so leave it as the user's reason")
	// ErrNameIsSet is the rule that a job's short name is written once and then
	// stands, like the ask and the why.
	ErrNameIsSet = errors.New("the name is written once and then stands, so leave it as it was first set")
	// ErrFinishedTaskRemoved is the rule that a job never takes a finished task
	// off its list, however much the rest of the list is rearranged.
	ErrFinishedTaskRemoved = errors.New("a finished task stays on a job's list, so leave it where it is")
	// ErrBeforeTheFirstCheckpoint says the wind-back asked for a moment before
	// the record was created, and there is nothing there.
	ErrBeforeTheFirstCheckpoint = errors.New("that is before the record's first checkpoint, so wind back fewer steps")
	// ErrAskIsElsewhere says this checkpoint keeps the user's ask in another
	// checkpoint, and that one was not among the ones read, so the record cannot
	// be handed back whole.
	ErrAskIsElsewhere = errors.New("the checkpoint that carries the user's ask was not read, so read the whole log of this record")
	// ErrNoSuchResult says the label names no result this record ever wrote.
	ErrNoSuchResult = errors.New("no result with that label was written by this record, so check it against the result list")
	// ErrFailureAlreadyWritten says the lesson is on the record already: a
	// failure written twice teaches nothing twice and costs the context every
	// round after, and the fifth game build's play-test task wrote one ten
	// times over while it stalled.
	ErrFailureAlreadyWritten = errors.New("the record already holds this failure, so act on it or move on rather than writing it again")
	// ErrRecordTooLarge is the rule that a record stays small enough to sit in
	// front of any model, which is what lets a task be put down and picked up on
	// a smaller one days later. A record that grows past it is not a slow task
	// but a task that cannot run at all, because the working-context builder
	// refuses the whole prompt.
	ErrRecordTooLarge = errors.New("this change would take the record past the size it is promised to stay under, so shorten it")
)
