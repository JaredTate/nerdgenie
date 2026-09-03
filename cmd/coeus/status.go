package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/channel"
	"github.com/JaredTate/coeus/internal/contract"
)

// statusForAScreen is what a screen is told the moment it attaches and on every
// heartbeat: which model is in use, which task is running, that the agent is
// answering for itself, and the command list the palette is filled from.
func (running *agent) statusForAScreen() map[string]string {
	listed := strings.Builder{}
	for _, one := range running.registry.All() {
		listed.WriteString(channel.CommandPrefix + one.Name + contract.StatusCommandSeparator + one.Help + "\n")
	}

	state := contract.StateIdle
	if running.loopIsBusy() {
		state = contract.StateThinking
	}
	fields := map[string]string{
		contract.StatusFieldModel:    running.model.Name(),
		contract.StatusFieldTask:     running.loop.Running(),
		contract.StatusFieldState:    state,
		contract.StatusFieldBudget:   running.budgetLine(),
		contract.StatusFieldHealthy:  "true",
		contract.StatusFieldCommands: strings.TrimRight(listed.String(), "\n"),
	}
	running.watched.fillStatus(fields)
	if line := running.lastRecordLine(); line != "" {
		fields[contract.StatusFieldRecordLine] = line
	}
	if line := running.lastToolLine(); line != "" {
		fields[contract.StatusFieldToolLine] = line
	}
	return fields
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
	running.busyGuard.Lock()
	running.recordLine = line
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
	running.toolLine = line
	running.busyGuard.Unlock()
	running.tellTheScreens()
}

// lastToolLine is the newest line about a tool call.
func (running *agent) lastToolLine() string {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	return running.toolLine
}
