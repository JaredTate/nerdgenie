package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/browser"
	"github.com/JaredTate/nerdgenie/internal/channel"
	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/sandbox"
	"github.com/JaredTate/nerdgenie/internal/skill"
	browserskill "github.com/JaredTate/nerdgenie/internal/skill/browser"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

// buildingToolsTakes is how long one task's tool registry may take to build. It
// reads the user's own tools folder, so it touches the disk and needs a bound.
const buildingToolsTakes = 30 * time.Second

// openTheFront opens everything a message meets on its way in and out: the
// model, the event stream, the local socket, the fence, the tools, the skills,
// the turn loop, the commands, and the router.
//
// The memory of what each screen's newest task was doing is rebuilt here out of
// the event log and handed to the two things that read it: the router, whose
// one function that starts a task is where a message is handed to a task already
// under way, and the commands, because the clear command forgets from it.
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
	lastTasks := running.rememberedTasks(ctx)
	if err := running.registerCommands(lastTasks); err != nil {
		return err
	}
	return running.openTheRouter(lastTasks)
}

// rememberedTasks is the memory of what each screen's newest task was doing,
// rebuilt out of the event log so that a restart forgets nothing a person could
// pick up. A log the rebuild cannot read is noted and the agent starts with an
// empty memory, so the next message from each screen starts a fresh task rather
// than nothing at all. A task the shutdown left running is named in the record
// line, so the first status a screen gets tells the person it was interrupted
// and to say "continue".
func (running *agent) rememberedTasks(ctx context.Context) *screenTasks {
	lastTasks, err := rememberedFromTheLog(ctx, running.events)
	if err != nil {
		running.note("the memory of what each screen was last doing could not be rebuilt from the log, so the next message from each screen starts a fresh task: " + err.Error())
	}
	if line := interruptedRecordLine(lastTasks.interrupted); line != "" {
		running.noteRecordLine(line)
	}
	return lastTasks
}

// interruptedRecordLine is the one line the first status names an interrupted
// task in, or nothing when the shutdown cut none off. It says which task and
// what to do, because the person is being told about work they did not stop and
// can pick up again.
func interruptedRecordLine(numbers []string) string {
	switch len(numbers) {
	case 0:
		return ""
	case 1:
		return "task " + numbers[0] + " was interrupted; your next message picks it up"
	default:
		return "tasks " + strings.Join(numbers, ", ") + " were interrupted; your next message picks the newest up"
	}
}

// openTheModelAndTheScreens opens the model the configuration names, the event
// stream every screen reads, and the local socket the screens attach to.
func (running *agent) openTheModelAndTheScreens() error {
	model, err := running.openModel(context.Background())
	if err != nil {
		return err
	}
	running.watched = newWatchedModel(model, clock.System(), running.tellTheScreens)
	running.model = running.watched
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
		Show:           running.theShowAnswerer().answer,
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
	// The box is made before the builder because the builder reads the skill
	// list through it at the start of every task, which is how the model is
	// told what skills it may load.
	running.skillsBox = &skillsBox{}
	running.builder = newPerTaskContext(running.home, running.settings, running.skillsBox, running.note)
	running.fence = running.openTheFence()
	running.browser = running.openTheBrowser()
	running.desktop = running.openTheDesktop()

	walking, stopWalking := withinTheToolWalkLimit(ctx)
	defer stopWalking()
	// The user's own tools are asked what they are once, here, and every
	// registry after this one takes the answer rather than asking again.
	userTools, err := tool.LoadUserTools(walking, running.toolSettings("", nil))
	if err != nil {
		return err
	}
	running.userTools = userTools
	built, err := tool.New(walking, running.toolSettings("", nil))
	if err != nil {
		return err
	}
	running.tools = built
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
	running.installTheShippedSkill(ctx, store)
	return nil
}

// installTheShippedSkill writes the quality skill into the skills folder when
// there is no copy of it there. A skill that ships with the program is no use to
// anybody until it is on disk, and a person who has written their own copy keeps
// it: the folder is only filled when it is empty of that name.
func (running *agent) installTheShippedSkill(ctx context.Context, store *skill.Store) {
	if _, err := store.Load(ctx, browserskill.QASkillName); err == nil {
		return
	}
	if err := browserskill.InstallQASkill(ctx, store); err != nil {
		running.note("the quality skill could not be installed, so /skills will not list it: " + err.Error())
	}
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
		BufferedEvents: running.settings.Caps.BufferedBrowserEvents,
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

// theSandboxIsOffNote is what the agent says at start when the sandbox setting
// is off, which is what a fresh install runs as. It is the only warning a person
// gets that commands are running on their machine as them, so it says both what
// is gone and what is left.
const theSandboxIsOffNote = "the sandbox is off: commands run straight on this machine as you, and the ask-me-first list is the gate"

// openTheFence builds what every shell command runs through: the direct runner
// when the sandbox setting is off, which is the default, and the bwrap fence
// when the configuration asks for it. A machine that cannot build a fence it was
// asked for is not a machine that must not run: the agent comes up without it,
// says so, and the shell tool refuses rather than running loose.
func (running *agent) openTheFence() contract.Sandbox {
	if running.settings.SandboxMode() == contract.SandboxOff {
		running.note(theSandboxIsOffNote)
		return sandbox.NewDirect(sandbox.Settings{OutputCap: running.settings.Caps.ToolOutputBytes})
	}
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
		// The fence unshares the network unless it is told otherwise, and the
		// shell tool is how the agent installs a package, clones a repository,
		// and calls an interface on this machine, so the agent's own fence
		// keeps its network. What a command may reach is ruled on by the
		// permission function and bounded by the roots, rather than by taking
		// the network away from every command there is.
		Network: true,
		// The browser profile is the agent's own logins and the backups are its
		// whole history, so neither may sit inside a folder a command can read,
		// wherever the configuration has put them.
		AlsoOutside: []string{running.settings.BrowserProfilePath, running.settings.BackupPath},
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
		Configuration:    running.settings,
		Home:             running.home,
		UserTools:        running.userTools,
		UserHome:         userHomeOrEmpty(),
		TaskID:           taskID,
		Note:             running.note,
		Log:              running.events,
		Sandbox:          running.fence,
		Permission:       running.decider,
		Memory:           running.memories,
		Jobs:             running.jobs,
		Clock:            clock.System(),
		NerdGenieProgram: thisProgramOrEmpty(),
	}
	settings.Skills = running.skillsBox
	if running.browser != nil {
		settings.Browser = running.browser
		settings.Credentials = running.browser.Credentials
		settings.TwoFactorCode = running.browser.TwoFactorCode
		settings.AskUser = running.browser.AskUser
	}
	if running.desktop != nil {
		settings.Desktop = running.desktop
	}
	if records != nil {
		settings.Records = records
		settings.Results = records
	}
	// A job's reports are read by their labels out of the log, so a task of
	// the job can read what the tasks before it reported.
	settings.Reports = record.ReportsIn(running.events)
	return settings
}

// toolsForTask builds the registry one task calls through, so that the task tool
// writes that task's record and a label such as r7 reads that task's results.
func (running *agent) toolsForTask(taskID string, records loop.TaskRecord) (contract.ToolRegistry, error) {
	ctx, giveUp := withinTheToolWalkLimit(context.Background())
	defer giveUp()
	return tool.New(ctx, running.toolSettings(taskID, records))
}

// withinTheToolWalkLimit bounds one walk of the user's tools folder. Every
// program in it is asked what it is, under a timeout of the tool package's own,
// so a folder of slow programs would otherwise hold startup open for as long as
// it liked.
func withinTheToolWalkLimit(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, buildingToolsTakes)
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

// openTheLoop builds the turn loop over everything the agent owns. The
// reply's pieces go to the screens through the socket as the model writes them,
// and a retry withdraws them, which is brief 6.8.
func (running *agent) openTheLoop() error {
	built, err := loop.New(loop.Options{
		Model:        running.model,
		Tools:        running.tools,
		ToolsForTask: running.toolsForTask,
		Permission:   running.decider,
		Store:        running.events,
		Clock:        clock.System(),
		Context:      running.builder,
		Jobs:         running.jobs,
		Memory:       running.memories,
		Skills:       running.skillsBox,
		Sandbox:      running.fence,
		Caps:         running.settings.Caps,
		ToolLine:     running.noteToolLine,
		RecordLine:   running.noteRecordLine,
		Deltas:       running.streamReplyPiece,
	})
	if err != nil {
		return err
	}
	running.loop = built

	running.nightly, err = replay.NewNightly(replay.NightlySettings{
		Jobs:   running.jobs,
		Memory: running.memories,
		Skills: running.skillsBox,
		DryRun: running.dryRunOneSkill,
		Send:   running.sendToTheUserHere,
	})
	return err
}

// dryRunOneSkill is the skill store's own dry run, which contract.Skill does not
// carry, so it is passed to the self-check as a function of its own.
func (running *agent) dryRunOneSkill(ctx context.Context, name string) (string, error) {
	if running.skills == nil {
		return "", fmt.Errorf("there are no skills open, so %q cannot be dry run", name)
	}
	return running.skills.DryRun(ctx, name)
}

// sendToTheUserHere puts one line in front of the user on the channel they are
// normally talked to on.
func (running *agent) sendToTheUserHere(ctx context.Context, text string) error {
	return running.sendToTheUser(ctx, "", text)
}

// openTheRouter gives the router the five things it needs from the rest of the
// program, and the one function that starts a task the memory of what each
// screen's newest task was doing, which is the only place a message can be
// handed to a task that is already under way.
func (running *agent) openTheRouter(lastTasks *screenTasks) error {
	built, err := channel.NewRouter(channel.Routes{
		RunCommand:  running.registry.Run,
		FindChannel: running.channelNamed,
		StartTask: func(ctx context.Context, message contract.Inbound) error {
			return running.startTask(ctx, lastTasks, message)
		},
		StopTask: running.stopTask,
		Skills:   running.skillsBox,
	})
	if err != nil {
		return err
	}
	running.router = built
	return nil
}

// channelNamed finds the channel a message came through. The local socket is the
// terminal channel; Signal joins this list when its daemon is wired in.
func (running *agent) channelNamed(name string) (contract.Channel, bool) {
	if name == contract.TerminalChannelName {
		return running.userChannel(), true
	}
	if talking := running.signalChannel(); talking != nil && name == talking.Name() {
		return talking, true
	}
	return nil, false
}
