package testkit

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

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

// ErrSandboxTimedOut means the command's time was up before it could run, which
// is what a real sandbox reports when it kills a command on its deadline.
var ErrSandboxTimedOut = errors.New("the command's time was up before it finished, so give it longer or run less at once")

// Run returns whatever the test scripted for the command's prefix, after
// checking everything the contract says a sandbox checks: the time left, the
// roots, and what is in the environment.
func (sandbox *FakeSandbox) Run(ctx context.Context, command contract.SandboxCommand) (contract.SandboxResult, error) {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()

	if sandbox.unavailable != nil {
		return contract.SandboxResult{}, fmt.Errorf("the sandbox cannot run this command: %w", sandbox.unavailable)
	}
	sandbox.commands = append(sandbox.commands, command)

	if err := timeLeft(ctx, command.Timeout); err != nil {
		return contract.SandboxResult{TimedOut: true}, err
	}
	if err := sandbox.insideARoot(command.WorkingDirectory); err != nil {
		return contract.SandboxResult{}, err
	}
	if err := nothingSecretIn(command.Environment); err != nil {
		return contract.SandboxResult{}, err
	}

	whole := strings.TrimSpace(command.Program + " " + strings.Join(command.Arguments, " "))
	if result, scripted := sandbox.longestMatch(whole); scripted {
		return result, nil
	}
	return contract.SandboxResult{}, fmt.Errorf("the fake sandbox has nothing scripted for %q, so call Script with that prefix first", whole)
}

// longestMatch returns the result scripted for the longest prefix the command
// begins with. The keys are sorted, so that a command matching both "git" and
// "git status" always gets the one the test meant.
func (sandbox *FakeSandbox) longestMatch(whole string) (contract.SandboxResult, bool) {
	prefixes := slices.Sorted(maps.Keys(sandbox.scripted))
	slices.Reverse(prefixes)
	for _, prefix := range prefixes {
		if strings.HasPrefix(whole, prefix) {
			return sandbox.scripted[prefix], true
		}
	}
	return contract.SandboxResult{}, false
}

// timeLeft says whether there is time to run the command at all: the caller's
// context may already be done, and the command's own timeout may already have
// passed.
func timeLeft(ctx context.Context, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w, because the caller gave up first: %w", ErrSandboxTimedOut, err)
	}
	if timeout <= 0 {
		return nil
	}
	own, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := own.Err(); err != nil {
		return fmt.Errorf("%w: the timeout of %s had already passed", ErrSandboxTimedOut, timeout)
	}
	return nil
}

// insideARoot refuses a working directory outside every root the test set. With
// no roots set, anything is allowed, because a test that says nothing about the
// fence is not testing the fence.
func (sandbox *FakeSandbox) insideARoot(directory string) error {
	if len(sandbox.roots) == 0 || directory == "" {
		return nil
	}
	for _, root := range sandbox.roots {
		if directory == root || strings.HasPrefix(directory, strings.TrimSuffix(root, "/")+"/") {
			return nil
		}
	}
	return fmt.Errorf("the working directory %q is outside every sandbox root (%s), so run the command somewhere inside the fence",
		directory, strings.Join(sandbox.roots, ", "))
}

// nothingSecretIn refuses an environment carrying a secret reference. The vault
// resolves a reference for the harness alone, and nothing secret ever crosses
// the fence.
func nothingSecretIn(environment []string) error {
	for _, setting := range environment {
		name, value, _ := strings.Cut(setting, "=")
		if strings.Contains(value, contract.SecretReferencePrefix) {
			return fmt.Errorf("the setting %s carries a secret reference, so resolve it outside the fence and pass nothing secret in", name)
		}
	}
	return nil
}
