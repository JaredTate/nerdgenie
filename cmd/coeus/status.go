package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/channel"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
)

// statusForAScreen is what a screen is told the moment it attaches and on every
// heartbeat: which model is in use, which task is running, that the agent is
// answering for itself, and the command list the palette is filled from.
func (running *agent) statusForAScreen() map[string]string {
	// Every piece is looked at before it is read. The event stream sends its
	// first status five seconds after it is built, and it is built while the
	// rest of the agent is still being put together; a panic in that goroutine
	// cannot be recovered, so it would take the whole program with it.
	fields := map[string]string{
		contract.StatusFieldState:   contract.StateIdle,
		contract.StatusFieldHealthy: "true",
	}
	if running.model != nil {
		fields[contract.StatusFieldModel] = running.model.Name()
		fields[contract.StatusFieldBudget] = running.budgetLine()
	}
	if running.watched != nil {
		running.watched.fillStatus(fields)
	}
	if running.loop != nil {
		fields[contract.StatusFieldTask] = running.loop.Running()
		running.fillTheJob(fields)
	}
	if running.loopIsBusy() {
		fields[contract.StatusFieldState] = contract.StateThinking
	}
	if running.guard != nil && !running.guard.Healthy() {
		fields[contract.StatusFieldHealthy] = "false"
	}
	if running.registry != nil {
		fields[contract.StatusFieldCommands] = running.commandList()
	}
	if line := running.lastRecordLine(); line != "" {
		fields[contract.StatusFieldRecordLine] = line
	}
	if line := running.lastToolLine(); line != "" {
		fields[contract.StatusFieldToolLine] = line
	}
	return fields
}

// fillTheJob writes the job the running task belongs to, which is what the side
// panel draws: the job's number, its ask, which of its tasks is running, and
// its task list. All four are sent empty when the running task is a person's
// or nothing is running, so that a screen goes back to its count of jobs rather
// than keeping the job it drew last. A job whose record cannot be read is
// still named by its number and its task, because a person watching a job
// that has just gone wrong wants to know which one it was.
func (running *agent) fillTheJob(fields map[string]string) {
	for _, field := range []string{
		contract.StatusFieldJob, contract.StatusFieldJobAsk, contract.StatusFieldJobTask, contract.StatusFieldJobTasks,
	} {
		fields[field] = ""
	}
	fromJob, there := running.loop.RunningJobTask()
	if !there || running.jobs == nil {
		return
	}
	held, err := running.jobs.Load(context.Background(), fromJob.JobID)
	if err != nil {
		fields[contract.StatusFieldJob] = fromJob.JobID
		fields[contract.StatusFieldJobTask] = fromJob.TaskID
		return
	}
	fillTheJobFields(fields, fromJob, held)
}

// fillTheJobFields writes the four job fields from the job's record and the
// task of it that is running.
func fillTheJobFields(fields map[string]string, fromJob contract.TaskToRun, held contract.Record) {
	fields[contract.StatusFieldJob] = fromJob.JobID
	fields[contract.StatusFieldJobAsk] = onOneLine(held.Goal.Ask)
	fields[contract.StatusFieldJobTask] = fromJob.TaskID
	fields[contract.StatusFieldJobTasks] = contract.JobTaskLines(held.Work.Tasks)
}

// commandList is the palette: one command per line, its name and its help with
// the separator the screen reads between them.
func (running *agent) commandList() string {
	listed := strings.Builder{}
	for _, one := range running.registry.All() {
		listed.WriteString(channel.CommandPrefix + one.Name + contract.StatusCommandSeparator + one.Help + "\n")
	}
	return strings.TrimRight(listed.String(), "\n")
}

// budgetLine says in plain words what one task may spend, which is the caps the
// configuration set.
func (running *agent) budgetLine() string {
	caps := running.settings.Caps
	return fmt.Sprintf("%d rounds, %s per task", caps.RoundsPerTask, plainDuration(caps.TimePerTask))
}

// plainDuration writes a length of time the way a person says it.
func plainDuration(long time.Duration) string {
	switch {
	case long >= time.Hour && long%time.Hour == 0:
		return fmt.Sprintf("%dh", int(long/time.Hour))
	case long >= time.Minute:
		return fmt.Sprintf("%dm", int(long/time.Minute))
	default:
		return long.String()
	}
}

// tellTheScreens sends the status to every attached screen at once, rather than
// leaving it until the next heartbeat, which is what makes a call that has just
// begun or just ended show up straight away.
func (running *agent) tellTheScreens() {
	if running.stream == nil || running.registry == nil {
		return
	}
	_ = running.stream.Publish(contract.SocketEnvelope{
		Type:   contract.SocketStatus,
		Fields: running.statusForAScreen(),
	})
}

// noteRecordLine remembers the newest line about a task or a job and tells the
// screens at once, so that a person sees work start and finish as it happens.
func (running *agent) noteRecordLine(line string) {
	// A task beginning is where the data boundary is rolled, so that no two
	// tasks ever mark their tool results the same way. This is the one place
	// that fires for a message and for a job's task alike.
	if running.builder != nil && strings.Contains(line, " started"+loop.RecordLineSeparator) {
		running.builder.startingATask()
	}
	running.busyGuard.Lock()
	running.recordLine = onOneLine(line)
	running.busyGuard.Unlock()
	running.tellTheScreens()
}

// lastRecordLine is the newest line about a task or a job.
func (running *agent) lastRecordLine() string {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	return running.recordLine
}

// noteToolLine remembers the line for the tool call in flight and tells the
// screens at once, so that a person watching seven tool calls go by sees each
// one rather than a spinner.
func (running *agent) noteToolLine(line string) {
	running.busyGuard.Lock()
	running.toolLine = onOneLine(line)
	running.busyGuard.Unlock()
	running.tellTheScreens()
}

// lastToolLine is the newest line about a tool call.
func (running *agent) lastToolLine() string {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	return running.toolLine
}

// onOneLine folds text onto one line. The strip draws these in one row beside
// everything else, and a line break in one of them would tear the frame in two;
// what a person asked for is their own text and may hold anything at all.
func onOneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
