package main

import (
	"context"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
)

// The waits the wiring takes when a task is ending. They are short because a
// person watching the screen is waiting through them.
const (
	// aMomentForATaskToEnd is how long a new message waits for a task that is
	// closing before it is handed to that task instead.
	aMomentForATaskToEnd = 500 * time.Millisecond
	// lookAgainAfter is how often that wait looks again.
	lookAgainAfter = 20 * time.Millisecond
)

// startTask hands one message to the turn loop. A message that arrives while a
// task is already running is delivered to that task instead, which is how a
// correction reaches the work it corrects rather than waiting for it to end.
//
// A message that arrives when nothing is running may still belong to the task
// this screen put down: the answer to a question the model asked, or the word
// that carries on a task that stopped. The memory of that screen's newest task
// says which, and the loop is given its number so that the work goes on under
// the number it already had.
func (running *agent) startTask(ctx context.Context, lastTasks *screenTasks, message contract.Inbound) error {
	where, found := running.channelNamed(message.Channel)
	if !found {
		return fmt.Errorf("the message came through the channel %q, which is not one this agent is running, so the reply would have nowhere to go", message.Channel)
	}
	// Nothing new is started while the guard says no, which is what the
	// crash-loop breaker and the updater's drain marker both work through. The
	// guard says why in its own words, because the reason is the user's to hear.
	if why := running.guard.WhyNoNewTask(); why != "" {
		return where.Send(ctx, why)
	}
	if !running.takeTheLoopWithinAMoment() {
		return running.loop.Deliver(message)
	}

	// The task runs beside the drainer rather than inside it. The drainer is
	// the one path a command travels, so a task run inside it would hold every
	// later message behind itself: the person would press Escape and nothing
	// would happen until the model had answered, which is exactly what the
	// first human trial found.
	finished := running.tookTheMessage()
	// One session runs one turn at a time, and a turn that outlives the turn cap
	// is stopped: both are the guard's, and both hold whether the work came from
	// a person or from a job. The channel the task answers on writes every reply
	// into the log before it sends it, so a crash between the two cannot lose
	// the answer and the next start sends again what never arrived.
	session := screenNamed(message.Channel, message.Sender)
	carryOn := lastTasks.taskToCarryOn(session, message.Text)
	answering := throughTheLedger(where, running.guard)
	go func() {
		defer running.freeTheLoop()
		defer finished()
		err := running.guard.RunTurn(context.WithoutCancel(ctx), session, func(turn context.Context) error {
			outcome, err := running.loop.Run(turn,
				loop.Task{Message: message, Channel: answering, ResumeID: carryOn})
			// Where the task ended is written down whatever happened, so that a
			// task that could not be picked up again is not picked up again and
			// again by every message that follows it.
			lastTasks.remember(session, outcome)
			return err
		})
		if err != nil {
			running.note("a task did not finish: " + err.Error())
		}
	}()
	return nil
}

// takeTheLoopWithinAMoment takes the loop, waiting a moment for a task that is
// just ending.
//
// A task sends its reply and then takes a little longer to write its record and
// close, and a person who types the moment the answer appears would otherwise
// have their message handed to the task that is already over, where nothing
// would ever read it. The wait is short and bounded: past it the task really is
// a long one, and the message belongs to it as a correction.
func (running *agent) takeTheLoopWithinAMoment() bool {
	waited := time.Duration(0)
	for {
		if running.takeTheLoop() {
			return true
		}
		if waited >= aMomentForATaskToEnd {
			return false
		}
		time.Sleep(lookAgainAfter)
		waited += lookAgainAfter
	}
}

// takeTheLoop says this caller may run a task, and says no when one is already
// running. The loop's own Running is the record's number, which a task has only
// after its first tool call, so a question answered with no tools would look
// like an idle agent to a second message arriving beside it.
func (running *agent) takeTheLoop() bool {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	if running.busy {
		return false
	}
	running.busy = true
	return true
}

// freeTheLoop says the task has ended.
func (running *agent) freeTheLoop() {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	running.busy = false
}

// loopIsBusy says whether a task is running, for the status a screen is sent. A
// model call in flight counts too, because a plain reply is not a task and the
// person still wants to see that the agent is thinking.
func (running *agent) loopIsBusy() bool {
	running.busyGuard.Lock()
	busy := running.busy
	running.busyGuard.Unlock()
	return busy || (running.watched != nil && running.watched.calling())
}

// stopTask asks the running task to stop, which is what Escape at the terminal
// falls back to when no command answers to the word.
func (running *agent) stopTask(_ context.Context) error {
	running.loop.Stop()
	return nil
}
