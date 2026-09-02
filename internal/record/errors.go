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
)
