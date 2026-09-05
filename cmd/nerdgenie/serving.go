// The loops one running agent turns: the socket, the queue drainer, the job
// driver, and the watchdog feed. They all stop together, because a program that
// can no longer accept must not go on holding its run lock.

package main

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// serve takes connections on the socket, drains the queue beside it, and runs
// the jobs that fall due, until the context is done.
func (running *agent) serve(ctx context.Context) error {
	// The three loops beside the socket run on a context of their own, which is
	// cancelled the moment Serve comes back. Without that, a socket that can no
	// longer accept — "too many open files" is the everyday cause — leaves them
	// looping on a context that is still alive, and the program hangs forever
	// holding its run lock while the service manager waits for an exit that
	// never comes.
	beside, stopThem := context.WithCancel(ctx)
	defer stopThem()

	alongside := []func(context.Context){
		running.drain, running.runDueJobs, running.feedTheWatchdog, running.openTheSignalChannel,
	}
	working := make(chan struct{}, len(alongside))
	for _, work := range alongside {
		go func() {
			defer func() { working <- struct{}{} }()
			work(beside)
		}()
	}

	err := running.socket.Serve(ctx)
	stopThem()
	for range alongside {
		<-working
	}
	return err
}

// feedTheWatchdog keeps telling the service manager that the program is alive
// for as long as it is. On a machine that started the program by hand there is
// no watchdog to feed and this returns at once.
func (running *agent) feedTheWatchdog(ctx context.Context) {
	if err := running.guard.FeedWatchdog(ctx); err != nil && ctx.Err() == nil {
		running.note("the service manager is no longer being told the program is alive: " + err.Error())
	}
}

// drain takes every message off the queue and hands it to the router, waiting on
// the queue's own nudge in between rather than asking again and again.
func (running *agent) drain(ctx context.Context) {
	for ctx.Err() == nil {
		if running.takeWhatIsWaiting(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
		case <-running.queue.Arrived():
		}
	}
}

// takeWhatIsWaiting routes up to one pass of queued messages and says whether it
// stopped at the bound, which means more are waiting and the caller should come
// straight back rather than wait for a nudge.
func (running *agent) takeWhatIsWaiting(ctx context.Context) bool {
	for taken := 0; taken < maxMessagesInOnePass; taken++ {
		queued, held, err := running.queue.Take(ctx)
		if err != nil {
			running.note("a message could not be taken off the queue: " + err.Error())
			return false
		}
		if !held {
			return false
		}
		running.holdTheMessage(queued.Sequence)
		if _, err := running.router.Route(ctx, queued.Message); err != nil {
			running.note("a message was not answered: " + err.Error())
		}
		// A message that started a task is finished by the task, whenever that
		// ends; everything else is finished here and now.
		if running.stillHolding() {
			running.finishTheMessage(queued.Sequence)
		}
	}
	return true
}

// holdTheMessage says which message the drainer is working on, so that a task
// started from it can take it over and finish it when the task ends.
func (running *agent) holdTheMessage(sequence int64) {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	running.holding = sequence
	running.handedOver = false
}

// tookTheMessage hands the message the drainer is holding to the task that is
// about to run, and gives back the function that finishes it.
func (running *agent) tookTheMessage() func() {
	running.busyGuard.Lock()
	sequence := running.holding
	running.handedOver = true
	running.busyGuard.Unlock()
	return func() { running.finishTheMessage(sequence) }
}

// stillHolding says the drainer still owns the message it took, which is true
// for everything but a message that started a task.
func (running *agent) stillHolding() bool {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	return !running.handedOver
}

// finishTheMessage marks one message done, so that a restart does not hand it
// out again.
func (running *agent) finishTheMessage(sequence int64) {
	if err := running.queue.Done(context.Background(), sequence); err != nil {
		running.note("a finished message could not be marked done: " + err.Error())
	}
}

// runDueJobs runs the task a job has due whenever nothing else is running. The
// job store says how long there is until the next moment worth looking at, and a
// pass that finds nothing rests, so that work already past its moment cannot
// turn the wait into a spin.
func (running *agent) runDueJobs(ctx context.Context) {
	for ctx.Err() == nil {
		if err := running.jobs.Wait(ctx); err != nil {
			return
		}
		found := false
		// Nothing is started while a task is already running or while the guard
		// says no, and the rest at the end of the round is taken either way: the
		// job store's own wait comes back at once for a moment already past, so
		// a round that skipped the rest would spin a whole core for as long as
		// the task ran.
		if !running.loopIsBusy() && running.guard.WhyNoNewTask() == "" {
			started, err := running.runWhatIsDue(ctx)
			if err != nil {
				running.note("a scheduled task did not finish: " + err.Error())
			}
			found = started
		}
		if !found {
			if err := clock.System().Sleep(ctx, restBetweenJobChecks); err != nil {
				return
			}
		}
	}
}

// jobSession is the name a job's task holds its turn lease under. A job's task
// comes from no screen, so it is named for the job rather than for a channel
// and a sender, and every task of one job takes its turn under the same name.
func jobSession(due contract.TaskToRun) string {
	return "job:" + due.JobID
}

// runWhatIsDue runs the task a job has due now, holding the loop for as long as
// it runs, the way a person's task holds it. A message the person types
// meanwhile then finds the loop held and is delivered to the running task,
// where it lands as a correction or as a stop, rather than taking the loop for
// a task of its own that waits behind the whole job. The loop is taken before
// the store is asked, because a task taken from the store cannot be taken again
// until its hour runs out, so one taken beside a person's task would sit there,
// taken and never run; a pass that finds the loop held finds nothing and rests.
//
// A task the nightly self-check owns is run by the check itself: it asks the
// memory its own questions and dry runs the skills, and handing it to the model
// instead would spend a whole task asking a model to do what the harness can do
// for nothing. The check runs nothing through the loop, so the loop is freed
// before it runs, and a message typed meanwhile starts a task of its own as it
// should, rather than waiting in the loop for a task that is not running.
func (running *agent) runWhatIsDue(ctx context.Context) (bool, error) {
	if !running.takeTheLoop() {
		return false, nil
	}
	due, there, err := running.jobs.NextTask(ctx, clock.System().Now())
	if err != nil {
		running.freeTheLoop()
		return false, fmt.Errorf("cannot ask the jobs which task is due now: %w", err)
	}
	if !there {
		running.freeTheLoop()
		return false, nil
	}
	if running.nightly.Handles(due) {
		running.freeTheLoop()
		if _, err := running.nightly.Run(ctx, due); err != nil {
			return true, fmt.Errorf("the nightly self-check did not finish: %w", err)
		}
		return true, nil
	}
	defer running.freeTheLoop()
	// The task has already been taken from the store above, and a task taken
	// once cannot be taken again, so the loop is handed the task itself rather
	// than asked for the next one: asking would take the job's second task and
	// leave the first sitting taken until its budget ran out.
	return true, running.loop.RunJobTask(ctx, due, throughTheLedger(running.userChannel(), running.guard))
}
