package contract

import (
	"context"
	"strconv"
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

// ProjectFolderLine opens the situation line of a job record that names the
// project's folder, as in "project folder: ~/Desktop/Tic Tac Toe".
const ProjectFolderLine = "project folder: "

// NewJob is what the model gives the harness to create a job.
type NewJob struct {
	// Ask is the user's message, word for word, the same as a task record's ask.
	Ask string
	// Name is a short name for the job, a few words, such as "Tater Tots Tetris",
	// which is what the job list and the side panel show in place of the ask.
	Name string
	// Why is the one line on why the user wants it.
	Why string
	// Schedule is the job's schedule, or nil when it has none.
	Schedule *Schedule
	// TaskTemplate is the text a scheduled job turns into one task per tick, and
	// is empty for a job with no schedule.
	TaskTemplate string
	// DoneWhen is the job's done list as the person wrote it, one line each,
	// none of them proved yet, when the ask was a work order; empty otherwise,
	// and the model writes the list itself. A line may end with a bracketed
	// check the harness runs itself.
	DoneWhen []string
	// Rules are the job's rules in the person's words, one line each, kept in
	// the record's rules where every task of the job reads them; empty when
	// the ask was not a work order.
	Rules []string
	// Folder is the project's folder as the work order's Where wrote it, such
	// as `~/Desktop/Tic Tac Toe`, kept in the job record's situation as the
	// line "project folder: ...", where every task of the job reads it and
	// works there; empty when the ask named none, and the home's work folder
	// is the folder then.
	Folder string
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

// TaskToRun is the next task a job wants started. The loop asks for one
// whenever nothing else is running, and runs it as an ordinary task whose
// report goes back into the job when it finishes.
type TaskToRun struct {
	// JobID is the job the task belongs to.
	JobID string
	// TaskID is the task's label inside the job, such as "t31".
	TaskID string
	// Text is the one line saying what the task does.
	Text string
	// Unattended says a schedule made the task and nobody has picked it up, so
	// nobody is there to answer a preview, and anything on the ask-me-first
	// list stops the task instead. A task the person picks up, with the word
	// that carries on or with an answer, is attended from then on whoever made
	// it.
	Unattended bool
}

// PutDownMark is the mark a job carries while it is put down on one of its
// tasks: the person stopped the task, or it asked them a question, and the job
// holds on it until they pick it up. The store keeps it beside the job's state
// rather than the loop keeping it in memory, so that the word that carries on,
// or the answer, picks that same task up under the same job after a restart.
type PutDownMark struct {
	// Task is the job's task the job holds on, as it was handed out.
	Task TaskToRun
	// Run is the number the loop ran the task under, which is what is picked
	// up again.
	Run string
	// HasRecord says that run made a task record, so it is resumed from its
	// last checkpoint. A run that had made none is started afresh.
	HasRecord bool
	// Waiting says the task asked the person a question, so their next message
	// answers it. Otherwise the person stopped it, and only the word that
	// carries on picks it up.
	Waiting bool
	// Question is the question the task asked, when Waiting, so that a task
	// started afresh on the answer is shown its own question before the answer
	// rather than asking it again. It is empty for a task the person stopped.
	Question string
}

// RunNumberOf reads the run a put-down mark carries as the number it is, and
// is zero for one that is not a number, so that a mark with no run at all is
// the oldest of any. The real job store and the fake both pick the newest
// put-down task by it, so it lives here rather than in each of them.
func RunNumberOf(run string) int {
	number, err := strconv.Atoi(run)
	if err != nil {
		return 0
	}
	return number
}

// RecordStatusOfJob maps where a job stands onto the status its record prints:
// a paused job is waiting, a job that is off is stopped, a done job is done,
// and anything else is running. The real job store and the fake both print
// it, so it lives here rather than in each of them.
func RecordStatusOfJob(state JobState) RecordStatus {
	switch state {
	case JobPaused:
		return StatusWaiting
	case JobOff:
		return StatusStopped
	case JobDone:
		return StatusDone
	default:
		return StatusRunning
	}
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
	// RunNow starts the job's next task without waiting for its date: the date
	// is taken off that task, a schedule ticks at once, and a job put down on a
	// task runs that task first and forgets its mark.
	RunNow(ctx context.Context, jobID string) error
	// Resume sets a paused job running again and nothing else: every task keeps
	// its date, a schedule keeps its next tick, the claims the paused run held
	// are let go, and the mark of a job put down on a task is forgotten. It is
	// what picks a put-down task up, because the person's word means "carry on
	// where you were" and not "run now". A job that is not there is refused with
	// an error naming it.
	Resume(ctx context.Context, jobID string) error
	// Pause stops the job after the running task finishes.
	Pause(ctx context.Context, jobID string) error
	// SwitchOff stops the job for good and tells the user.
	SwitchOff(ctx context.Context, jobID string) error
	// PutDown pauses a job on one of its tasks and marks it: the person stopped
	// the task, or it asked them a question. Nothing of the job runs until it
	// is set running again, by RunNow or by the person picking the task up,
	// and the mark is what that pick-up reads, even after a restart. A job set
	// running again forgets its mark. A job that is not there, or a task not on
	// its list, is refused with an error naming it.
	PutDown(ctx context.Context, mark PutDownMark) error
	// PutDownTask returns the mark of the paused job put down most recently,
	// which is the one whose run is newest, and false when no job is put down.
	PutDownTask(ctx context.Context) (PutDownMark, bool, error)
	// PickUpOnce says whether the job may pick one of its tasks up itself,
	// on a fresh window, after the harness's own guard stopped it: yes the
	// first time and no from then on, so that a task which stalls the same
	// way twice is put down for a person. The answer is written into the job,
	// so a restart remembers it. A job that is not there, a task not on its
	// list, and a finished task are refused with an error naming them.
	PickUpOnce(ctx context.Context, jobID string, taskID string) (bool, error)
	// NextTask returns the next task that may start now: the first unfinished
	// task of the oldest running job whose due time has passed, or, for a job
	// with a schedule whose tick has come, one new task made from its template.
	// It returns false when nothing is due.
	NextTask(ctx context.Context, now time.Time) (TaskToRun, bool, error)
	// FinishTask writes a finished task's report into its job and returns the
	// report's id, such as "j4.2". A task that failed stays on the list to be
	// tried again and counts toward the failures in a row: three pause the job,
	// and ten switch a scheduled job off.
	FinishTask(ctx context.Context, jobID string, taskID string, report string, failed bool) (string, error)
	// ProveDoneLine marks one line of the job's done list, counting from one,
	// done and pointing at the report that proves it, or unmarks it when the
	// result is empty, and closes the job when every line is proved and every
	// task is done. A job or a line that is not there is an error naming it.
	ProveDoneLine(ctx context.Context, jobID string, number int, resultID string) error
	// SetProjectFolder writes the job's project folder into its record as the
	// ProjectFolderLine, for a job whose ask named none: the harness learns
	// the folder from the files the first task wrote, and every later task of
	// the job works there.
	SetProjectFolder(ctx context.Context, jobID string, folder string) error
	// Load returns the job's record, which is what rides above the task record
	// while one of the job's tasks runs and what "/jobs 4" prints.
	Load(ctx context.Context, jobID string) (Record, error)
}
