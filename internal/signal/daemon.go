// The way signal-cli is started and watched was borrowed from OpenClaw's Signal
// daemon at ~/Code/openclaw/extensions/signal/src/daemon.ts, which is where the
// argument order, the --no-receive-stdout flag, the polled health check and the
// term-then-kill shutdown come from, and from its lifecycle file at
// ~/Code/openclaw/extensions/signal/src/daemon-lifecycle.ts. The Go here is
// written fresh, and unlike the reference it puts the child in its own process
// group and kills that group by its exact identifier, never by name.

package signal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// DaemonStartupTimeout is how long signal-cli has to answer its health check
	// after it is started before the supervisor gives up on it.
	DaemonStartupTimeout = 30 * time.Second
	// daemonHealthPoll is how often the health check is tried while waiting for
	// signal-cli to come up.
	daemonHealthPoll = 250 * time.Millisecond
	// daemonStopGrace is how long a polite request to stop is given before the
	// process group is killed outright.
	daemonStopGrace = 2 * time.Second
	// MaxDaemonStarts is how many times in a row signal-cli may be started
	// before the supervisor decides it will not stay up and stops trying.
	MaxDaemonStarts = 20
)

// DaemonOptions is everything the supervisor needs to run signal-cli.
type DaemonOptions struct {
	// Program is the signal-cli to run, either a path or a name on the PATH.
	Program string
	// Account is the phone number the daemon is linked to.
	Account string
	// Address is the host and port the daemon serves its HTTP interface on.
	Address string
	// Clock is where every wait is measured.
	Clock contract.Clock
	// Healthy says whether the daemon is answering. The channel points this at
	// the client's health check; a test points it wherever it likes.
	Healthy func(context.Context) bool
}

// Daemon starts signal-cli as a child in its own process group and keeps it
// running. When it dies, the supervisor waits and starts it again, with the wait
// growing the way the stream's does.
type Daemon struct {
	options DaemonOptions

	guard   sync.Mutex
	process *os.Process
	stopped bool
	starts  atomic.Int64
}

// NewDaemon builds a supervisor. It runs nothing until Start is called.
func NewDaemon(options DaemonOptions) (*Daemon, error) {
	switch {
	case options.Program == "":
		return nil, errors.New("the Signal daemon has no program to run, so say where signal-cli is")
	case options.Account == "":
		return nil, errors.New("the Signal daemon has no account, so run \"nerdgenie signal link\" first")
	case options.Address == "":
		return nil, errors.New("the Signal daemon has no address to serve on, so pass a host and port")
	case options.Clock == nil:
		return nil, errors.New("the Signal daemon has no clock, and every wait it makes is measured on one")
	}
	if options.Healthy == nil {
		options.Healthy = func(context.Context) bool { return true }
	}
	return &Daemon{options: options}, nil
}

// Start runs signal-cli, waits for it to answer its health check, and then
// watches it until Stop is called or the context is cancelled. A daemon that
// never becomes healthy is killed and the error says so.
func (daemon *Daemon) Start(ctx context.Context) error {
	if err := daemon.launch(); err != nil {
		return err
	}
	if err := daemon.waitUntilHealthy(ctx); err != nil {
		_ = daemon.Stop()
		return err
	}
	go daemon.watch(ctx)
	return nil
}

// Stop asks signal-cli to stop and kills its whole process group if it will not.
// The group is named by the child's own process identifier, never by a pattern,
// because a pattern matches whatever else happens to look like it.
func (daemon *Daemon) Stop() error {
	daemon.guard.Lock()
	running := daemon.process
	daemon.stopped = true
	daemon.process = nil
	daemon.guard.Unlock()

	if running == nil {
		return nil
	}
	if err := syscall.Kill(-running.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("cannot ask signal-cli (process %d) to stop: %w", running.Pid, err)
	}
	gone := make(chan struct{})
	go func() {
		_, _ = running.Wait()
		close(gone)
	}()
	select {
	case <-gone:
	case <-time.After(daemonStopGrace):
		_ = syscall.Kill(-running.Pid, syscall.SIGKILL)
	}
	return nil
}

// ProcessID is the identifier of the running signal-cli, or zero when none is
// running. It is what a test checks and what a log line names.
func (daemon *Daemon) ProcessID() int {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	if daemon.process == nil {
		return 0
	}
	return daemon.process.Pid
}

// Starts is how many times signal-cli has been started, which is how a caller or
// a test can see that it died and came back.
func (daemon *Daemon) Starts() int {
	return int(daemon.starts.Load())
}

// Arguments is the command line signal-cli is given. The account comes before
// the subcommand because it is a setting for the whole program, and the address
// comes after it because it is a setting for the daemon.
func (daemon *Daemon) Arguments() []string {
	return []string{
		"-a", daemon.options.Account,
		"daemon",
		"--http", daemon.options.Address,
		"--no-receive-stdout",
	}
}

// launch starts one signal-cli in a process group of its own.
func (daemon *Daemon) launch() error {
	running := exec.Command(daemon.options.Program, daemon.Arguments()...)
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := running.Start(); err != nil {
		return fmt.Errorf("cannot run signal-cli at %s, so install it and check the path in the configuration: %w", daemon.options.Program, err)
	}

	daemon.guard.Lock()
	daemon.process = running.Process
	daemon.stopped = false
	daemon.guard.Unlock()
	daemon.starts.Add(1)
	return nil
}

// waitUntilHealthy polls the health check until signal-cli answers or the
// startup time runs out.
func (daemon *Daemon) waitUntilHealthy(ctx context.Context) error {
	began := daemon.options.Clock.Now()
	for {
		if daemon.options.Healthy(ctx) {
			return nil
		}
		if daemon.options.Clock.Now().Sub(began) >= DaemonStartupTimeout {
			return fmt.Errorf("signal-cli did not answer its health check at %s within %v, so look at what it printed", daemon.options.Address, DaemonStartupTimeout)
		}
		if err := daemon.options.Clock.Sleep(ctx, daemonHealthPoll); err != nil {
			return fmt.Errorf("gave up waiting for signal-cli to answer its health check: %w", err)
		}
	}
}

// watch waits for signal-cli to die and starts it again after a wait that grows,
// until it is stopped, the context is cancelled, or it has been started too many
// times in a row to believe it will ever stay up.
func (daemon *Daemon) watch(ctx context.Context) {
	wait := ReconnectMinimumWait
	for daemon.Starts() < MaxDaemonStarts {
		running := daemon.currentProcess()
		if running == nil {
			return
		}
		_, _ = running.Wait()
		if daemon.wasStopped() || ctx.Err() != nil {
			return
		}
		if err := daemon.options.Clock.Sleep(ctx, wait); err != nil {
			return
		}
		wait = nextWait(wait)
		if err := daemon.launch(); err != nil {
			return
		}
		if err := daemon.waitUntilHealthy(ctx); err != nil {
			_ = daemon.Stop()
			return
		}
	}
}

// currentProcess is the running signal-cli, or nil when there is none.
func (daemon *Daemon) currentProcess() *os.Process {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	return daemon.process
}

// wasStopped says whether somebody asked the daemon to stop.
func (daemon *Daemon) wasStopped() bool {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	return daemon.stopped
}
