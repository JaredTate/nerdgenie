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
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxTasksInARow is how many of a job's tasks the loop will run back to back
// before it hands control back to its caller. A job with more tasks than this
// carries on the next time the caller asks for work.
const MaxTasksInARow = 100

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
	// Deltas is where a streamed reply goes as it arrives, and is nil when
	// nobody is watching.
	Deltas func(delta string)
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
}

// Loop runs one task at a time. Everything it needs is an interface, so the
// whole of it is driven by fakes in tests.
type Loop struct {
	options    Options
	oneAtATime sync.Mutex
	guard      sync.Mutex
	queued     []contract.Inbound
	stopWanted bool
	running    string
	highest    int
	counted    bool
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
	if options.Caps.RoundsPerTask <= 0 {
		options.Caps = contract.DefaultConfig().Caps
	}
	return &Loop{options: options}, nil
}

// Run takes one task to an end state and returns where it ended. A second call
// waits until the first is finished, because the agent works on one task at a
// time.
func (theLoop *Loop) Run(ctx context.Context, task Task) (Outcome, error) {
	theLoop.oneAtATime.Lock()
	defer theLoop.oneAtATime.Unlock()
	theLoop.clearStopAsked()
	return theLoop.runTaskAndItsJob(ctx, task)
}

// RunNextJobTask asks the job store for the task that is due now and runs it
// unattended. It returns false when nothing is due.
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
	if _, err := theLoop.Run(ctx, taskFromJob(due, where)); err != nil {
		return true, err
	}
	return true, nil
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
// report into the job and carries on with the job's next task.
func (theLoop *Loop) runTaskAndItsJob(ctx context.Context, task Task) (Outcome, error) {
	first := Outcome{}
	for turn := range MaxTasksInARow {
		outcome, err := theLoop.runOne(ctx, task)
		if turn == 0 {
			first = outcome
		}
		if err != nil {
			return first, err
		}
		if task.FromJob == nil {
			return first, nil
		}
		next, more, err := theLoop.finishJobTask(ctx, task, outcome)
		if err != nil || !more {
			return first, err
		}
		task = next
	}
	return first, nil
}

// runOne takes one task through its rounds.
func (theLoop *Loop) runOne(ctx context.Context, task Task) (Outcome, error) {
	running, err := theLoop.newRun(ctx, task)
	if err != nil {
		return Outcome{}, err
	}
	theLoop.nowRunning(running.taskID())
	defer theLoop.nowRunning("")
	return running.play(ctx)
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
	defer theLoop.guard.Unlock()
	theLoop.stopWanted = true
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

// nextTaskNumber is the number the next record takes: one above the highest the
// log has ever held. The log is read once and the count is kept afterwards.
func (theLoop *Loop) nextTaskNumber(ctx context.Context) (string, error) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if !theLoop.counted {
		saved, err := theLoop.options.Store.ByKind(ctx, contract.EventCheckpoint)
		if err != nil {
			return "", fmt.Errorf("cannot read the log to number the next task: %w", err)
		}
		for _, event := range saved {
			if number, isTask := taskNumberOf(event.TaskID); isTask && number > theLoop.highest {
				theLoop.highest = number
			}
		}
		theLoop.counted = true
	}
	theLoop.highest++
	return strconv.Itoa(theLoop.highest), nil
}

// taskNumberOf reads a task's own number out of a log key. A job's key begins
// with a letter, so this returns false for one.
func taskNumberOf(logKey string) (int, bool) {
	number, err := strconv.Atoi(logKey)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
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

// ErrNoRecord means something asked for a record the log does not hold.
var ErrNoRecord = errors.New("there is no record with that number, so check the number with the tasks command")
