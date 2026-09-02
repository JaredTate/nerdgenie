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
	// only ever appended and never edited or removed.
	ErrCorrectionIsFixed = errors.New("a correction is the user's own words and is only ever added, so pass what the user said")
	// ErrWrongKind is the rule that a task and a job each have their own half of
	// the work section, and neither takes the other's.
	ErrWrongKind = errors.New("this belongs to the other kind of record, so use the call that matches this one")
	// ErrWhyIsSet is the rule that the one line on why the user wants this is
	// written once and then stands, like the ask above it.
	ErrWhyIsSet = errors.New("the why is written once and then stands, so leave it as the user's reason")
	// ErrFinishedTaskRemoved is the rule that a job never takes a finished task
	// off its list, however much the rest of the list is rearranged.
	ErrFinishedTaskRemoved = errors.New("a finished task stays on a job's list, so leave it where it is")
	// ErrBeforeTheFirstCheckpoint says the wind-back asked for a moment before
	// the record was created, and there is nothing there.
	ErrBeforeTheFirstCheckpoint = errors.New("that is before the record's first checkpoint, so wind back fewer steps")
	// ErrNoSuchResult says the label names no result this record ever wrote.
	ErrNoSuchResult = errors.New("no result with that label was written by this record, so check it against the result list")
)
