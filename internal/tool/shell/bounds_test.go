package shell_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// TestTheNumbersThisToolIsBoundedByAreTheOnesTheDesignNames is finding 38 of
// the wave 6 gate review: every one of these five could be changed in a scratch
// copy and the package stayed green, because each test that used one measured
// against the constant and moved with it. The literal is written here, with the
// sentence that says why it is that number, so a change to any of them has to
// be a change somebody meant to make.
func TestTheNumbersThisToolIsBoundedByAreTheOnesTheDesignNames(t *testing.T) {
	// Ten seconds is what design section 7 names in words: long enough for
	// nearly every command a model runs, short enough that it never sits
	// waiting on a build.
	if shell.YieldAfter != 10*time.Second {
		t.Errorf("the tool hands back an id after %s, and the design says ten seconds", shell.YieldAfter)
	}
	// Sixteen kilobytes is a long command line and a short script, so anything
	// longer belongs in a file the model writes first.
	if shell.MaxCommandBytes != 16*1024 {
		t.Errorf("the longest command is %d bytes, want 16384", shell.MaxCommandBytes)
	}
	// Eight commands at once is more than a model has ever needed and few
	// enough that a task cannot fill the machine with them.
	if shell.MaxRunning != 8 {
		t.Errorf("%d commands may run at once, want 8", shell.MaxRunning)
	}
	// Twenty kilobytes of each stream is about three hundred lines, which is
	// enough of a build log to act on and small enough to leave room in the
	// working context for everything else.
	if shell.MaxStreamBytes != 20*1024 {
		t.Errorf("each stream is cut at %d bytes, want 20480", shell.MaxStreamBytes)
	}
	// Five hundred characters is a paragraph, and the reason for administrator
	// powers is meant to be one line the user can read at a glance.
	if shell.MaxReasonRunes != 500 {
		t.Errorf("the reason may be %d characters, want 500", shell.MaxReasonRunes)
	}
}

// refusingPermission is a permission function that cannot answer at all, which
// is how a test proves that a command with no ruling behind it never runs.
type refusingPermission struct{ *testkit.FakePermission }

// Decide refuses to rule, and says why.
func (refusingPermission) Decide(_ context.Context, _ contract.PermissionRequest) (contract.PermissionDecision, error) {
	return contract.PermissionDecision{}, errors.New("the permission function is not answering, so nothing can be ruled on")
}

func TestACommandWithNoRulingBehindItDoesNotRun(t *testing.T) {
	written := fakeSudo(t)
	tool := escalatingTool(t, refusingPermission{testkit.NewFakePermission(contract.RulingAllow)})

	if _, err := run(t, tool, map[string]any{"command": "apt update", "escalate": true, "reason": "the lists are stale"}); err == nil {
		t.Fatalf("a command ran although nothing could rule on it")
	}
	if _, err := os.Stat(written); !os.IsNotExist(err) {
		t.Errorf("sudo was run although nothing could rule on the command")
	}
}

func TestAPreviewWithNoTextOfItsOwnStillSaysWhatWouldRun(t *testing.T) {
	fakeSudo(t)
	permission := testkit.NewFakePermission(contract.RulingAllow)
	permission.Rule(contract.ToolShell, contract.PermissionDecision{Ruling: contract.RulingAsk, Reason: "on the ask-me-first list"})
	tool := escalatingTool(t, permission)

	output, err := run(t, tool, map[string]any{"command": "apt update", "escalate": true, "reason": "the lists are stale"})
	if err != nil {
		t.Fatalf("a command that must be asked about was treated as a failure: %v", err)
	}
	if !strings.Contains(output.Text, "sudo apt update") {
		t.Errorf("the preview reads %q and does not say what would run", output.Text)
	}
}

func TestATailAndAPollOnACommandStillRunningSayThereIsNothingYet(t *testing.T) {
	sandbox := newSlowSandbox()
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), clock)

	go func() { _, _ = run(t, tool, map[string]any{"command": "sleep 30"}) }()
	waitForSleepers(t, clock, 1)
	clock.Advance(shell.YieldAfter)
	waitForRunning(t, tool, shell.FirstProcessID)

	tailed, err := run(t, tool, map[string]any{"action": "tail", "id": shell.FirstProcessID})
	if err != nil {
		t.Fatalf("tailing a running command failed: %v", err)
	}
	if !strings.Contains(tailed.Text, "nothing back yet") {
		t.Errorf("tailing a running command said %q", tailed.Text)
	}

	if _, err := run(t, tool, map[string]any{"action": "kill", "id": shell.FirstProcessID}); err != nil {
		t.Fatalf("killing the running command failed: %v", err)
	}
	waitUntilFinished(t, tool, shell.FirstProcessID)

	killedAgain, err := run(t, tool, map[string]any{"action": "kill", "id": shell.FirstProcessID})
	if err != nil {
		t.Fatalf("killing a command that had already finished failed: %v", err)
	}
	if !strings.Contains(killedAgain.Text, "already finished") {
		t.Errorf("killing a finished command said %q", killedAgain.Text)
	}
}

func TestAFinishedCommandMakesRoomForANewOne(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("done\n")})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	for at := range shell.MaxRunning * 2 {
		if _, err := run(t, tool, map[string]any{"command": "echo done"}); err != nil {
			t.Fatalf("command number %d was refused although every one before it had finished: %v", at+1, err)
		}
	}
}

func TestAToolWithNoSandboxOrNoClockSaysWhatIsMissing(t *testing.T) {
	home := testkit.NewTempHome(t)
	noSandbox := shell.New(shell.Settings{Clock: testkit.NewFakeClock(theMoment), Home: home})
	if _, err := run(t, noSandbox, map[string]any{"command": "echo alpha"}); err == nil {
		t.Errorf("a command ran with no sandbox wired in")
	}

	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{})
	noClock := shell.New(shell.Settings{Sandbox: sandbox, Home: home, Timeout: time.Minute})
	if _, err := run(t, noClock, map[string]any{"command": "echo alpha"}); err == nil {
		t.Errorf("a command ran with no clock to count the yield on")
	}
}

func TestACommandWithNoTimeoutStillHasOne(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("done\n")})
	home := testkit.NewTempHome(t)
	tool := shell.New(shell.Settings{
		Sandbox:          sandbox,
		Clock:            testkit.NewFakeClock(theMoment),
		Home:             home,
		WorkingDirectory: t.TempDir(),
	})

	if _, err := run(t, tool, map[string]any{"command": "echo done"}); err != nil {
		t.Fatalf("a command with no timeout in the settings failed: %v", err)
	}
}

func TestAReasonLongerThanTheCapIsRefused(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	_, err := run(t, tool, map[string]any{
		"command": "apt update", "escalate": true, "reason": strings.Repeat("y", shell.MaxReasonRunes+1),
	})
	if err == nil {
		t.Errorf("a reason longer than the cap was accepted")
	}
}

func TestTheAskpassHelperFallsBackToThisProgramWhenNoneIsNamed(t *testing.T) {
	home := testkit.NewTempHome(t)
	tool := shell.New(shell.Settings{Home: home})

	path, err := tool.AskpassHelper()
	if err != nil {
		t.Fatalf("cannot write the askpass helper with no program named: %v", err)
	}
	if filepath.Dir(path) != home.RunFolder() {
		t.Errorf("the askpass helper is at %s, want it under the run folder %s", path, home.RunFolder())
	}
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the askpass helper: %v", err)
	}
	if !strings.Contains(string(held), shell.AskpassSubcommand) {
		t.Errorf("the askpass helper reads %q and does not run the askpass subcommand", string(held))
	}
}

func TestACommandThatCannotBeStartedAtAllSaysSo(t *testing.T) {
	written := fakeSudo(t)
	if err := os.Remove(filepath.Dir(written)); err == nil {
		t.Skip("the folder holding the fake sudo could be removed, so this machine will not show the failure")
	}

	home := testkit.NewTempHome(t)
	if err := os.Chmod(home.RunFolder(), 0o500); err != nil {
		t.Fatalf("cannot shut the run folder: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home.RunFolder(), contract.HomeFolderMode) })

	tool := shell.New(shell.Settings{
		Sandbox:          testkit.NewFakeSandbox(),
		Permission:       testkit.NewFakePermission(contract.RulingAllow),
		Clock:            testkit.NewFakeClock(theMoment),
		Home:             home,
		NerdGenieProgram:     "/bin/true",
		WorkingDirectory: t.TempDir(),
		Timeout:          time.Minute,
	})

	output, err := run(t, tool, map[string]any{"command": "apt update", "escalate": true, "reason": "the lists are stale"})
	if err != nil {
		t.Fatalf("a command that could not be started was treated as a failure of the tool: %v", err)
	}
	if !strings.Contains(output.Text, "could not be run") {
		t.Errorf("a command that could not be started said %q", output.Text)
	}
}
