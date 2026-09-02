package testkit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// FakeSandbox runs nothing. It returns the result a test scripted for a command
// prefix, and records every command it was asked to run.
type FakeSandbox struct {
	guard       sync.Mutex
	unavailable error
	roots       []string
	scripted    map[string]contract.SandboxResult
	commands    []contract.SandboxCommand
}

// NewFakeSandbox returns a sandbox that is available and has nothing scripted.
func NewFakeSandbox() *FakeSandbox {
	return &FakeSandbox{scripted: map[string]contract.SandboxResult{}}
}

// SetRoots says which folders the sandbox may run in. A command whose working
// directory is outside every one of them is refused, which is how a test proves
// that the vault, the browser profile, and the agent's home folder stay outside
// the fence. With no roots set, any working directory is allowed.
func (sandbox *FakeSandbox) SetRoots(roots ...string) {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()
	sandbox.roots = roots
}

// SetAvailable makes the sandbox report itself unavailable, the way the real one
// does when bwrap is not installed. Pass nil to make it available again.
func (sandbox *FakeSandbox) SetAvailable(reason error) {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()
	sandbox.unavailable = reason
}

// Script says what to return for any command whose program and arguments begin
// with the prefix, such as "git status".
func (sandbox *FakeSandbox) Script(prefix string, result contract.SandboxResult) {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()
	sandbox.scripted[prefix] = result
}

// Commands is every command the sandbox was asked to run, in order.
func (sandbox *FakeSandbox) Commands() []contract.SandboxCommand {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()
	copied := make([]contract.SandboxCommand, len(sandbox.commands))
	copy(copied, sandbox.commands)
	return copied
}

// Available returns the reason the sandbox cannot run, or nil when it can.
func (sandbox *FakeSandbox) Available() error {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()
	return sandbox.unavailable
}

// Run returns whatever the test scripted for the command's prefix.
func (sandbox *FakeSandbox) Run(_ context.Context, command contract.SandboxCommand) (contract.SandboxResult, error) {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()

	if sandbox.unavailable != nil {
		return contract.SandboxResult{}, fmt.Errorf("the sandbox cannot run this command: %w", sandbox.unavailable)
	}
	sandbox.commands = append(sandbox.commands, command)

	whole := strings.TrimSpace(command.Program + " " + strings.Join(command.Arguments, " "))
	for prefix, result := range sandbox.scripted {
		if strings.HasPrefix(whole, prefix) {
			return result, nil
		}
	}
	return contract.SandboxResult{}, fmt.Errorf("the fake sandbox has nothing scripted for %q, so call Script with that prefix first", whole)
}
