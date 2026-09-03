//go:build integration

package shell_test

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/shell"
)

// runningSandbox really runs the command it is given, in its own process group,
// and is what this test uses in place of the fence. The fence itself belongs to
// a package written in the same wave as this one, so this test proves the tool
// against something that really starts a process rather than against the fence.
type runningSandbox struct{}

// Available says the sandbox can run.
func (runningSandbox) Available() error { return nil }

// Run runs the command and returns what it wrote.
func (runningSandbox) Run(ctx context.Context, command contract.SandboxCommand) (contract.SandboxResult, error) {
	running := exec.CommandContext(ctx, command.Program, command.Arguments...)
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	running.Dir = command.WorkingDirectory
	running.Cancel = func() error { return syscall.Kill(-running.Process.Pid, syscall.SIGKILL) }

	written, err := running.Output()
	result := contract.SandboxResult{StandardOutput: written}
	if quit, isQuit := err.(*exec.ExitError); isQuit {
		result.StandardError = quit.Stderr
		result.ExitCode = quit.ExitCode()
		return result, nil
	}
	return result, err
}

func TestARealCommandRunsPollsAndIsKilled(t *testing.T) {
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, runningSandbox{}, testkit.NewFakePermission(contract.RulingAllow), clock)

	quick, err := run(t, tool, map[string]any{"command": "echo alpha"})
	if err != nil {
		t.Fatalf("running a real command failed: %v", err)
	}
	if !strings.Contains(quick.Text, "alpha") {
		t.Errorf("the command said %q, want what it printed", quick.Text)
	}

	go func() { _, _ = run(t, tool, map[string]any{"command": "sleep 30"}) }()
	waitForSleepers(t, clock, 1)
	clock.Advance(shell.YieldAfter)
	waitForRunning(t, tool, "p2")

	polled, err := run(t, tool, map[string]any{"action": "poll", "id": "p2"})
	if err != nil {
		t.Fatalf("polling a real running command failed: %v", err)
	}
	if !strings.Contains(polled.Text, "still running") {
		t.Errorf("polling a real running command said %q", polled.Text)
	}

	killedAt := time.Now()
	if _, err := run(t, tool, map[string]any{"action": "kill", "id": "p2"}); err != nil {
		t.Fatalf("killing a real running command failed: %v", err)
	}
	waitUntilFinished(t, tool, "p2")
	if time.Since(killedAt) > 25*time.Second {
		t.Errorf("the killed command was waited out rather than stopped, which took %s", time.Since(killedAt))
	}
}
