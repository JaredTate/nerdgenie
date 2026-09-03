package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/browser"
	"github.com/JaredTate/coeus/internal/channel"
	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/command"
	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/sandbox"
	signalchannel "github.com/JaredTate/coeus/internal/signal"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/tool"
	"github.com/JaredTate/coeus/internal/vault"
)

// buildingToolsTakes is how long one task's tool registry may take to build. It
// reads the user's own tools folder, so it touches the disk and needs a bound.
const buildingToolsTakes = 30 * time.Second

// openTheFront opens everything a message meets on its way in and out: the
// model, the event stream, the local socket, the fence, the tools, the skills,
// the turn loop, the commands, and the router.
func (running *agent) openTheFront(ctx context.Context) error {
	if err := running.openTheModelAndTheScreens(); err != nil {
		return err
	}
	if err := running.openTheWorkbench(ctx); err != nil {
		return err
	}
	if err := running.openTheLoop(); err != nil {
		return err
	}
	if err := running.registerCommands(); err != nil {
		return err
	}
	return running.openTheRouter()
}

// openTheModelAndTheScreens opens the model the configuration names, the event
// stream every screen reads, and the local socket the screens attach to.
func (running *agent) openTheModelAndTheScreens() error {
	model, err := running.openModel(context.Background())
	if err != nil {
		return err
	}
	running.model = model
	running.stream = channel.NewStream(channel.StreamOptions{
		Clock:  clock.System(),
		Status: running.statusForAScreen,
	})

	running.socket, err = channel.Listen(channel.Options{
		Path:           running.home.SocketFile(),
		Stream:         running.stream,
		Queue:          running.queue,
		Secrets:        running.secrets,
		Clock:          clock.System(),
		AnswerDeadline: running.settings.Caps.TimePerTurn,
	})
	return err
}

// openTheWorkbench opens what the model works with: the working-context builder,
// the sandbox fence, the browser, the tool registry every task shares, and the
// skill store.
//
// The skills and the tools each need the other, because a skill replays through
// the tools and the skill tool offers the skills. The knot is untied with a box:
// the registry is built holding an empty box, the skill store is built over that
// registry, and the box is then filled with the store. Nothing asks the box a
// question until the agent is serving, which is long after it is filled.
func (running *agent) openTheWorkbench(ctx context.Context) error {
	builder, err := workingcontext.New(workingcontext.Options{
		Home:            running.home,
		MemoryCaps:      running.settings.MemoryCaps,
		MaxOutputTokens: running.settings.Caps.OutputTokensPerCall,
	})
	if err != nil {
		return err
	}
	running.builder = builder
	running.fence = running.openTheFence()
	running.browser = running.openTheBrowser()
	running.skillsBox = &skillsBox{}

	if running.tools, err = tool.New(ctx, running.toolSettings("", nil)); err != nil {
		return err
	}
	store, err := skill.New(skill.Options{
		Home:       running.home,
		Clock:      clock.System(),
		Tools:      running.tools,
		Permission: running.decider,
		Standing:   running.decider,
		Ask:        running.userChannel().ShowPreview,
	})
	if err != nil {
		return err
	}
	running.skills = store
	running.skillsBox.fill(store)
	return nil
}

// openTheBrowser starts the Go side of the browser when the worker bundle is
// beside the binary, and reports in the doctor's own manner when it is not: the
// agent comes up, the seven browser tools refuse, and the line says what to do.
func (running *agent) openTheBrowser() *browser.Browser {
	bundle := theBrowserBundle()
	if _, err := os.Stat(bundle); err != nil {
		running.note("the browser worker is not built, so the browser tools are switched off; it belongs at " + bundle)
		return nil
	}
	node, err := exec.LookPath("node")
	if err != nil {
		running.note("node is not on the PATH, so the browser tools are switched off; install Node and start again")
		return nil
	}

	start, err := browser.ProcessStart([]string{node, bundle}, running.browserProfile(), browser.PacingHuman, running.noteLine)
	if err != nil {
		running.note("the browser worker could not be set up, so the browser tools are switched off: " + err.Error())
		return nil
	}
	made, err := browser.New(browser.Options{
		Start:          start,
		Channel:        running.userChannel(),
		Secrets:        running.secrets,
		Codes:          running.secrets,
		Clock:          clock.System(),
		HandoffTimeout: running.settings.HandoffTimeout,
		Note:           running.noteLine,
	})
	if err != nil {
		running.note("the browser could not be built, so the browser tools are switched off: " + err.Error())
		return nil
	}
	return made
}

// noteLine writes one formatted line wherever the agent writes its notes.
func (running *agent) noteLine(format string, arguments ...any) {
	running.note(fmt.Sprintf(format, arguments...))
}

// theBrowserBundle is where the browser worker is installed: beside the binary,
// under workers/browser/main.js, which is what "make build" writes.
func theBrowserBundle() string {
	program, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(program), "workers", "browser", "main.js")
}

// browserProfile is the Chrome profile the agent drives, which is never the
// user's daily one.
func (running *agent) browserProfile() string {
	if running.settings.BrowserProfilePath != "" {
		return running.settings.BrowserProfilePath
	}
	return running.home.BrowserProfile("default")
}

// userChannel is the channel the user is talked to on, with every question it
// asks written down while it waits, so that "/approve 3" and "/deny 3" can
// answer one.
func (running *agent) userChannel() contract.Channel {
	return watchedChannel{Channel: running.socket, waiting: running.previews}
}

// openTheFence builds the sandbox every shell command runs inside. A machine
// that cannot build one is not a machine that must not run: the agent comes up
// without it, says so, and the shell tool refuses rather than running loose.
func (running *agent) openTheFence() contract.Sandbox {
	userHome, err := os.UserHomeDir()
	if err != nil {
		running.note("cannot find your home directory, so no sandbox was built and the shell tool will refuse: " + err.Error())
		return nil
	}
	fence, err := sandbox.New(sandbox.Settings{
		Roots:     running.settings.SandboxRoots,
		UserHome:  userHome,
		AgentHome: running.home.Root,
		OutputCap: running.settings.Caps.ToolOutputBytes,
	})
	if err != nil {
		running.note("no sandbox could be built, so the shell tool will refuse rather than run loose: " + err.Error())
		return nil
	}
	return fence
}

// toolSettings is what every tool registry is built from. The task's number and
// its record are empty for the registry built at startup, and filled in for the
// registry one running task uses.
func (running *agent) toolSettings(taskID string, records loop.TaskRecord) tool.Settings {
	settings := tool.Settings{
		Configuration: running.settings,
		Home:          running.home,
		UserHome:      userHomeOrEmpty(),
		TaskID:        taskID,
		Note:          running.note,
		Log:           running.eventLog,
		Sandbox:       running.fence,
		Permission:    running.decider,
		Memory:        running.memories,
		Jobs:          running.jobs,
		Clock:         clock.System(),
		CoeusProgram:  thisProgramOrEmpty(),
	}
	settings.Skills = running.skillsBox
	if running.browser != nil {
		settings.Browser = running.browser
		settings.Credentials = running.browser.Credentials
		settings.TwoFactorCode = running.browser.TwoFactorCode
		settings.AskUser = running.browser.AskUser
	}
	if records != nil {
		settings.Records = records
		settings.Results = records
	}
	return settings
}

// toolsForTask builds the registry one task calls through, so that the task tool
// writes that task's record and a label such as r7 reads that task's results.
func (running *agent) toolsForTask(taskID string, records loop.TaskRecord) (contract.ToolRegistry, error) {
	ctx, giveUp := context.WithTimeout(context.Background(), buildingToolsTakes)
	defer giveUp()
	return tool.New(ctx, running.toolSettings(taskID, records))
}

// userHomeOrEmpty is the user's own home directory, or nothing when it cannot be
// found, which the tools read as "work the forbidden paths out from nothing".
func userHomeOrEmpty() string {
	found, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return found
}

// thisProgramOrEmpty is the path of the running binary, which is what sudo reads
// the administrator password from through the askpass subcommand.
func thisProgramOrEmpty() string {
	found, err := os.Executable()
	if err != nil {
		return ""
	}
	return found
}

// openTheLoop builds the turn loop over everything the agent owns.
//
// Deltas are not asked for, and that is a decision rather than an omission. Both
// provider.WithRetries and provider.NewChain hold an attempt's text back until
// that attempt has succeeded, so a delta today is not live text: the whole answer
// arrives in one lump a moment before the reply. The two then travel to a screen
// by different paths, deltas on the event stream and the reply straight out of
// the channel, and nothing orders those two paths against each other. Measured
// against the real local model on this machine, the reply won every time, and the
// terminal screen, which reads a delta arriving after a reply as the start of a
// new answer, drew the same answer twice. Live text becomes possible the day a
// channel's reply rides the same event stream its deltas do.
func (running *agent) openTheLoop() error {
	built, err := loop.New(loop.Options{
		Model:        running.model,
		Tools:        running.tools,
		ToolsForTask: running.toolsForTask,
		Permission:   running.decider,
		Store:        running.eventLog,
		Clock:        clock.System(),
		Context:      loop.TheWorkingContext(running.builder),
		Jobs:         running.jobs,
		Memory:       running.memories,
		Skills:       running.skillsBox,
		Sandbox:      running.fence,
		Caps:         running.settings.Caps,
	})
	if err != nil {
		return err
	}
	running.loop = built
	return nil
}

// openTheRouter gives the router the five things it needs from the rest of the
// program.
func (running *agent) openTheRouter() error {
	built, err := channel.NewRouter(channel.Routes{
		RunCommand:  running.registry.Run,
		FindChannel: running.channelNamed,
		StartTask:   running.startTask,
		StopTask:    running.stopTask,
		Skills:      running.skillsBox,
	})
	if err != nil {
		return err
	}
	running.router = built
	return nil
}

// startTask hands one message to the turn loop. A message that arrives while a
// task is already running is delivered to that task instead, which is how a
// correction reaches the work it corrects rather than waiting for it to end.
func (running *agent) startTask(ctx context.Context, message contract.Inbound) error {
	where, found := running.channelNamed(message.Channel)
	if !found {
		return fmt.Errorf("the message came through the channel %q, which is not one this agent is running, so the reply would have nowhere to go", message.Channel)
	}
	if !running.takeTheLoop() {
		return running.loop.Deliver(message)
	}
	defer running.freeTheLoop()
	_, err := running.loop.Run(ctx, loop.Task{Message: message, Channel: where})
	return err
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

// loopIsBusy says whether a task is running, for the status a screen is sent.
func (running *agent) loopIsBusy() bool {
	running.busyGuard.Lock()
	defer running.busyGuard.Unlock()
	return running.busy
}

// stopTask asks the running task to stop, which is what Escape at the terminal
// falls back to when no command answers to the word.
func (running *agent) stopTask(_ context.Context) error {
	running.loop.Stop()
	return nil
}

// openModel builds the model the configuration's default alias names, wrapped in
// retries, with the fallback chain behind it.
func (running *agent) openModel(ctx context.Context) (contract.Model, error) {
	options := provider.Options{Clock: clock.System(), Home: running.home, Log: running.note}
	models := []contract.Model{}

	for _, name := range append([]string{running.settings.DefaultModel}, running.settings.FallbackChain...) {
		alias, found := aliasNamed(running.settings, name)
		if !found {
			return nil, fmt.Errorf("config.toml names %q as a model to use, and no models block defines it, so add one or change the name", name)
		}
		key, err := running.keyFor(ctx, alias)
		if err != nil {
			return nil, err
		}
		withKey := options
		withKey.APIKey = key
		one, err := provider.New(alias, withKey)
		if err != nil {
			return nil, err
		}
		models = append(models, provider.WithRetries(one, withKey))
	}
	return provider.NewChain(models, options)
}

// aliasNamed finds one model alias in the configuration by its name.
func aliasNamed(settings contract.Config, name string) (contract.ModelAlias, bool) {
	for _, alias := range settings.Models {
		if alias.Name == name {
			return alias, true
		}
	}
	return contract.ModelAlias{}, false
}

// keyFor reads a model's API key out of the vault, which is where the key lives
// and where the model never sees it. An alias with no key reference needs none,
// which is every local server and both subscription programs.
func (running *agent) keyFor(ctx context.Context, alias contract.ModelAlias) (string, error) {
	if alias.KeyReference == "" {
		return "", nil
	}
	found, err := running.secrets.Resolve(ctx, alias.KeyReference)
	if err != nil {
		return "", fmt.Errorf("the model alias %q needs the key at %s, and the vault could not give it: %w", alias.Name, alias.KeyReference, err)
	}
	return found.Password, nil
}

// registerCommands fills the one registry with every slash command each package
// owns, in the order the help listing prints them.
func (running *agent) registerCommands() error {
	running.registry = command.NewRegistry()
	core := command.New(running.registry, command.Deps{
		Settings:        running.settings,
		Store:           running.eventLog,
		Jobs:            running.jobs,
		ResumeJob:       running.jobs.Resume,
		CurrentModel:    running.model.Name,
		PendingPreviews: running.previews.list,
		Answer:          running.previews.answer,
		Channels:        func() []contract.Channel { return []contract.Channel{running.userChannel()} },
	})

	all := append(core.All(),
		running.loop.TasksCommand(),
		running.loop.StopCommand(),
		running.jobs.JobsCommand(),
		running.jobs.CronCommand(),
		running.skills.Command(),
		vault.NewCommand(running.secrets),
		running.memories.Command(),
		readyCommand())
	if running.settings.SignalAccount != "" {
		pairing, err := signalchannel.NewPairing(running.home, clock.System())
		if err != nil {
			return err
		}
		all = append(all, signalchannel.PairCommand(pairing))
	}

	for _, one := range all {
		if err := running.registry.Register(one); err != nil {
			return err
		}
	}
	return nil
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

// channelNamed finds the channel a message came through. The local socket is the
// terminal channel; Signal joins this list when its daemon is wired in.
func (running *agent) channelNamed(name string) (contract.Channel, bool) {
	if name == contract.TerminalChannelName {
		return running.userChannel(), true
	}
	return nil, false
}

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
	return map[string]string{
		contract.StatusFieldModel:    running.model.Name(),
		contract.StatusFieldTask:     running.loop.Running(),
		contract.StatusFieldState:    state,
		contract.StatusFieldHealthy:  "true",
		contract.StatusFieldCommands: strings.TrimRight(listed.String(), "\n"),
	}
}

// skillsBox holds the skill store once it exists. The tool registry is built
// before the store, because the store replays through the registry, so the
// registry is handed this box and the box is filled a moment later. Every method
// answers plainly while it is empty rather than failing.
type skillsBox struct {
	store contract.Skill
}

// fill puts the real store in the box, which happens once at startup, before
// anything asks the box a question.
func (box *skillsBox) fill(store contract.Skill) { box.store = store }

// List is the skills there are, and none while the box is empty.
func (box *skillsBox) List(ctx context.Context) ([]contract.SkillSummary, error) {
	if box.store == nil {
		return nil, nil
	}
	return box.store.List(ctx)
}

// Load is one skill's body.
func (box *skillsBox) Load(ctx context.Context, name string) (string, error) {
	if box.store == nil {
		return "", fmt.Errorf("the skills are not open yet, so %q cannot be loaded", name)
	}
	return box.store.Load(ctx, name)
}

// Run replays one skill.
func (box *skillsBox) Run(ctx context.Context, name string, arguments string) (string, error) {
	if box.store == nil {
		return "", fmt.Errorf("the skills are not open yet, so %q cannot be run", name)
	}
	return box.store.Run(ctx, name, arguments)
}

// Save writes one skill folder.
func (box *skillsBox) Save(ctx context.Context, name string, files map[string][]byte) error {
	if box.store == nil {
		return fmt.Errorf("the skills are not open yet, so %q was not written", name)
	}
	return box.store.Save(ctx, name, files)
}

// Match says whether a message's words fire a saved skill.
func (box *skillsBox) Match(ctx context.Context, text string) (contract.SkillMatch, error) {
	if box.store == nil {
		return contract.SkillMatch{}, nil
	}
	return box.store.Match(ctx, text)
}
