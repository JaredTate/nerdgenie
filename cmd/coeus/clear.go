package main

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// clearName is the slash command that empties the screen.
const clearName = "clear"

// theClearedText is the one line the clear command answers with.
const theClearedText = "cleared: the next message starts a fresh task"

// clearCommand is the "/clear" command: it stops the task that is running, if
// there is one, forgets that the terminal had a task to carry on, so that the
// next message starts a fresh task rather than answering a question the person
// can no longer see, and tells every attached screen to empty its transcript.
//
// The command sends its own reply rather than handing text back to the router,
// because the router sends words alone and the screen has to be told to clear
// by the envelope's Clear field. It is kept to the terminal, because only the
// terminal has a transcript, and because the memory of tasks is kept by screen,
// which is a channel and a sender, and a command is told only its channel.
func (running *agent) clearCommand(lastTasks *screenTasks) contract.Command {
	return contract.Command{
		Name:         clearName,
		Help:         "Empty the screen, stop the running task, and start the next message as a fresh task.",
		TerminalOnly: true,
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			running.loop.Stop()
			running.waitForTheTaskToEnd()
			// The socket names its one sender after the channel, which is what
			// starting.go reads off every message the terminal sends.
			lastTasks.forget(screenNamed(contract.TerminalChannelName, contract.TerminalChannelName))
			return "", running.stream.Publish(contract.SocketEnvelope{
				Type:  contract.SocketReply,
				Text:  theClearedText,
				Clear: true,
			})
		},
	}
}

// waitForTheTaskToEnd waits a moment for the task that was asked to stop to
// write down where it ended, so that forgetting it is not undone a moment later
// by the task remembering itself. A task that has just answered is still closing
// when the person types the next line, and a task that was stopped mid-call ends
// when its call does. The wait is the same short, bounded one a new message
// takes; past it the task is a long one, and when it ends it is remembered as
// stopped, which only the word "continue" picks up.
func (running *agent) waitForTheTaskToEnd() {
	if running.takeTheLoopWithinAMoment() {
		running.freeTheLoop()
	}
}
