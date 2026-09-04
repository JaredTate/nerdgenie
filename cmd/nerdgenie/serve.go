package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/JaredTate/nerdgenie/internal/browser"
	"github.com/JaredTate/nerdgenie/internal/channel"
	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/desktop"
	"github.com/JaredTate/nerdgenie/internal/job"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/memory"
	"github.com/JaredTate/nerdgenie/internal/permission"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/replay"
	signalchannel "github.com/JaredTate/nerdgenie/internal/signal"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/tool"
	"github.com/JaredTate/nerdgenie/internal/update"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// serveSubcommand is the agent itself: the one long-running program that holds
// the queue, the event log, the records, the memory, the jobs, the skills, the
// permissions, the vault, the tools, the turn loop, and the model, and answers
// every screen that attaches to its local socket.
//
// This file, cmd/nerdgenie/wiring.go, and the subcommand table in main.go belong to
// the orchestrator. Each package exports what it owns and everything is put
// together in those files, so that no two workers ever edit the same lines.
var serveSubcommand = subcommand{
	name: "serve",
	help: "Runs the agent and listens on the local socket, which is what the screens attach to.",
	run:  runServe,
}

// The bounds the serving loops run inside, because every loop in Nerd Genie has one.
const (
	// maxMessagesInOnePass is how many queued messages one pass of the drainer
	// takes before it goes round again.
	maxMessagesInOnePass = 64
	// restBetweenJobChecks is how long the job driver waits after finding
	// nothing due, so that a job store with work already past its moment cannot
	// spin.
	restBetweenJobChecks = time.Second
)

// readyName is the slash command a health check sends. It is answered on the
// socket by the same path a person's message travels, so an answer proves the
// whole way in and out is working, not merely that a socket file exists.
const readyName = "readyz"

// runServe opens everything the agent owns, says where it is listening, and
// serves until the terminal interrupt or the service manager stops it.
func runServe(arguments []string, output io.Writer, problems io.Writer) int {
	if len(arguments) > 0 {
		fmt.Fprintf(problems, "nerdgenie serve takes no arguments, and it was given %q. Run \"nerdgenie help\" for the list.\n", strings.Join(arguments, " "))
		return contract.ExitUsage
	}
	home, err := config.HomeFolder()
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie serve: %v\n", err)
		return contract.ExitBadConfiguration
	}

	fmt.Fprintln(output, whichBinaryIsServing())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	running, err := openAgent(ctx, home, func(line string) { fmt.Fprintln(output, line) })
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie serve: %v\n", err)
		return exitCodeFor(err)
	}
	defer func() {
		if err := running.close(); err != nil {
			fmt.Fprintf(problems, "nerdgenie serve: stopping was not clean: %v\n", err)
		}
	}()

	if found, err := running.guard.Start(ctx); err != nil {
		fmt.Fprintf(problems, "nerdgenie serve: the reliability checks did not finish: %v\n", err)
	} else {
		writeWhatTheGuardFound(output, found)
	}

	fmt.Fprintf(output, "nerdgenie is listening on %s with the model %s. Open a screen with \"nerdgenie\".\n",
		home.SocketFile(), running.model.Name())
	if err := running.guard.Ready(); err != nil {
		fmt.Fprintf(problems, "nerdgenie serve: the service manager was not told the program is up: %v\n", err)
	}
	if nightlyJob, err := running.nightly.Register(ctx); err != nil {
		fmt.Fprintf(problems, "nerdgenie serve: the nightly self-check was not put on the job list: %v\n", err)
	} else if err := running.jobs.KeepRunningWhenItsTasksFail(ctx, nightlyJob); err != nil {
		// The self-check is the one job that must outlive its own failures: it
		// exists to say when something is wrong, and a run of bad nights is
		// exactly when it is needed most.
		fmt.Fprintf(problems, "nerdgenie serve: the nightly self-check will stop itself after a run of bad nights: %v\n", err)
	}
	if err := running.serve(ctx); err != nil {
		fmt.Fprintf(problems, "nerdgenie serve: %v\n", err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// writeWhatTheGuardFound prints the lines that say what the last life of the
// program left behind, and nothing at all when it left nothing.
func writeWhatTheGuardFound(output io.Writer, found reliability.Startup) {
	if found.UncleanExit {
		fmt.Fprintln(output, "the last run of nerdgenie did not exit cleanly, so its state was checked before anything opened it.")
	}
	if found.DatabaseMovedTo != "" {
		fmt.Fprintf(output, "the database was damaged and was moved to %s.\n", found.DatabaseMovedTo)
	}
	if found.RestoredFrom != "" {
		fmt.Fprintf(output, "the database was put back from the archive %s.\n", found.RestoredFrom)
	}
	if found.BreakerTripped {
		fmt.Fprintln(output, "nerdgenie has crashed several times in a row, so it will answer you but start no task until it has been quiet for half an hour.")
	}
	if found.RepliesSentAgain > 0 {
		fmt.Fprintf(output, "%d replies from the last run were written down but never delivered, and were sent again.\n", found.RepliesSentAgain)
	}
}

// whichBinaryIsServing is the first line the log carries: the whole path of the
// program that is running and the commit it was built from. A person reading a
// log has to know which build they are looking at, and a worktree's binary and
// the installed one look the same from the outside.
func whichBinaryIsServing() string {
	program, err := os.Executable()
	if err != nil {
		program = "a program whose path could not be read"
	}
	return fmt.Sprintf("nerdgenie %s (commit %s) serving from %s", version, commit, program)
}

// exitCodeFor turns what went wrong at startup into the code the service unit
// reads: a configuration a restart cannot fix stops the service and waits for a
// person, and everything else may be tried again.
func exitCodeFor(err error) int {
	problem := config.Problem{}
	if errors.As(err, &problem) {
		return contract.ExitBadConfiguration
	}
	// A database from a newer Nerd Genie is not something a restart will fix, and a
	// service that tried every five seconds would fill the log and change
	// nothing.
	if errors.Is(err, update.ErrDatabaseFromANewerNerdGenie) {
		return contract.ExitBadConfiguration
	}
	return contract.ExitFailure
}

// agent is one running Nerd Genie and everything it owns. Every field is built by one
// package, and this struct is the only place they meet.
type agent struct {
	home     contract.Home
	settings contract.Config
	sayLine  func(line string)

	lock     *runLock
	eventLog *log.Log
	events   contract.Store
	queue    *channel.Queue
	secrets  *vault.Vault
	memories *memory.Memory
	jobs     *job.Jobs
	decider  *permission.Decider
	guard    *reliability.Guard

	model    contract.Model
	stream   *channel.Stream
	socket   *channel.Socket
	previews *waitingPreviews
	builder  *perTaskContext
	fence    contract.Sandbox
	browser  *browser.Browser
	desktop  *desktop.Desktop
	// userTools are the user's own programs, asked what they are once at start
	// and shared by every task's registry.
	userTools []contract.Tool
	tools     *tool.Registry
	skills    *skill.Store
	skillsBox *skillsBox
	loop      *loop.Loop
	nightly   *replay.Nightly
	registry  *command.Registry
	router    *channel.Router

	watched *watchedModel

	signal *signalchannel.Channel

	busyGuard  sync.Mutex
	busy       bool
	modelInUse string
	recordLine string
	toolLine   string
	holding    int64
	handedOver bool
}

// openAgent makes the home folder if it is not there, reads the configuration,
// and opens everything in the order the packages need: the stores first, because
// the event log owns the database file's identity, and then the model, the
// socket, the tools, the loop, the commands, and the router.
func openAgent(ctx context.Context, home contract.Home, note func(line string)) (*agent, error) {
	if err := makeHomeFolders(home); err != nil {
		return nil, err
	}
	settings, err := config.Load(home)
	if err != nil {
		return nil, err
	}

	running := &agent{home: home, settings: settings, sayLine: note, previews: newWaitingPreviews()}
	if err := running.openTheStores(ctx); err != nil {
		return nil, errors.Join(err, running.close())
	}
	if err := running.openTheFront(ctx); err != nil {
		return nil, errors.Join(err, running.close())
	}
	return running, nil
}

// note writes one line about what the agent is doing, through the vault's
// redactor once there is one.
//
// A note from a provider, a worker, or the tool registry can carry a key, and a
// log is read by whoever can read the machine. The vault is opened a few lines
// into startup, and nothing that has seen a secret has run before then.
func (running *agent) note(line string) {
	if running.secrets != nil {
		line = running.secrets.Redact(line)
	}
	if running.sayLine != nil {
		running.sayLine(line)
	}
}

// makeHomeFolders makes every folder of the layout that is not there yet, so
// that "nerdgenie serve" comes up on a machine where nobody has run "nerdgenie init".
func makeHomeFolders(home contract.Home) error {
	for _, folder := range home.Folders() {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			return fmt.Errorf("cannot make the folder %s that Nerd Genie keeps its work in, so check who owns %s: %w", folder, home.Root, err)
		}
	}
	return nil
}

// openTheStores takes the run lock and opens everything that keeps something
// between runs. The event log is opened before the queue, the memory, and the
// jobs, because the log owns the database file's identity and a file holding
// only another package's table is not one internal/log will open.
func (running *agent) openTheStores(ctx context.Context) error {
	now := clock.System()
	lock, err := takeRunLock(running.home.LockFile())
	if err != nil {
		return err
	}
	running.lock = lock

	databaseFile, err := running.theDatabaseToOpen(ctx, now)
	if err != nil {
		return err
	}
	if running.eventLog, err = log.Open(ctx, databaseFile); err != nil {
		return err
	}
	// Everything downstream writes through this rather than through the log
	// itself, so that an event written by something with no clock of its own
	// still carries the moment it happened.
	running.events = timedEvents(running.eventLog, now)
	running.queue, err = channel.OpenQueue(ctx, databaseFile, running.settings.Caps.QueuedMessages)
	if err != nil {
		return err
	}
	if running.secrets, err = vault.Open(running.home, now); err != nil {
		return err
	}
	running.memories, err = memory.Open(ctx, running.home, running.events, now, running.settings.MemoryCaps)
	if err != nil {
		return err
	}
	if running.jobs, err = job.Open(ctx, running.home, running.events, now); err != nil {
		return err
	}
	if running.decider, err = permission.New(running.settings, now); err != nil {
		return err
	}
	running.guard, err = reliability.New(reliability.Settings{
		Home:         running.home,
		Clock:        now,
		Store:        running.events,
		Caps:         running.settings.Caps,
		BackupFolder: running.settings.BackupPath,
		Send:         running.sendToTheUser,
	})
	if err != nil {
		return err
	}
	// The jobs ask the guard before they start anything and tell the user in
	// their own words when one stops, which is how a job that keeps failing is
	// something a person hears about rather than something that goes quiet.
	running.jobs.OnlyStartWorkWhen(running.guard.MayStartTask)
	running.jobs.TellTheUser(running.sendToTheUserHere)
	return nil
}

// theDatabaseToOpen makes the one database file good before anything opens it,
// and says which path to open.
//
// The whole recovery runs first: the check, the moving aside of a database that
// failed it, and putting the newest archive back. The guard will not start until
// it has run, because a log opened on a damaged file is a program that cannot
// say what happened to it. Then the schema is looked at, because an older binary
// must never open a database a newer Nerd Genie has migrated: it would read the file
// wrong, and reading a record wrong is worse than not reading it at all.
func (running *agent) theDatabaseToOpen(ctx context.Context, now contract.Clock) (string, error) {
	databaseFile, err := reliability.PrepareDatabase(ctx, reliability.RecoverySettings{
		Home:         running.home,
		Clock:        now,
		BackupFolder: running.settings.BackupPath,
	})
	if err != nil {
		return "", err
	}
	if err := update.CheckSchema(ctx, databaseFile); err != nil {
		return "", err
	}
	return databaseFile, nil
}

// sendToTheUser is how the reliability guard reaches whoever is listening: the
// channel it names, or the one the user is normally talked to on.
func (running *agent) sendToTheUser(ctx context.Context, named string, text string) error {
	if named == "" {
		named = contract.TerminalChannelName
	}
	where, found := running.channelNamed(named)
	if !found {
		return fmt.Errorf("there is no channel named %q to tell the user on, so the message was not sent", named)
	}
	// A send with nobody attached is not a delivery. The socket answers with no
	// error when no screen is listening, and the delivery ledger reads that as
	// the reply having reached the user, so every reply the last run never
	// delivered would be thrown away at startup, before a screen has had a
	// chance to attach.
	if named == contract.TerminalChannelName && running.socket.Attached() == 0 {
		return fmt.Errorf("no screen is attached, so %q reached nobody and is still owed to the user", shortened(text))
	}
	return where.Send(ctx, text)
}

// shortened is the first few words of a message, for an error that names what
// could not be sent without repeating the whole of it.
func shortened(text string) string {
	if len(text) <= 60 {
		return text
	}
	return text[:57] + "..."
}

// close puts down everything the agent opened, in the reverse order it was
// opened, and gives back every problem it met rather than the first.
func (running *agent) close() error {
	problems := []error{}
	if running.guard != nil {
		problems = append(problems, running.guard.Stop())
	}
	if running.browser != nil {
		problems = append(problems, running.browser.Close())
	}
	if running.desktop != nil {
		problems = append(problems, running.desktop.Close())
	}
	if running.socket != nil {
		problems = append(problems, running.socket.Close())
	}
	if running.stream != nil {
		running.stream.Close()
	}
	if running.jobs != nil {
		problems = append(problems, running.jobs.Close())
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
