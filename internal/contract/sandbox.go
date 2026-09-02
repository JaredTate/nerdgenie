package contract

import (
	"context"
	"time"
)

// SandboxCommand is one command to run inside the fence.
type SandboxCommand struct {
	// Program is the executable to run.
	Program string
	// Arguments are its arguments, not including the program name.
	Arguments []string
	// WorkingDirectory is where it runs, and must be inside a sandbox root.
	WorkingDirectory string
	// Environment is the environment as "NAME=value" strings. Nothing is
	// inherited that is not listed here, so a secret cannot leak in by accident.
	Environment []string
	// Timeout kills the command and everything it started when it runs long.
	Timeout time.Duration
	// StandardInput is fed to the command, and may be empty.
	StandardInput []byte
}

// SandboxResult is what one sandboxed command produced.
type SandboxResult struct {
	// StandardOutput is what the command wrote to its output.
	StandardOutput []byte
	// StandardError is what it wrote to its error output.
	StandardError []byte
	// ExitCode is the number it reported when it quit.
	ExitCode int
	// TimedOut says the timeout stopped it rather than the command finishing.
	TimedOut bool
}

// Sandbox runs a command walled off from the rest of the machine with bwrap and
// Landlock. The vault, the browser profile, the agent's home folder, and the
// user's SSH keys are always outside it.
type Sandbox interface {
	// Available returns nil when the sandbox can run, and otherwise an error
	// saying what is missing. When it cannot run, the shell tool is turned off.
	Available() error
	// Run runs one command inside the fence.
	Run(ctx context.Context, command SandboxCommand) (SandboxResult, error)
}
