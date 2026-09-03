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

// TestAFailingCommandPipedIntoAnotherReportsTheFailingCode is what brief 6.6
// carried over from the human trial: the model wrote "node --test tests/ 2>&1 |
// tail -40", the tests failed, and the tool said "finished with exit code 0",
// because the code a pipe reports is the code of its last command. The command
// runs through bash with pipefail set, so the failing command's own code comes
// back. This test really runs the command, because the point of it is what the
// shell on this machine does rather than what the tool asked for.
func TestAFailingCommandPipedIntoAnotherReportsTheFailingCode(t *testing.T) {
	tool := newTool(t, runningSandbox{}, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "exit 7 | cat"})
	if err != nil {
		t.Fatalf("running a failing command in a pipe failed: %v", err)
	}
	if !strings.Contains(output.Text, "exit code 7") {
		t.Errorf("a command that quit with 7 into a pipe said %q, and the model would read that as a run that passed", output.Text)
	}
}

// TestAPipeClosedEarlyByHeadIsSaidToBeNormal covers what pipefail brought with
// it: "find . | head -5" ends with code 141 because head closed the pipe while
// find was still writing, and a model that reads 141 as a failure retries a
// command that worked. The result says so in plain words.
func TestAPipeClosedEarlyByHeadIsSaidToBeNormal(t *testing.T) {
	tool := newTool(t, runningSandbox{}, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))
	output, err := run(t, tool, map[string]any{"command": "yes | head -1"})
	if err != nil {
		t.Fatalf("running the pipe failed: %v", err)
	}
	if !strings.Contains(output.Text, "141") || !strings.Contains(output.Text, "normal") {
		t.Errorf("the result does not say a pipe closed early is normal: %q", output.Text)
	}
}
