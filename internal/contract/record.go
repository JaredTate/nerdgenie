package contract

// RecordKind says whether a record is a task or a job. One set of types serves
// both, because a job has the same four parts as a task: a goal, rules, work,
// and lessons. Only the work section differs, and the kind says how to read it.
type RecordKind string

const (
	// RecordTask is one sitting of work, a few minutes long.
	RecordTask RecordKind = "task"
	// RecordJob is a piece of work too big for one sitting, whose plan is a list
	// of tasks and whose results are their reports.
	RecordJob RecordKind = "job"
)

// RecordStatus is where a record stands.
type RecordStatus string

const (
	// StatusRunning means work is happening now.
	StatusRunning RecordStatus = "running"
	// StatusWaiting means the record is waiting for the user to answer, and
	// nothing is held in any context window until they do.
	StatusWaiting RecordStatus = "waiting"
	// StatusStopped means something on the stop list fired.
	StatusStopped RecordStatus = "stopped"
	// StatusFailed means the work could not be finished.
	StatusFailed RecordStatus = "failed"
	// StatusDone means every done line pointed at a result and the done-check
	// passed.
	StatusDone RecordStatus = "done"
)

// CostLine is the token count of one turn, which the harness writes into the
// header so the user can always see what a task cost and where it went.
type CostLine struct {
	// InputTokens is everything the provider read this turn.
	InputTokens int
	// CachedInputTokens is how much of that it reused from its cache.
	CachedInputTokens int
	// OutputTokens is what the model wrote.
	OutputTokens int
}

// Header is the first line or two of a record. A task carries the budget line
// and the cost line; a job carries the progress line and the next due task.
type Header struct {
	// Kind says whether to read this as a task or a job.
	Kind RecordKind
	// ID is the record's number as a string, such as "17" or "4".
	ID string
	// Status is where it stands.
	Status RecordStatus
	// Origin is the channel the ask came in on, such as "Signal".
	Origin string
	// RoundsLeft is the task's remaining tool rounds, and is unused on a job.
	RoundsLeft int
	// MinutesLeft is the task's remaining minutes, and is unused on a job.
	MinutesLeft int
	// Cost is the task's cost line for this turn, and is unused on a job.
	Cost CostLine
	// TasksDone is the first half of a job's progress line, and is unused on a
	// task.
	TasksDone int
	// TasksTotal is the second half of that line.
	TasksTotal int
	// NextDue is a job's next due task in plain words, such as
	// "task 31 today at 14:00", and is unused on a task.
	NextDue string
}

// DoneLine is one thing that must be true before the record can close. The
// done-check refuses to close a record while any line has nothing behind it.
type DoneLine struct {
	// Text is the line itself, in the model's words.
	Text string
	// Done says the model believes this line is satisfied.
	Done bool
	// ResultID is the result that proves it, such as "r6" on a task or "j4.2" on
	// a job.
	ResultID string
	// UserReply is the user's own words standing in for a result, when the proof
	// is something the user said rather than something a tool returned.
	UserReply string
}

// Goal is what the user asked for and what done looks like. The ask is never
// edited, by anyone, for any reason.
type Goal struct {
	// Ask is the user's message, word for word.
	Ask string
	// Why is the one line on why the user wants it, which is what lets the model
	// act sensibly when the plan breaks.
	Why string
	// DoneWhen is the done list.
	DoneWhen []DoneLine
}

// Correction is something the user said while the work was running, in their own
// words. Corrections are only ever added to, and never edited.
type Correction struct {
	// ID is the correction's label, such as "C1".
	ID string
	// Text is the user's words, unchanged.
	Text string
}

// Rules is what the user has corrected and what must stop the work.
type Rules struct {
	// Corrections is the list, in the order they arrived.
	Corrections []Correction
	// StopWhen is the short list of things that stop the work at once. The
	// harness checks it before every tool call and adds two lines of its own:
	// the budget running out, and a login page or a captcha appearing.
	StopWhen []string
}

// PlanStep is one step of a task's plan.
type PlanStep struct {
	// Number is the step's position, starting at one.
	Number int
	// Text says what the step does.
	Text string
	// Done says the step finished.
	Done bool
	// ResultID is the result that proves it, such as "r3".
	ResultID string
}

// JobTask is one task on a job's task list, which is what a job has in place of
// a task's plan.
type JobTask struct {
	// TaskID is the task's label, such as "t31".
	TaskID string
	// Text says what the task does, with one clear done line behind it.
	Text string
	// Done says the task finished.
	Done bool
	// ReportID is the report it produced, such as "j4.2".
	ReportID string
	// DueAt is the date the task must wait for, in plain words such as
	// "today at 14:00", and is empty when it need not wait.
	DueAt string
}

// ResultLine is one line standing for something a tool returned, on a task, or
// for a finished task's report, on a job. The full text is in the event log, and
// "read r7" or "read j4.2" brings it back.
type ResultLine struct {
	// ID is the line's label: "r7" on a task, "j4.2" on a job.
	ID string
	// Summary is the one line the record holds, such as "draft post, 236
	// characters".
	Summary string
}

// Work is where things stand. A task fills Plan and Results; a job fills Tasks
// and Results.
type Work struct {
	// Situation is the few facts the harness can check on its own: the page the
	// browser is on, the files changed, the last command and whether it worked.
	Situation []string
	// Plan is a task's list of steps, and is empty on a job.
	Plan []PlanStep
	// Tasks is a job's list of tasks, and is empty on a task.
	Tasks []JobTask
	// Results is one line per tool result on a task, or one line per finished
	// task's report on a job.
	Results []ResultLine
}

// Decision is a choice the model made, with its reason, so that it does not
// argue with itself later. The task tool refuses a decision with no reason.
type Decision struct {
	// ID is the decision's label, such as "D1".
	ID string
	// Text is the choice.
	Text string
	// Reason is why.
	Reason string
}

// Failure is something that went wrong, with its cause, so that it is not
// repeated. The task tool refuses a failure with no cause.
type Failure struct {
	// ID is the failure's label, such as "F1".
	ID string
	// Text is what went wrong.
	Text string
	// Cause is why it went wrong.
	Cause string
}

// Lessons is what has been decided and what has gone wrong.
type Lessons struct {
	// Decisions is the list of choices with their reasons.
	Decisions []Decision
	// Failures is the list of failures with their causes.
	Failures []Failure
}

// Record is the whole of a task's or a job's state: the four parts of an
// operations order, one to three thousand tokens, never summarized and never
// squashed. It is what the model reads on every call in place of a transcript.
type Record struct {
	// Header is the first line, with the budget or the progress on it.
	Header Header
	// Goal is what was asked and what done looks like.
	Goal Goal
	// Rules is what was corrected and what must stop the work.
	Rules Rules
	// Work is where things stand.
	Work Work
	// Lessons is what was decided and what went wrong.
	Lessons Lessons
}
