// The one place every slash command the program answers is put together. Each
// package exports what it owns as a contract.Command value and this file
// registers it, so that no two workers ever edit the same registration.

package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/JaredTate/coeus/internal/browser"
	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	signalchannel "github.com/JaredTate/coeus/internal/signal"
	browserskill "github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/vault"
)

// registerCommands fills the one registry with every slash command each package
// owns, in the order the help listing prints them. The memory of what each
// screen's newest task was doing is handed in because the clear command
// forgets from it.
func (running *agent) registerCommands(lastTasks *screenTasks) error {
	running.registry = command.NewRegistry()
	core := command.New(running.registry, command.Deps{
		Settings:        running.settings,
		Store:           running.events,
		Jobs:            running.jobs,
		ResumeJob:       running.jobs.Resume,
		CurrentModel:    running.currentModel,
		SetModel:        running.useTheModel,
		CostSoFar:       running.costSoFar,
		PendingPreviews: running.previews.list,
		Answer:          running.previews.answer,
		Channels:        running.everyChannel,
	})

	all := append(onlyTheOnesThatWork(core.All()),
		running.loop.TasksCommand(),
		running.loop.StopCommand(),
		running.jobs.JobsCommand(),
		running.jobs.CronCommand(),
		running.skills.Command(),
		browserskill.WalkCommand(running.walkOptions()),
		browser.ScreenCommand(running.browser),
		vault.NewCommand(running.secrets),
		running.memories.Command(),
		running.clearCommand(lastTasks),
		running.thinkCommand(),
		readyCommand())
	if running.settings.SignalAccount != "" {
		all = append(all, signalchannel.PairCommand(running.pairingStore()))
	}

	for _, one := range all {
		if err := running.registry.Register(one); err != nil {
			return err
		}
	}
	return nil
}

// theCommandsWithNothingBehindThem are the core commands this program cannot
// carry out. There is no session store, so "/new" and "/sessions" could only
// answer with the name of a Go field somebody has to fill in, and a command that
// can only disappoint is better not offered: the help listing and the palette
// are read as a promise.
var theCommandsWithNothingBehindThem = []string{"new", "sessions"}

// onlyTheOnesThatWork drops the commands nothing stands behind yet.
func onlyTheOnesThatWork(all []contract.Command) []contract.Command {
	kept := make([]contract.Command, 0, len(all))
	for _, one := range all {
		if !slices.Contains(theCommandsWithNothingBehindThem, one.Name) {
			kept = append(kept, one)
		}
	}
	return kept
}

// currentModel is the alias in use, which "/model" prints and the status carries.
func (running *agent) currentModel() string {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	if running.modelInUse != "" {
		return running.modelInUse
	}
	return running.settings.DefaultModel
}

// useTheModel makes one alias the model this session talks to, building it the
// same way the first one was built, so that a switch is a real switch rather
// than a note somebody has to act on.
func (running *agent) useTheModel(alias string) error {
	if _, found := aliasNamed(running.settings, alias); !found {
		return fmt.Errorf("config.toml names no model called %q, so run /model on its own to see the ones it does name", alias)
	}
	settings := running.settings
	settings.DefaultModel = alias
	settings.FallbackChain = nil

	built, err := openTheChain(context.Background(), running, settings)
	if err != nil {
		return fmt.Errorf("the model %q could not be reached, so the one in use has not changed: %w", alias, err)
	}
	running.watched.use(built)

	running.busyGuard.Lock()
	running.modelInUse = alias
	running.busyGuard.Unlock()
	return nil
}

// costSoFar is what this session has spent, which "/status" prints.
func (running *agent) costSoFar() contract.CostLine {
	return running.watched.costSoFar()
}

// everyChannel is every channel the program is listening on, which "/status"
// asks the health of.
func (running *agent) everyChannel() []contract.Channel {
	listening := []contract.Channel{running.userChannel()}
	if talking := running.signalChannel(); talking != nil {
		listening = append(listening, talking)
	}
	return listening
}

// pairingStore is where the pairing codes live. It is the channel's own store
// when Signal is running, so that "/pair" and the sender writing in are working
// on one set of codes rather than two.
func (running *agent) pairingStore() *signalchannel.Pairing {
	if talking := running.signalChannel(); talking != nil {
		return talking.Pairing()
	}
	made, err := signalchannel.NewPairing(running.home, clock.System())
	if err != nil {
		running.note("the pairing codes could not be opened, so /pair will say so: " + err.Error())
		return nil
	}
	return made
}

// walkOptions is what the /walk command works with. The browser is only put in
// when there really is one: a nil pointer stored in an interface is not nil, and
// the command would then try to drive a browser that is not there instead of
// saying plainly that the browser tools are switched off.
func (running *agent) walkOptions() browserskill.WalkOptions {
	walk := browserskill.WalkOptions{
		Model:  running.model,
		Ask:    running.userChannel().ShowPreview,
		Skills: running.skills,
		Home:   running.home,
	}
	if running.browser != nil {
		walk.Browser = running.browser
	}
	return walk
}

// readyCommand answers a health check. It is a slash command rather than
// anything of its own, because then the answer travels the whole way a person's
// message travels: in on the socket, through the queue, out through the router.
func readyCommand() contract.Command {
	return contract.Command{
		Name: readyName,
		Help: "Answers while the agent is running, which is how a health check knows it is up.",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return "ready", nil
		},
	}
}
