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

	"github.com/JaredTate/coeus/internal/browser"
	"github.com/JaredTate/coeus/internal/channel"
	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/config"
	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/job"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/memory"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/tool"
	"github.com/JaredTate/coeus/internal/vault"
)

// serveSubcommand is the agent itself: the one long-running program that holds
// the queue, the event log, the records, the memory, the jobs, the skills, the
// permissions, the vault, the tools, the turn loop, and the model, and answers
// every screen that attaches to its local socket.
//
// This file, cmd/coeus/wiring.go, and the subcommand table in main.go belong to
// the orchestrator. Each package exports what it owns and everything is put
// together in those files, so that no two workers ever edit the same lines.
var serveSubcommand = subcommand{
	name: "serve",
	help: "Runs the agent and listens on the local socket, which is what the screens attach to.",
	run:  runServe,
}

// The bounds the serving loops run inside, because every loop in Coeus has one.
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

	if found, err := running.guard.Start(ctx); err != nil {
		fmt.Fprintf(problems, "coeus serve: the reliability checks did not finish: %v\n", err)
	} else {
		writeWhatTheGuardFound(output, found)
	}

	fmt.Fprintf(output, "coeus is listening on %s with the model %s. Open a screen with \"coeus\".\n",
		home.SocketFile(), running.model.Name())
	if err := running.guard.Ready(); err != nil {
		fmt.Fprintf(problems, "coeus serve: the service manager was not told the program is up: %v\n", err)
	}
	if err := running.serve(ctx); err != nil {
		fmt.Fprintf(problems, "coeus serve: %v\n", err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// writeWhatTheGuardFound prints the lines that say what the last life of the
// program left behind, and nothing at all when it left nothing.
func writeWhatTheGuardFound(output io.Writer, found reliability.Startup) {
	if found.UncleanExit {
		fmt.Fprintln(output, "the last run of coeus did not exit cleanly, so its state was checked before anything opened it.")
	}
	if found.DatabaseMovedTo != "" {
		fmt.Fprintf(output, "the database was damaged and was moved to %s.\n", found.DatabaseMovedTo)
	}
	if found.RestoredFrom != "" {
		fmt.Fprintf(output, "the database was put back from the archive %s.\n", found.RestoredFrom)
	}
	if found.BreakerTripped {
		fmt.Fprintln(output, "coeus has crashed several times in a row, so it will answer you but start no task until it has been quiet for half an hour.")
	}
	if found.RepliesSentAgain > 0 {
		fmt.Fprintf(output, "%d replies from the last run were written down but never delivered, and were sent again.\n", found.RepliesSentAgain)
	}
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
	jobs     *job.Jobs
	decider  *permission.Decider
	guard    *reliability.Guard

	model     contract.Model
	stream    *channel.Stream
	socket    *channel.Socket
	previews  *waitingPreviews
	builder   *workingcontext.Builder
	fence     contract.Sandbox
	browser   *browser.Browser
	tools     *tool.Registry
	skills    *skill.Store
	skillsBox *skillsBox
	loop      *loop.Loop
	registry  *command.Registry
	router    *channel.Router

	busyGuard sync.Mutex
	busy      bool
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

	running := &agent{home: home, settings: settings, note: note, previews: newWaitingPreviews()}
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

	// SQLite's own quick check runs before anything opens the file, which is
	// the one part of the reliability guard's startup that cannot wait for the
	// guard, because the guard needs the event log the check is guarding.
	if err := reliability.CheckDatabase(ctx, running.home.DatabaseFile()); err != nil {
		running.note("the database did not pass its check, and the guard will move it aside: " + err.Error())
	}
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
	if running.jobs, err = job.Open(ctx, running.home, running.eventLog, now); err != nil {
		return err
	}
	if running.decider, err = permission.New(running.settings, now); err != nil {
		return err
	}
	running.guard, err = reliability.New(reliability.Settings{
		Home:         running.home,
		Clock:        now,
		Store:        running.eventLog,
		Caps:         running.settings.Caps,
		BackupFolder: running.settings.BackupPath,
		Send:         running.sendToTheUser,
	})
	return err
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
	return where.Send(ctx, text)
}

// serve takes connections on the socket, drains the queue beside it, and runs
// the jobs that fall due, until the context is done.
func (running *agent) serve(ctx context.Context) error {
	working := make(chan struct{}, 3)
	for _, work := range []func(context.Context){running.drain, running.runDueJobs, running.feedTheWatchdog} {
		go func() {
			defer func() { working <- struct{}{} }()
			work(ctx)
		}()
	}

	err := running.socket.Serve(ctx)
	<-working
	<-working
	<-working
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
		if _, err := running.router.Route(ctx, queued.Message); err != nil {
			running.note("a message was not answered: " + err.Error())
		}
		if err := running.queue.Done(ctx, queued.Sequence); err != nil {
			running.note("a finished message could not be marked done: " + err.Error())
		}
	}
	return true
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
		if running.loop.Running() != "" {
			continue
		}
		found, err := running.loop.RunNextJobTask(ctx, running.socket)
		if err != nil {
			running.note("a scheduled task did not finish: " + err.Error())
		}
		if !found {
			if err := clock.System().Sleep(ctx, restBetweenJobChecks); err != nil {
				return
			}
		}
	}
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
