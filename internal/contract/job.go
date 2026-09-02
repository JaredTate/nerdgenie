package contract

import (
	"context"
	"time"
)

// ScheduleKind says which of the three ways a job's schedule is written.
type ScheduleKind string

const (
	// ScheduleAt runs once at one moment.
	ScheduleAt ScheduleKind = "at"
	// ScheduleEvery runs on a fixed interval.
	ScheduleEvery ScheduleKind = "every"
	// ScheduleCron runs on a cron expression, such as every weekday at seven.
	ScheduleCron ScheduleKind = "cron"
)

// Schedule is when a job creates its next task. A job without one runs its tasks
// back to back; a job with one creates a task per tick from its template.
type Schedule struct {
	// Kind says which of the three fields below is filled in.
	Kind ScheduleKind
	// At is the one moment, when Kind is ScheduleAt.
	At time.Time
	// Every is the interval, when Kind is ScheduleEvery.
	Every time.Duration
	// Cron is the expression, when Kind is ScheduleCron.
	Cron string
	// Timezone is the location name the schedule is read in, such as
	// "America/New_York". An empty timezone means the machine's own.
	Timezone string
}

// JobState is where a job stands.
type JobState string

const (
	// JobRunning means the job is working through its tasks.
	JobRunning JobState = "running"
	// JobPaused means the user paused it, or three failures in a row did.
	JobPaused JobState = "paused"
	// JobOff means it was switched off, which is what ten failures in a row do
	// to a scheduled job.
	JobOff JobState = "off"
	// JobDone means every task finished and the done list passed.
	JobDone JobState = "done"
)

// JobSummary is one line about a job: what the "/jobs" listing shows, and what
// rides above the task record while one of the job's tasks is running.
type JobSummary struct {
	// ID is the job's number as a string, such as "4".
	ID string
	// Title is the one line saying what the job does.
	Title string
	// State is running, paused, off, or done.
	State JobState
	// TasksDone is the first half of the progress line, "3 of 12 tasks done".
	TasksDone int
	// TasksTotal is the second half of that line.
	TasksTotal int
	// NextTaskID is the task that runs next, such as "t31", or empty.
	NextTaskID string
	// NextDue is when that task may start, and is the zero time when it need not
	// wait for a date.
	NextDue time.Time
	// LastRun is when a task of this job last finished.
	LastRun time.Time
	// NextRun is when the schedule fires next, and is the zero time when the job
	// has no schedule.
	NextRun time.Time
	// FailuresInARow counts consecutive failed tasks. Three pauses the job, and
	// ten switches a scheduled job off.
	FailuresInARow int
}

// NewJob is what the model gives the harness to create a job.
type NewJob struct {
	// Ask is the user's message, word for word, the same as a task record's ask.
	Ask string
	// Why is the one line on why the user wants it.
	Why string
	// Schedule is the job's schedule, or nil when it has none.
	Schedule *Schedule
	// TaskTemplate is the text a scheduled job turns into one task per tick, and
	// is empty for a job with no schedule.
	TaskTemplate string
}

// NewTask is one task added to a job's task list.
type NewTask struct {
	// JobID says which job the task belongs to.
	JobID string
	// Text is the one line saying what the task does, with one clear done line
	// behind it.
	Text string
	// DueAt is when the task may start, and is the zero time when it may start
	// as soon as the tasks before it are done.
	DueAt time.Time
}

// Job is a piece of work too big for one sitting: the same four parts as a task
// record, but its plan is a list of tasks and its results are their reports.
// A scheduled job is simply a job with a schedule, so there is one idea here and
// "/cron" only shows the ones that have one.
type Job interface {
	// Create starts a job, with or without a schedule, and returns its id.
	Create(ctx context.Context, job NewJob) (string, error)
	// AddTask puts one more task on a job's list and returns its id, such as
	// "t31". A finished task is never removed.
	AddTask(ctx context.Context, task NewTask) (string, error)
	// List returns every job, newest first.
	List(ctx context.Context) ([]JobSummary, error)
	// RunNow starts the job's next task without waiting for its date.
	RunNow(ctx context.Context, jobID string) error
	// Pause stops the job after the running task finishes.
	Pause(ctx context.Context, jobID string) error
	// SwitchOff stops the job for good and tells the user.
	SwitchOff(ctx context.Context, jobID string) error
}
