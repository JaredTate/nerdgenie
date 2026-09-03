package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/JaredTate/coeus/internal/channel"
	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/memory"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/provider"
	signalchannel "github.com/JaredTate/coeus/internal/signal"
	"github.com/JaredTate/coeus/internal/vault"
)

// serveSubcommand is the agent itself: the one long-running program that holds
// the queue, the event log, the memory, the permissions, the vault, and the
// model, and answers every screen that attaches to its local socket.
//
// This file and the subcommand table in main.go belong to the orchestrator. Each
// package exports what it owns, and everything is put together here in one
// place, so that no two workers ever edit the same lines.
var serveSubcommand = subcommand{
	name: "serve",
	help: "Runs the agent and listens on the local socket, which is what the screens attach to.",
	run:  runServe,
}

// maxMessagesInOnePass is how many queued messages one pass of the drainer takes
// before it goes round again, so that the drain loop is bounded like every other
// loop in Coeus.
const maxMessagesInOnePass = 64

// readyName is the slash command a health check sends. It is answered on the
// socket by the same path a person's message travels, so an answer proves the
// whole way in and out is working, not merely that a socket file exists.
const readyName = "readyz"

// runServe opens everything the agent owns, says where it is listening, and
// serves until the terminal interrupt or the service manager stops it.
func runServe(arguments []string, output io.Writer, problems io.Writer) int {
	if len(arguments) > 0 {
		fmt.Fprintf(problems, "coeus serve takes no arguments, and it was given %q. Run \"coeus help\" for the list.\n", strings.Join(arguments, " "))
		return contract.ExitUsage
	}
	home, err := config.HomeFolder()
	if err != nil {
		fmt.Fprintf(problems, "coeus serve: %v\n", err)
		return contract.ExitBadConfiguration
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	running, err := openAgent(ctx, home, func(line string) { fmt.Fprintln(output, line) })
	if err != nil {
		fmt.Fprintf(problems, "coeus serve: %v\n", err)
		return exitCodeFor(err)
	}
	defer func() {
		if err := running.close(); err != nil {
			fmt.Fprintf(problems, "coeus serve: stopping was not clean: %v\n", err)
		}
	}()

	fmt.Fprintf(output, "coeus is listening on %s with the model %s. Open a screen with \"coeus\".\n",
		home.SocketFile(), running.model.Name())
	if err := running.serve(ctx); err != nil {
		fmt.Fprintf(problems, "coeus serve: %v\n", err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// exitCodeFor turns what went wrong at startup into the code the service unit
// reads: a configuration a restart cannot fix stops the service and waits for a
// person, and everything else may be tried again.
func exitCodeFor(err error) int {
	problem := config.Problem{}
	if errors.As(err, &problem) {
		return contract.ExitBadConfiguration
	}
	return contract.ExitFailure
}

// agent is one running Coeus and everything it owns. Every field is built by one
// package, and this struct is the only place they meet.
type agent struct {
	home     contract.Home
	settings contract.Config
	note     func(line string)

	lock     *runLock
	eventLog *log.Log
	queue    *channel.Queue
	secrets  *vault.Vault
	memories *memory.Memory
	decider  *permission.Decider

	model    contract.Model
	stream   *channel.Stream
	socket   *channel.Socket
	registry *command.Registry
	router   *channel.Router
	turn     *firstTurn
}

// openAgent makes the home folder if it is not there, reads the configuration,
// and opens everything in the order the packages need: the stores first, because
// the event log owns the database file's identity, and then the model, the
// socket, the commands, and the router.
func openAgent(ctx context.Context, home contract.Home, note func(line string)) (*agent, error) {
	if err := makeHomeFolders(home); err != nil {
		return nil, err
	}
	settings, err := config.Load(home)
	if err != nil {
		return nil, err
	}

	running := &agent{home: home, settings: settings, note: note}
	if err := running.openTheStores(ctx); err != nil {
		return nil, errors.Join(err, running.close())
	}
	if err := running.openTheFront(ctx); err != nil {
		return nil, errors.Join(err, running.close())
	}
	return running, nil
}

// makeHomeFolders makes every folder of the layout that is not there yet, so
// that "coeus serve" comes up on a machine where nobody has run "coeus init".
func makeHomeFolders(home contract.Home) error {
	for _, folder := range home.Folders() {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			return fmt.Errorf("cannot make the folder %s that Coeus keeps its work in, so check who owns %s: %w", folder, home.Root, err)
		}
	}
	return nil
}

// openTheStores takes the run lock and opens everything that keeps something
// between runs. The event log is opened before the queue, because the log owns
// the database file's identity and a file holding only the queue's table is not
// one the log will open.
func (running *agent) openTheStores(ctx context.Context) error {
	now := clock.System()
	lock, err := takeRunLock(running.home.LockFile())
	if err != nil {
		return err
	}
	running.lock = lock

	if running.eventLog, err = log.Open(ctx, running.home.DatabaseFile()); err != nil {
		return err
	}
	running.queue, err = channel.OpenQueue(ctx, running.home.DatabaseFile(), running.settings.Caps.QueuedMessages)
	if err != nil {
		return err
	}
	if running.secrets, err = vault.Open(running.home, now); err != nil {
		return err
	}
	running.memories, err = memory.Open(ctx, running.home, running.eventLog, now, running.settings.MemoryCaps)
	if err != nil {
		return err
	}
	running.decider, err = permission.New(running.settings, now)
	return err
}

// openTheFront opens the model, the event stream, the local socket, the command
// registry, and the router that decides what each message is.
func (running *agent) openTheFront(ctx context.Context) error {
	model, err := running.openModel(ctx)
	if err != nil {
		return err
	}
	running.model = model
	running.stream = channel.NewStream()

	running.socket, err = channel.Listen(channel.Options{
		Path:    running.home.SocketFile(),
		Stream:  running.stream,
		Queue:   running.queue,
		Secrets: running.secrets,
		Clock:   clock.System(),
	})
	if err != nil {
		return err
	}
	if err := running.registerCommands(); err != nil {
		return err
	}

	running.turn = &firstTurn{
		home:     running.home,
		settings: running.settings,
		model:    running.model,
		stream:   running.stream,
		eventLog: running.eventLog,
		talk:     &conversation{},
		channels: running.channelNamed,
	}
	running.router, err = channel.NewRouter(channel.Routes{
		FindCommand: running.registry.Lookup,
		FindChannel: running.channelNamed,
		// This is the one line the orchestrator changes the day internal/loop
		// lands: loop.New(...).StartTask goes here and firstturn.go goes away.
		StartTask: running.turn.StartTask,
		Skills:    noSkillsYet{},
	})
	return err
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
		Settings:     running.settings,
		Store:        running.eventLog,
		CurrentModel: running.model.Name,
		Channels:     func() []contract.Channel { return []contract.Channel{running.socket} },
	})

	all := append(core.All(),
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
		return running.socket, true
	}
	return nil, false
}

// serve takes connections on the socket and drains the queue beside it, until
// the context is done.
func (running *agent) serve(ctx context.Context) error {
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		running.drain(ctx)
	}()

	err := running.socket.Serve(ctx)
	<-drained
	return err
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
		if _, err := running.router.Route(ctx, queued.Message); err != nil {
			running.note("a message was not answered: " + err.Error())
		}
		if err := running.queue.Done(ctx, queued.Sequence); err != nil {
			running.note("a finished message could not be marked done: " + err.Error())
		}
	}
	return true
}

// close puts down everything the agent opened, in the reverse order it was
// opened, and gives back every problem it met rather than the first.
func (running *agent) close() error {
	problems := []error{}
	if running.socket != nil {
		problems = append(problems, running.socket.Close())
	}
	if running.stream != nil {
		running.stream.Close()
	}
	if running.memories != nil {
		problems = append(problems, running.memories.Close())
	}
	if running.queue != nil {
		problems = append(problems, running.queue.Close())
	}
	if running.secrets != nil {
		problems = append(problems, running.secrets.Close())
	}
	if running.eventLog != nil {
		problems = append(problems, running.eventLog.Close())
	}
	if running.lock != nil {
		problems = append(problems, running.lock.release())
	}
	return errors.Join(problems...)
}

// runLock is the file that stops a second copy of the agent from running on one
// home folder. The lock is held by the operating system on the open file, so a
// copy that is killed outright still lets the next one start.
type runLock struct {
	path string
	file *os.File
}

// takeRunLock takes the lock, refusing when another copy of the agent holds it.
func takeRunLock(path string) (*runLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, contract.SecretFileMode)
	if err != nil {
		return nil, fmt.Errorf("cannot open the lock file %s, so check that the run folder is there and this account can write in it: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("another copy of coeus is already running on this home folder, because something holds the lock file %s, so stop that one first: %w", path, err)
	}

	// The number of the process holding the lock is written into the file, so
	// that a person who finds the file can see which process to stop.
	if err := file.Truncate(0); err == nil {
		_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return &runLock{path: path, file: file}, nil
}

// release gives the lock up. The file itself is left where it is, because
// removing it would take the lock away from whoever opened it next.
func (lock *runLock) release() error {
	unlocked := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	return errors.Join(unlocked, lock.file.Close())
}

// noSkillsYet is the skill store the router asks about every message that is not
// a command. It knows no skills, so every one of them goes to the model, and
// internal/skill takes its place in wave 4.
type noSkillsYet struct{}

// List returns no skills, because there are none.
func (noSkillsYet) List(_ context.Context) ([]contract.SkillSummary, error) { return nil, nil }

// Load says there is nothing to load.
func (noSkillsYet) Load(_ context.Context, name string) (string, error) {
	return "", fmt.Errorf("there are no skills on this machine yet, so nothing named %q can be loaded", name)
}

// Run says there is nothing to run.
func (noSkillsYet) Run(_ context.Context, name string, _ string) (string, error) {
	return "", fmt.Errorf("there are no skills on this machine yet, so nothing named %q can be run", name)
}

// Save says a skill cannot be written yet.
func (noSkillsYet) Save(_ context.Context, name string, _ map[string][]byte) error {
	return fmt.Errorf("skills cannot be saved yet, so the folder for %q was not written", name)
}

// Match never fires, so every message that is not a command goes to the model.
func (noSkillsYet) Match(_ context.Context, _ string) (contract.SkillMatch, error) {
	return contract.SkillMatch{}, nil
}
