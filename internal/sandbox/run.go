package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// A Fence is a contract.Sandbox, and this line is what says so at build time.
var _ contract.Sandbox = (*Fence)(nil)

// gracePeriod is how long a command has to stop after it is asked politely,
// before it is killed outright.
const gracePeriod = 2 * time.Second

// signalExitBase is what a shell adds to a signal number to report a command
// that was stopped rather than one that finished, and this package reports the
// same number for the same reason.
const signalExitBase = 128

// Run runs one command inside the fence and returns what it printed, what it
// exited with, and whether its time ran out.
//
// The command runs in its own process group. When the context or the command's
// own timeout ends first, the whole of that group is asked to stop and then
// killed, so that nothing the command started is left behind.
func (fence *Fence) Run(ctx context.Context, command contract.SandboxCommand) (contract.SandboxResult, error) {
	if err := fence.Available(); err != nil {
		return contract.SandboxResult{}, fmt.Errorf("the sandbox cannot run this command: %w", err)
	}
	plan, err := fence.planFor(command)
	if err != nil {
		return contract.SandboxResult{}, err
	}

	scratchHome := filepath.Join(fence.roots[0], scratchHomeName)
	if err := os.MkdirAll(scratchHome, contract.HomeFolderMode); err != nil {
		return contract.SandboxResult{}, fmt.Errorf("the sandbox cannot make the scratch home folder %q, so check that the first root can be written to: %w", scratchHome, err)
	}
	return fence.start(ctx, plan, command)
}

// start builds the process, runs it, and gathers the result. It is separate from
// Run so that Run reads as the four things it checks before anything starts.
func (fence *Fence) start(ctx context.Context, plan fencePlan, command contract.SandboxCommand) (contract.SandboxResult, error) {
	runContext := ctx
	if command.Timeout > 0 {
		var stopWaiting context.CancelFunc
		runContext, stopWaiting = context.WithTimeout(ctx, command.Timeout)
		defer stopWaiting()
	}

	standardOutput := &cappedWriter{limit: fence.outputCap}
	standardError := &cappedWriter{limit: fence.outputCap}
	started := exec.Command(bubblewrapProgram, buildArguments(plan)...)
	started.Env = []string{}
	started.Stdin = bytes.NewReader(command.StandardInput)
	started.Stdout = standardOutput
	started.Stderr = standardError
	started.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := started.Start(); err != nil {
		return contract.SandboxResult{}, fmt.Errorf("the sandbox cannot start %s, so check that it is installed and can run: %w", bubblewrapProgram, err)
	}

	timedOut, reaped := waitOrKill(runContext, started)
	if !reaped {
		return contract.SandboxResult{TimedOut: true},
			errors.New("the sandboxed command did not stop after it was killed, so something outside the sandbox is holding it open")
	}
	return contract.SandboxResult{
		StandardOutput: standardOutput.Bytes(),
		StandardError:  standardError.Bytes(),
		ExitCode:       exitCodeOf(started),
		TimedOut:       timedOut,
	}, nil
}

// waitOrKill waits for the command, and stops the whole process group when the
// context ends first. It says whether the context ended it and whether the
// command was reaped, because the result may only be read once it was.
func waitOrKill(ctx context.Context, started *exec.Cmd) (bool, bool) {
	finished := make(chan error, 1)
	go func() { finished <- started.Wait() }()

	select {
	case <-finished:
		return false, true
	case <-ctx.Done():
	}
	return true, stopProcessGroup(started.Process.Pid, finished)
}

// stopProcessGroup asks the group this program started to stop, and kills it two
// seconds later when it has not. Only the group started here is signalled, and
// it is named by its own number, so nothing else on the machine is touched.
func stopProcessGroup(processID int, finished chan error) bool {
	group := -processID
	_ = syscall.Kill(group, syscall.SIGTERM)
	select {
	case <-finished:
		return true
	case <-time.After(gracePeriod):
	}

	_ = syscall.Kill(group, syscall.SIGKILL)
	select {
	case <-finished:
		return true
	case <-time.After(gracePeriod):
		return false
	}
}

// exitCodeOf is the number the command reported, or a hundred and twenty-eight
// plus the signal that stopped it, which is what a shell reports for the same
// thing.
func exitCodeOf(started *exec.Cmd) int {
	state := started.ProcessState
	if state == nil {
		return contract.ExitFailure
	}
	if status, isWaitStatus := state.Sys().(syscall.WaitStatus); isWaitStatus && status.Signaled() {
		return signalExitBase + int(status.Signal())
	}
	return state.ExitCode()
}
