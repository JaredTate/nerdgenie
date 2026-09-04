// Running the command straight on the machine, in its own process group, with
// the group stopped by its own number when the run ends, is how Hermes runs a
// command on the host in execute_code at
// ~/Code/hermes-agent/tools/code_execution_tool.py, and how OpenClaw's exec tool
// runs one at ~/Code/openclaw/src/agents/bash-tools.exec-runtime.ts, stopping
// the tree through ~/Code/openclaw/src/process/kill-tree.ts. Neither boxes the
// command in; the gate is the approval in front of it, and that is the shape
// kept here.

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A Direct runner is a contract.Sandbox, and this line is what says so at build
// time.
var _ contract.Sandbox = (*Direct)(nil)

// Direct runs a command straight on this machine as the user, with no fence
// around it, which is what the sandbox setting "off" asks for and what the
// agent does unless the configuration asks for the fence. Hermes and OpenClaw
// both run on the host this way, and a person who wants their agent to build in
// their own repository, reach their own tools, and leave the results where they
// look for them wants the same.
//
// Turning the fence off does not turn the bounds off. The command still runs in
// its own process group, still ends at its timeout or when the run is cancelled,
// still has the whole group stopped when it does, and still has each of its two
// output streams capped, all of it the same code the fence runs. What is gone is
// the box: the command reaches the whole machine, so the gate that is left is
// the permission function and the user's ask-me-first list, and the file tools
// keep the vault, the browser profile, the backups, and the user's SSH keys out
// of reach through tool.NewOpenPathCheck.
type Direct struct {
	outputCap int
}

// NewDirect returns the runner the agent uses when the sandbox is off. It takes
// the same settings the fence does and reads only the output cap from them,
// because the roots and the paths that stay outside are the file tools' business
// once there is no fence to build. Nothing here can fail, so there is no error
// to give back: a machine that can run the agent can run a command.
func NewDirect(settings Settings) *Direct {
	outputCap := settings.OutputCap
	if outputCap <= 0 {
		outputCap = contract.DefaultConfig().Caps.ToolOutputBytes
	}
	return &Direct{outputCap: outputCap}
}

// Available says the direct runner can run, always. It needs no bwrap, no
// Landlock, and no user namespace, which is the whole reason a person turns the
// fence off: on a machine where the fence cannot be built, the shell tool still
// works.
func (direct *Direct) Available() error { return nil }

// Run runs one command on this machine and returns what it printed, what it
// exited with, and whether its time ran out.
//
// The command runs in its own process group, so that when the run is cancelled
// or the command's own timeout ends first, the whole of that group is asked to
// stop and then killed by the number this program was given for it, and nothing
// the command started is left behind. No name is ever matched against, because a
// pattern would catch other people's processes as well as this one's.
func (direct *Direct) Run(ctx context.Context, command contract.SandboxCommand) (contract.SandboxResult, error) {
	if command.Program == "" {
		return contract.SandboxResult{}, errors.New("this call names no program to run, so give the command the program to start")
	}
	runContext := ctx
	if command.Timeout > 0 {
		var stopWaiting context.CancelFunc
		runContext, stopWaiting = context.WithTimeout(ctx, command.Timeout)
		defer stopWaiting()
	}

	standardOutput := &cappedWriter{limit: direct.outputCap}
	standardError := &cappedWriter{limit: direct.outputCap}
	started := exec.Command(command.Program, command.Arguments...)
	started.Dir = command.WorkingDirectory
	started.Env = append(os.Environ(), command.Environment...)
	started.Stdin = bytes.NewReader(command.StandardInput)
	started.Stdout = standardOutput
	started.Stderr = standardError
	started.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := started.Start(); err != nil {
		return contract.SandboxResult{}, fmt.Errorf(
			"the command %q could not be started on this machine, so check that it is installed and can be run: %w", command.Program, err)
	}
	return directResult(runContext, started, standardOutput, standardError)
}

// directResult waits for the command and gathers what it did, stopping the whole
// process group when the run ends first. It is separate from Run so that Run
// reads as the one command it builds.
func directResult(ctx context.Context, started *exec.Cmd, standardOutput *cappedWriter, standardError *cappedWriter) (contract.SandboxResult, error) {
	timedOut, reaped := waitOrKill(ctx, started)
	if !reaped {
		return contract.SandboxResult{TimedOut: true},
			errors.New("the command did not stop after it was killed, so something on this machine is holding it open")
	}
	return contract.SandboxResult{
		StandardOutput: standardOutput.Bytes(),
		StandardError:  standardError.Bytes(),
		ExitCode:       exitCodeOf(started),
		TimedOut:       timedOut,
	}, nil
}
