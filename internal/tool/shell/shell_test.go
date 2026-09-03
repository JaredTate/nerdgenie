package shell_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/shell"
)

// theMoment is the time the fake clock starts at in every test here.
var theMoment = time.Unix(1700000000, 0).UTC()

// slowSandbox is a sandbox that holds a command until a test lets it go, which
// is how a test watches the tool hand back an id after ten seconds without
// waiting ten seconds.
type slowSandbox struct {
	*testkit.FakeSandbox
	guard    sync.Mutex
	released chan struct{}
	ran      int
}

// newSlowSandbox returns a sandbox that holds every command until Release.
func newSlowSandbox() *slowSandbox {
	return &slowSandbox{FakeSandbox: testkit.NewFakeSandbox(), released: make(chan struct{})}
}

// Release lets every held command finish.
func (sandbox *slowSandbox) Release() { close(sandbox.released) }

// Run holds the command until the test lets it go, or until the context ends.
func (sandbox *slowSandbox) Run(ctx context.Context, command contract.SandboxCommand) (contract.SandboxResult, error) {
	sandbox.guard.Lock()
	sandbox.ran++
	sandbox.guard.Unlock()
	select {
	case <-sandbox.released:
		return sandbox.FakeSandbox.Run(ctx, command)
	case <-ctx.Done():
		return contract.SandboxResult{}, ctx.Err()
	}
}

// Ran is how many commands the sandbox was asked to run.
func (sandbox *slowSandbox) Ran() int {
	sandbox.guard.Lock()
	defer sandbox.guard.Unlock()
	return sandbox.ran
}

// newTool builds the shell tool over the sandbox and permission given.
func newTool(t *testing.T, sandbox contract.Sandbox, permission contract.Permission, clock contract.Clock) *shell.Tool {
	t.Helper()
	home := testkit.NewTempHome(t)
	return shell.New(shell.Settings{
		Sandbox:          sandbox,
		Permission:       permission,
		Clock:            clock,
		Home:             home,
		CoeusProgram:     filepath.Join(t.TempDir(), "coeus"),
		WorkingDirectory: t.TempDir(),
		Timeout:          time.Minute,
	})
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *shell.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))
	spec := tool.Spec()

	if spec.Name != contract.ToolShell {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolShell)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "command,action,id,escalate,reason" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a shell call by command, escalate, and reason", names)
	}
}

func TestACommandThatFinishesAtOnceReturnsItsOutput(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script("/bin/sh -c", contract.SandboxResult{StandardOutput: []byte("alpha\nbeta\n")})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "echo alpha; echo beta"})
	if err != nil {
		t.Fatalf("running a command failed: %v", err)
	}
	testkit.Golden(t, "a_finished_command.txt", []byte(output.Text))

	commands := sandbox.Commands()
	if len(commands) != 1 {
		t.Fatalf("the sandbox was asked to run %d commands, want one", len(commands))
	}
	if commands[0].Program != "/bin/sh" || len(commands[0].Arguments) != 2 || commands[0].Arguments[0] != "-c" {
		t.Errorf("the sandbox was asked to run %s %v, want the command through a shell", commands[0].Program, commands[0].Arguments)
	}
	if commands[0].Arguments[1] != "echo alpha; echo beta" {
		t.Errorf("the shell was given %q, want the command the model wrote", commands[0].Arguments[1])
	}
}

func TestACommandThatFailsSaysTheCodeItQuitWith(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script("/bin/sh -c", contract.SandboxResult{StandardError: []byte("no such file\n"), ExitCode: 2})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "cat nothing"})
	if err != nil {
		t.Fatalf("a command that failed was treated as a failure of the tool: %v", err)
	}
	testkit.Golden(t, "a_failed_command.txt", []byte(output.Text))
}

func TestACommandStillRunningAfterTenSecondsHandsBackAnIdToPollTailAndKill(t *testing.T) {
	sandbox := newSlowSandbox()
	sandbox.Script("/bin/sh -c", contract.SandboxResult{StandardOutput: []byte("done at last\n")})
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), clock)

	handed := make(chan contract.ToolOutput, 1)
	failed := make(chan error, 1)
	go func() {
		output, err := run(t, tool, map[string]any{"command": "sleep 30"})
		if err != nil {
			failed <- err
			return
		}
		handed <- output
	}()

	waitForSleepers(t, clock, 1)
	clock.Advance(shell.YieldAfter)

	var output contract.ToolOutput
	select {
	case output = <-handed:
	case err := <-failed:
		t.Fatalf("running a long command failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("the tool did not hand back an id ten seconds after the command started")
	}
	if !strings.Contains(output.Text, shell.FirstProcessID) {
		t.Fatalf("the tool said %q and did not hand back an id to poll", output.Text)
	}

	polled, err := run(t, tool, map[string]any{"action": "poll", "id": shell.FirstProcessID})
	if err != nil {
		t.Fatalf("polling the running command failed: %v", err)
	}
	if !strings.Contains(polled.Text, "still running") {
		t.Errorf("polling a running command said %q", polled.Text)
	}

	sandbox.Release()
	waitUntilFinished(t, tool)

	tailed, err := run(t, tool, map[string]any{"action": "tail", "id": shell.FirstProcessID})
	if err != nil {
		t.Fatalf("tailing the finished command failed: %v", err)
	}
	if !strings.Contains(tailed.Text, "done at last") {
		t.Errorf("tailing the command said %q, want what it wrote", tailed.Text)
	}
}

// waitForSleepers waits until the fake clock has the number of sleepers wanted,
// so that a test never advances the clock before the tool is waiting on it.
func waitForSleepers(t *testing.T, clock *testkit.FakeClock, wanted int) {
	t.Helper()
	for range 500 {
		if clock.Sleepers() >= wanted {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the tool never waited on the clock, so it has %d sleepers and wants %d", clock.Sleepers(), wanted)
}

// waitUntilFinished waits until polling says the command has finished.
func waitUntilFinished(t *testing.T, tool *shell.Tool) {
	t.Helper()
	for range 500 {
		polled, err := run(t, tool, map[string]any{"action": "poll", "id": shell.FirstProcessID})
		if err == nil && !strings.Contains(polled.Text, "still running") {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the command never finished after the sandbox let it go")
}

func TestKillingARunningCommandStopsIt(t *testing.T) {
	sandbox := newSlowSandbox()
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), clock)

	go func() { _, _ = run(t, tool, map[string]any{"command": "sleep 30"}) }()
	waitForSleepers(t, clock, 1)
	clock.Advance(shell.YieldAfter)
	waitForRunning(t, tool)

	killed, err := run(t, tool, map[string]any{"action": "kill", "id": shell.FirstProcessID})
	if err != nil {
		t.Fatalf("killing a running command failed: %v", err)
	}
	if !strings.Contains(killed.Text, shell.FirstProcessID) {
		t.Errorf("killing the command said %q and did not name it", killed.Text)
	}
	waitUntilFinished(t, tool)
}

// waitForRunning waits until the tool knows about the first command.
func waitForRunning(t *testing.T, tool *shell.Tool) {
	t.Helper()
	for range 500 {
		if _, err := run(t, tool, map[string]any{"action": "poll", "id": shell.FirstProcessID}); err == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the tool never took note of the running command")
}

func TestAnIdNothingIsRunningUnderIsRefused(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	for _, action := range []string{"poll", "tail", "kill"} {
		if _, err := run(t, tool, map[string]any{"action": action, "id": "p99"}); err == nil {
			t.Errorf("%s was allowed on an id nothing is running under", action)
		}
	}
}

func TestASandboxThatCannotRunTurnsTheToolOff(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.SetAvailable(errors.New("bwrap is not on this machine, so install it with apt install bubblewrap"))
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	_, err := run(t, tool, map[string]any{"command": "echo alpha"})
	if err == nil {
		t.Fatalf("a command ran with no sandbox to run it in")
	}
	if !strings.Contains(err.Error(), "bubblewrap") {
		t.Errorf("the refusal reads %q and does not say why the sandbox cannot run", err)
	}
	if len(sandbox.Commands()) != 0 {
		t.Errorf("a command was handed to a sandbox that said it could not run")
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	if _, err := run(t, tool, map[string]any{}); err == nil {
		t.Errorf("a call with no command was run")
	}
	if _, err := run(t, tool, map[string]any{"action": "dance", "id": "p1"}); err == nil {
		t.Errorf("an action the tool does not know was run")
	}
	if _, err := run(t, tool, map[string]any{"command": strings.Repeat("x", shell.MaxCommandBytes+1)}); err == nil {
		t.Errorf("a command longer than the cap was run")
	}
	if _, err := run(t, tool, map[string]any{"action": "poll"}); err == nil {
		t.Errorf("a poll with no id was run")
	}
}

func TestTheTableOfRunningCommandsIsBounded(t *testing.T) {
	sandbox := newSlowSandbox()
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), clock)

	for at := range shell.MaxRunning {
		go func() { _, _ = run(t, tool, map[string]any{"command": "sleep 30"}) }()
		waitForSleepers(t, clock, 1)
		clock.Advance(shell.YieldAfter)
		waitForRunningCount(t, sandbox, at+1)
	}

	if _, err := run(t, tool, map[string]any{"command": "one too many"}); err == nil {
		t.Errorf("a command was started with %d already running, and the table is capped at %d",
			shell.MaxRunning, shell.MaxRunning)
	}
}

// waitForRunningCount waits until the sandbox has been asked to run a number of
// commands.
func waitForRunningCount(t *testing.T, sandbox *slowSandbox, wanted int) {
	t.Helper()
	for range 500 {
		if sandbox.Ran() >= wanted {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the sandbox was asked to run %d commands, want %d", sandbox.Ran(), wanted)
}

func TestAResultOverTheCapIsCutWithANote(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script("/bin/sh -c", contract.SandboxResult{StandardOutput: []byte(strings.Repeat("x", shell.MaxStreamBytes+500))})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "cat something-enormous"})
	if err != nil {
		t.Fatalf("running a command that writes a great deal failed: %v", err)
	}
	if len(output.Text) > shell.MaxStreamBytes*2 {
		t.Errorf("the result is %d bytes, and each stream is capped at %d", len(output.Text), shell.MaxStreamBytes)
	}
	if !strings.Contains(output.Text, "not shown") {
		t.Errorf("the output was cut without saying so: %q", output.Text[len(output.Text)-80:])
	}
}

func TestACommandThatTimesOutSaysSo(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script("/bin/sh -c", contract.SandboxResult{TimedOut: true, ExitCode: -1})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "sleep 10000"})
	if err != nil {
		t.Fatalf("a command that ran out of time was treated as a failure of the tool: %v", err)
	}
	if !strings.Contains(output.Text, "ran out of time") {
		t.Errorf("a command that ran out of time said %q", output.Text)
	}
}

func TestASandboxThatRefusesTheCommandSaysWhy(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "nothing is scripted for this"})
	if err != nil {
		t.Fatalf("a sandbox that refused a command was treated as a failure of the tool: %v", err)
	}
	if !strings.Contains(output.Text, "nothing scripted") {
		t.Errorf("a refused command said %q, and it should carry what the sandbox complained about", output.Text)
	}
	_ = os.Getenv("PATH")
}
