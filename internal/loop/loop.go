// The shape of this loop is borrowed from OpenClaw's agent loop at
// ~/Code/openclaw/packages/agent-core/src/agent-loop.ts and its types at
// ~/Code/openclaw/packages/agent-core/src/types.ts, where the loop itself is
// pure and everything around it is a hook, and from ZeroClaw's turn engine at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/agent/turn/mod.rs, where a turn
// has a hard cap on its rounds and a forced final answer. The Go here is
// written fresh.

package loop

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Options is everything the loop needs. The first six are required; the rest
// switch off the part of the loop that uses them when they are missing.
type Options struct {
	// Model is the model to call.
	Model contract.Model
	// Tools is the set of tools this agent has.
	Tools contract.ToolRegistry
	// ToolsForTask, when it is set, is asked for the registry of each task, so
	// that the tools which need the record of the task running now, the task
	// tool and the reading of a past result, are built with it. When it is nil
	// the registry in Tools serves every task and the loop applies a task call
	// itself.
	ToolsForTask func(taskID string, records TaskRecord) (contract.ToolRegistry, error)
	// Permission is the rulebook every tool call goes through.
	Permission contract.Permission
	// Store is the event log.
	Store contract.Store
	// Clock is where the loop reads the time.
	Clock contract.Clock
	// Context builds the working context for one call.
	Context ContextBuilder
	// Jobs is the job store, and is nil when there are no jobs.
	Jobs contract.Job
	// Memory is where a review's last answer is kept, and is nil when there is
	// no memory.
	Memory contract.Memory
	// Skills is where a reviewed procedure is offered, and is nil when there
	// are no skills.
	Skills contract.Skill
	// Sandbox runs the command a done line names in backticks, and is nil when
	// the machine has no sandbox, in which case such a line is not checked.
	Sandbox contract.Sandbox
	// Caps are the limits from the configuration. A zero value means the
	// defaults from contract.
	Caps contract.Caps
	// ToolDeadline, when it is set, is asked for the context every tool call
	// runs under, so that the reliability guard can cut short a tool waiting on
	// something that has stopped answering. A tool still running when the
	// deadline passes is stopped and the model is told so. When it is nil a tool
	// call runs under the task's own context and nothing changes.
	ToolDeadline func(ctx context.Context) (context.Context, context.CancelFunc)
	// Vision says the model reads pictures, so a picture a tool hands back
	// rides with its result instead of being said to be unseen.
	Vision bool
	// Deltas is where a streamed reply goes as it arrives, and is nil when
	// nobody is watching.
	Deltas func(delta string)
	// ToolLine takes the one dim line the strip draws for the tool call in
	// flight, such as "▸ read note.txt", and again with what came back once it
	// has, and is nil when nobody is watching.
	ToolLine func(line string)
	// RecordLine takes one line whenever a task or a job is created, started,
	// finished, or failed, such as "task 3 started · build the game". It is
	// what a screen draws so that a person can see what the agent is working
	// on, and is nil when nobody is watching.
	RecordLine func(line string)
	// WorkingDirectory is the folder the agent works in, which the orientation
	// a fresh window opens with lists. Empty leaves the folder out of it.
	WorkingDirectory string
}

// Budget is how much one task may spend. A zero budget means the caps.
type Budget struct {
	// Rounds is how many model calls the task may make.
	Rounds int
	// Time is how long the task may take.
	Time time.Duration
}

// Task is one piece of work handed to the loop: a message from a user, or the
// next task of a job.
type Task struct {
	// Message is what the user sent. Its text is the ask.
	Message contract.Inbound
	// Channel is where the answer goes.
	Channel contract.Channel
	// FromJob is the job's next task, or nil when the task came from a user.
	FromJob *contract.TaskToRun
	// Unattended says nobody is there to answer a preview, so a call that would
	// have asked stops the task instead.
	Unattended bool
	// Budget is the task's own budget, which a skill may set. A zero budget
	// means the caps.
	Budget Budget
	// ResumeID is the number of a task that was waiting or stopped and is being
	// picked up again, and is empty for a new task.
	ResumeID string
	// Answer is what the person said when they picked up a job's task that had
	// made no record: the answer to the question it asked, or the words they
	// carried it on with. There is nothing to pick up, so the task is started
	// afresh under the job with its own words as the ask, and the answer is put
	// in front of the model right after them, so the task is told both what to
	// do and what the person said. Its text is empty for every other task.
	Answer contract.Inbound
	// Correction is the message that picked a job's stopped task up when it
	// said more than the word that carries on, such as "continue, but post at
	// noon". The task reads the whole message as the person's words, and it is
	// written into the record as a correction, the way a message that arrives
	// mid-task is, so that the steer outlives this sitting. Its text is empty
	// for every other task.
	Correction contract.Inbound
	// Question is the question a job's task asked before it made a record,
	// when Answer is the person's answer to it. It is put in front of the model
	// as its own words right before the answer, so that a task started afresh
	// reads question then answer rather than asking the question again. It is
	// empty for every other task.
	Question string
}

// Outcome is where one task ended.
type Outcome struct {
	// TaskID is the record's number, and is empty when the task answered a
	// question with no tools and so made no record.
	TaskID string
	// Status is done, waiting, stopped, or failed.
	Status contract.RecordStatus
	// Report is what the user was told.
	Report string
	// StopLine is the line of the stop list that fired, when one did.
	StopLine string
	// ByThePerson says the person stopped the task, with Escape, "/stop", or
	// the word stop, rather than the harness stopping it on a line of the stop
	// list or a spent budget. A person's stop puts a job's task down whoever
	// made the task; the harness's stop on a schedule's task is a failure.
	ByThePerson bool
}

// Loop runs one task at a time. Everything it needs is an interface, so the
// whole of it is driven by fakes in tests.
type Loop struct {
	options     Options
	oneAtATime  sync.Mutex
	guard       sync.Mutex
	queued      []contract.Inbound
	stopWanted  bool
	stopTheCall context.CancelFunc
	running     string
	// runningFromJob is the job's task the loop is running now, and nil while
	// the running task is a person's or nothing is running.
	runningFromJob *contract.TaskToRun
	// jobTasks is the job's task behind each number this loop has run for a
	// job, and jobTaskNumbers is those numbers oldest first, which is how the
	// memory is held to MaxJobTasksRemembered.
	jobTasks       map[string]contract.TaskToRun
	jobTaskNumbers []string
	highest        int
	counted        bool
	thinkModel     string
	thinkLevel     contract.Think
}

// UseThink sets how hard one model is asked to think, for the rest of the
// session, which is what the "/think" command does. The level rides on every
// call the loop makes from here on, so it reaches the provider without the
// model being built again.
//
// It is kept against the name of the model it was chosen for, and only one is
// kept, because a person chooses a level for the model they are talking to.
// After "/model" switches to another one, that model's own level from
// config.toml stands rather than the one chosen for the model before it.
func (theLoop *Loop) UseThink(modelAlias string, level contract.Think) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	theLoop.thinkModel = modelAlias
	theLoop.thinkLevel = level
}

// thinkFor is the level chosen this session for one model, and is empty both
// when nobody has chosen one and when the one chosen was for another model.
func (theLoop *Loop) thinkFor(modelAlias string) contract.Think {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if theLoop.thinkModel != modelAlias {
		return contract.ThinkDefault
	}
	return theLoop.thinkLevel
}

// New builds a loop and says which dependency is missing when one is.
func New(options Options) (*Loop, error) {
	for _, needed := range []struct {
		name  string
		there bool
	}{
		{"a model to call", options.Model != nil},
		{"a tool registry", options.Tools != nil},
		{"a permission function", options.Permission != nil},
		{"an event log to write to", options.Store != nil},
		{"a clock to read the time from", options.Clock != nil},
		{"a working-context builder", options.Context != nil},
	} {
		if !needed.there {
			return nil, fmt.Errorf("the turn loop needs %s, so pass one in its options", needed.name)
		}
	}
	// A caller that passed no caps at all gets the shipped ones. A zero in any
	// one budget is not that: it is the user's own way of saying that budget
	// is off, and it is left alone.
	if options.Caps == (contract.Caps{}) {
		options.Caps = contract.DefaultConfig().Caps
	}
	return &Loop{options: options}, nil
}

// Run takes one task to an end state and returns where it ended. A second call
// waits until the first is finished, because the agent works on one task at a
// time. A person's message that picks a job's task up, by the word that
// carries on or by the task's number, is run as that job's task, and its
// report goes into the job; the job's next task is the driver's to take on
// its next ask, so that each task runs under a turn of its own.
func (theLoop *Loop) Run(ctx context.Context, task Task) (Outcome, error) {
	theLoop.oneAtATime.Lock()
	defer theLoop.oneAtATime.Unlock()
	theLoop.clearStopAsked()
	task, err := theLoop.pickUpTheJobsTask(ctx, task)
	if err != nil {
		return Outcome{}, err
	}
	return theLoop.runTaskAndItsJob(ctx, task)
}

// RunNextJobTask asks the job store for the task that is due now and runs it,
// one task per call. It returns false when nothing is due. The job's next task
// is taken on the caller's next ask, which the store's wake makes cheap, so
// that the one turn deadline the caller sets covers one task rather than a
// whole chain of them.
func (theLoop *Loop) RunNextJobTask(ctx context.Context, where contract.Channel) (bool, error) {
	if theLoop.options.Jobs == nil {
		return false, nil
	}
	due, there, err := theLoop.options.Jobs.NextTask(ctx, theLoop.options.Clock.Now())
	if err != nil {
		return false, fmt.Errorf("cannot ask the jobs which task is due now: %w", err)
	}
	if !there {
		return false, nil
	}
	return true, theLoop.RunJobTask(ctx, due, where)
}

// RunJobTask runs one task of a job that its caller has already taken from the
// job store, and writes its report into the job the way RunNextJobTask does.
// The job driver in cmd/nerdgenie asks the store for the due task itself, so
// that it can hand the nightly self-check to the checker rather than to the
// model, and a task taken once cannot be taken again: a driver that then asked
// this loop for the next task would run the second task of a job first and
// leave the first sitting taken until its budget ran out.
func (theLoop *Loop) RunJobTask(ctx context.Context, due contract.TaskToRun, where contract.Channel) error {
	_, err := theLoop.Run(ctx, taskFromJob(due, where))
	return err
}

// taskFromJob turns a job's next task into a task the loop can run.
func taskFromJob(due contract.TaskToRun, where contract.Channel) Task {
	return Task{
		Message:    contract.Inbound{ID: due.JobID + "." + due.TaskID, Text: due.Text, Channel: nameOf(where)},
		Channel:    where,
		FromJob:    &due,
		Unattended: due.Unattended,
	}
}

// nameOf is a channel's name, and is empty when there is no channel.
func nameOf(where contract.Channel) string {
	if where == nil {
		return ""
	}
	return where.Name()
}

// runTaskAndItsJob runs one task and, when it belongs to a job, writes its
// report into the job. It used to carry on with the job's next tasks, up to a
// hundred of them, inside the one call, so the turn deadline the driver set
// covered the whole chain; one task per call gives each its own turn, and the
// driver takes the next on its next ask.
func (theLoop *Loop) runTaskAndItsJob(ctx context.Context, task Task) (Outcome, error) {
	outcome, number, err := theLoop.runOne(ctx, task)
	if err != nil || task.FromJob == nil {
		return outcome, err
	}
	return outcome, theLoop.finishJobTask(ctx, task, number, outcome)
}

// runOne takes one task through its rounds, and says which number it ran
// under, which a task of a job is remembered by so that it can be picked up
// again as the job's task.
func (theLoop *Loop) runOne(ctx context.Context, task Task) (Outcome, string, error) {
	running, err := theLoop.newRun(ctx, task)
	if err != nil {
		return Outcome{}, "", err
	}
	theLoop.rememberTheJobTask(running.number, task.FromJob)
	theLoop.nowRunning(running.taskID())
	theLoop.nowRunningFromJob(task.FromJob)
	defer theLoop.nowRunning("")
	defer theLoop.nowRunningFromJob(nil)

	theLoop.noteRecordLine(RecordLineOf(running.number, task.FromJob, "started", task.Message.Text))
	outcome, err := running.play(ctx)
	// A task saves one checkpoint per model call, and its last round has no call
	// after it to save what that round left behind, so the ending saves it here,
	// under the ending's own context, because the turn's may be cancelled.
	wrappingUp, done := running.timeToWrapUp(ctx)
	defer done()
	if saving := running.saveTheRound(wrappingUp); saving != nil && err == nil {
		return outcome, running.number, saving
	}
	if err == nil {
		theLoop.noteRecordLine(RecordLineOf(running.number, task.FromJob, string(outcome.Status), task.Message.Text))
	}
	return outcome, running.number, err
}

// Deliver hands the loop a message that arrived while a task was running. The
// task picks it up as soon as the tool call in flight has finished.
func (theLoop *Loop) Deliver(message contract.Inbound) error {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if len(theLoop.queued) >= theLoop.options.Caps.QueuedMessages {
		return fmt.Errorf("%d messages are already waiting for the running task, which is as many as the queue holds",
			len(theLoop.queued))
	}
	theLoop.queued = append(theLoop.queued, message)
	return nil
}

// takeDelivered empties the queue of messages that arrived mid-task.
func (theLoop *Loop) takeDelivered() []contract.Inbound {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	waiting := theLoop.queued
	theLoop.queued = nil
	return waiting
}

// Stop asks the running task to stop as soon as the tool call in flight has
// finished.
func (theLoop *Loop) Stop() {
	theLoop.guard.Lock()
	stopTheCall := theLoop.stopTheCall
	theLoop.stopWanted = true
	theLoop.guard.Unlock()

	// The call in flight is cancelled outside the lock, because a model that is
	// waiting on the network can take a moment to notice, and nothing else
	// should have to wait behind it. A task waiting on a model that has gone
	// quiet is the whole reason the person pressed Escape.
	if stopTheCall != nil {
		stopTheCall()
	}
}

// holdTheCall remembers how to cancel the model call that is about to be made,
// so that a stop reaches it rather than waiting for it to answer.
func (theLoop *Loop) holdTheCall(stopTheCall context.CancelFunc) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	theLoop.stopTheCall = stopTheCall
}

// releaseTheCall forgets the call that has just finished.
func (theLoop *Loop) releaseTheCall() {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	theLoop.stopTheCall = nil
}

// noteRecordLine sends one line about a task or a job to whoever is watching.
func (theLoop *Loop) noteRecordLine(line string) {
	if theLoop.options.RecordLine == nil {
		return
	}
	theLoop.options.RecordLine(line)
}

// stopAsked says whether somebody asked the running task to stop.
func (theLoop *Loop) stopAsked() bool {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	return theLoop.stopWanted
}

// clearStopAsked forgets a stop that was asked for before this task began.
func (theLoop *Loop) clearStopAsked() {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	theLoop.stopWanted = false
}

// Running is the number of the task running now, or an empty string when
// nothing is running.
func (theLoop *Loop) Running() string {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	return theLoop.running
}

// nowRunning records which task is running, for the status commands.
func (theLoop *Loop) nowRunning(taskID string) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	theLoop.running = taskID
}

// RunningJobTask is the job's task running now, when the running task is a
// job's rather than a person's, and false otherwise. It is what the program
// builds the status a screen draws the job from, and it is known from the
// moment the task starts rather than from its first tool call, because a task
// of a job is a job's task whether or not it ever opens a record.
func (theLoop *Loop) RunningJobTask() (contract.TaskToRun, bool) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if theLoop.runningFromJob == nil {
		return contract.TaskToRun{}, false
	}
	return *theLoop.runningFromJob, true
}

// nowRunningFromJob records which job's task is running, or that none is.
func (theLoop *Loop) nowRunningFromJob(fromJob *contract.TaskToRun) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	theLoop.runningFromJob = fromJob
}

// logEvent writes one event about the running task, and says plainly when the
// log will not take it.
func (theLoop *Loop) logEvent(ctx context.Context, taskID string, kind contract.EventKind, body any) error {
	written, err := asJSON(body)
	if err != nil {
		return err
	}
	if _, err := theLoop.options.Store.Append(ctx, contract.Event{
		TaskID:   taskID,
		Occurred: theLoop.options.Clock.Now(),
		Kind:     kind,
		Body:     written,
	}); err != nil {
		return fmt.Errorf("cannot write the %s event of task %q to the log: %w", kind, taskID, err)
	}
	return nil
}
