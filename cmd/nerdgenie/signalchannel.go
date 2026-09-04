package main

import (
	"context"
	"os/exec"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	signalchannel "github.com/JaredTate/nerdgenie/internal/signal"
)

// openTheSignalChannel starts Signal when the configuration names an account.
//
// Signal is half of what Coeus is: the sentence that opens CLAUDE.md says a
// person talks to it in a terminal or through Signal. Until this ran, the only
// piece of Signal the program used was the pairing store behind "/pair", so a
// sender could be paired and then had nowhere to write.
//
// A machine with no signal-cli on its PATH is not a failure: signal-cli may be
// running in a container somewhere else, and the channel talks to that one. An
// account nobody linked switches Signal off entirely, which is the ordinary
// case on a machine that only uses the terminal.
func (running *agent) openTheSignalChannel(ctx context.Context) {
	if running.settings.SignalAccount == "" {
		return
	}
	options := signalchannel.ChannelOptions{
		Account: running.settings.SignalAccount,
		Program: theSignalProgram(running.note),
		Home:    running.home,
		Clock:   clock.System(),
		Secrets: running.secrets,
	}

	built, err := signalchannel.NewChannel(options)
	if err != nil {
		running.note("Signal is named in config.toml and could not be started, so it is switched off: " + err.Error())
		return
	}
	running.useSignal(built)

	// Receiving is what starts the daemon and the event stream, and it runs for
	// as long as the program does. Nothing waits on it: a Signal that cannot be
	// reached must never hold up the terminal.
	arriving, err := built.Receive(ctx)
	if err != nil {
		running.note("Signal could not be reached, so it is switched off until the next start: " + err.Error())
		running.useSignal(nil)
		return
	}
	running.signalMessages(ctx, arriving)
}

// useSignal puts the Signal channel where the router and the status can find it.
// It is written from the goroutine that starts Signal and read from every turn,
// so it is held under the same lock as everything else the agent changes while
// it runs.
func (running *agent) useSignal(built *signalchannel.Channel) {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	running.signal = built
}

// signalChannel is the Signal channel, or nothing when Signal is switched off or
// has not come up yet.
func (running *agent) signalChannel() *signalchannel.Channel {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	return running.signal
}

// theSignalProgram is the signal-cli to start, or nothing when it is not on the
// PATH, in which case the channel talks to a daemon somebody else is running.
func theSignalProgram(note func(line string)) string {
	found, err := exec.LookPath(signalProgramName)
	if err != nil {
		note("signal-cli is not on the PATH, so Coeus will talk to a daemon somebody else is running rather than start one")
		return ""
	}
	return found
}

// signalMessages hands the messages Signal received to the same queue every
// other channel feeds, so that the loop never learns which channel it is
// talking to.
func (running *agent) signalMessages(ctx context.Context, arriving <-chan contract.Inbound) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, open := <-arriving:
			if !open {
				return
			}
			if _, err := running.queue.Add(ctx, message); err != nil {
				running.note("a message from Signal could not be queued: " + err.Error())
			}
		}
	}
}
