package shell_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
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
		NerdGenieProgram: filepath.Join(t.TempDir(), "nerdgenie"),
		WorkingDirectory: t.TempDir(),
		Timeout:          time.Minute,
	})
}

// theShellPrefix is what a fake sandbox is scripted with: the shell this
// machine has, and the flags the tool puts in front of every command.
func theShellPrefix() string {
	program, arguments := shell.CommandLine("")
	return strings.TrimSpace(program + " " + strings.Join(arguments[:len(arguments)-1], " "))
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
	if strings.Join(names, ",") != "command,action,id,escalate,reason,expect,port,path" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a shell call by command, escalate, and reason", names)
	}
}

func TestACommandThatFinishesAtOnceReturnsItsOutput(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("alpha\nbeta\n")})
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
	program, arguments := shell.CommandLine("echo alpha; echo beta")
	if commands[0].Program != program || !slices.Equal(commands[0].Arguments, arguments) {
		t.Errorf("the sandbox was asked to run %s %v, want %s %v",
			commands[0].Program, commands[0].Arguments, program, arguments)
	}
	if last := commands[0].Arguments[len(commands[0].Arguments)-1]; last != "echo alpha; echo beta" {
		t.Errorf("the shell was given %q, want the command the model wrote", last)
	}
}

// TestACommandRunsThroughAShellThatTellsTheTruthAboutAPipe pins the shape of
// the command line brief 6.6 asked for: bash with pipefail set on a machine
// that has bash, so that the failing command in a pipe reports its own code,
// and /bin/sh with nothing added on a machine that has not, because dash cannot
// set pipefail. The model's command itself is handed over untouched either way.
func TestACommandRunsThroughAShellThatTellsTheTruthAboutAPipe(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	written := "node --test tests/ 2>&1 | tail -40"
	if _, err := run(t, tool, map[string]any{"command": written}); err != nil {
		t.Fatalf("running a command in a pipe failed: %v", err)
	}
	commands := sandbox.Commands()
	if len(commands) != 1 {
		t.Fatalf("the sandbox was asked to run %d commands, want one", len(commands))
	}
	held := commands[0]
	if last := held.Arguments[len(held.Arguments)-1]; last != written {
		t.Errorf("the shell was given %q, want the command the model wrote", last)
	}
	flags := strings.Join(held.Arguments[:len(held.Arguments)-1], " ")
	if strings.HasSuffix(held.Program, "/bash") {
		if flags != "-o pipefail -c" {
			t.Errorf("bash was given the flags %q, and without -o pipefail a failing command in a pipe reports 0", flags)
		}
		return
	}
	if held.Program != "/bin/sh" || flags != "-c" {
		t.Errorf("the sandbox was asked to run %s %v, want bash with pipefail or /bin/sh with -c", held.Program, held.Arguments)
	}
}

func TestACommandThatFailsSaysTheCodeItQuitWith(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardError: []byte("no such file\n"), ExitCode: 2})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"command": "cat nothing"})
	if err != nil {
		t.Fatalf("a command that failed was treated as a failure of the tool: %v", err)
	}
	testkit.Golden(t, "a_failed_command.txt", []byte(output.Text))
}

func TestACommandStillRunningAfterTenSecondsHandsBackAnIdToPollTailAndKill(t *testing.T) {
	sandbox := newSlowSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("done at last\n")})
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

	// A running command may hold a sleeper of its own for its timeout, so the
	// poll is the next sleeper over whatever is sleeping now.
	sleepersBefore := clock.Sleepers()
	polled := pollInTheBackground(t, tool, shell.FirstProcessID)
	waitForSleepers(t, clock, sleepersBefore+1)
	clock.Advance(shell.PollWaitsFor)
	if answer := <-polled; !strings.Contains(answer.Text, "still running") {
		t.Errorf("polling a running command said %q", answer.Text)
	}

	sandbox.Release()
	waitUntilFinished(t, tool, shell.FirstProcessID)

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
func waitUntilFinished(t *testing.T, tool *shell.Tool, id string) {
	t.Helper()
	for range 500 {
		polled, err := run(t, tool, map[string]any{"action": "poll", "id": id})
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
	waitForRunning(t, tool, shell.FirstProcessID)

	killed, err := run(t, tool, map[string]any{"action": "kill", "id": shell.FirstProcessID})
	if err != nil {
		t.Fatalf("killing a running command failed: %v", err)
	}
	if !strings.Contains(killed.Text, shell.FirstProcessID) {
		t.Errorf("killing the command said %q and did not name it", killed.Text)
	}
	waitUntilFinished(t, tool, shell.FirstProcessID)
}

// waitForRunning waits until the tool knows about the first command.
func waitForRunning(t *testing.T, tool *shell.Tool, id string) {
	t.Helper()
	// A tail answers at once on a command the tool knows; a poll would wait.
	for range 500 {
		if _, err := run(t, tool, map[string]any{"action": "tail", "id": id}); err == nil {
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
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte(strings.Repeat("x", shell.MaxStreamBytes+500))})
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
	sandbox.Script(theShellPrefix(), contract.SandboxResult{TimedOut: true, ExitCode: -1})
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

// TestAPollSaysHowLongTheCommandHasBeenRunning holds what the fourth Tetris run
// found: every poll of a quiet command answered with the same words, the loop
// read four identical answers as the model asking the same thing over and
// over, and stopped a task that was waiting on a slow test run. A poll names
// the time the command has been running, so no two polls read the same.
func TestAPollSaysHowLongTheCommandHasBeenRunning(t *testing.T) {
	sandbox := newSlowSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{})
	clock := testkit.NewFakeClock(theMoment)
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), clock)
	handed := make(chan contract.ToolOutput, 1)
	go func() {
		output, _ := run(t, tool, map[string]any{"command": "sleep 1000"})
		handed <- output
	}()
	waitForSleepers(t, clock, 1)
	clock.Advance(shell.YieldAfter)
	select {
	case <-handed:
	case <-time.After(5 * time.Second):
		t.Fatalf("the tool did not hand back an id ten seconds after the command started")
	}

	// A poll waits on the clock for the command, so each one is answered by
	// advancing the clock past the wait; the two answers are then twenty
	// seconds apart.
	// A running command may hold a sleeper of its own for its timeout, so the
	// poll is the next sleeper over whatever is sleeping now.
	sleepersBefore := clock.Sleepers()
	polled := pollInTheBackground(t, tool, shell.FirstProcessID)
	waitForSleepers(t, clock, sleepersBefore+1)
	clock.Advance(shell.PollWaitsFor)
	first := <-polled
	polled = pollInTheBackground(t, tool, shell.FirstProcessID)
	waitForSleepers(t, clock, 1)
	clock.Advance(shell.PollWaitsFor)
	second := <-polled
	sandbox.Release()
	if first.Text == second.Text {
		t.Errorf("two polls ten seconds apart read the same: %q", first.Text)
	}
	if !strings.Contains(second.Text, "after") {
		t.Errorf("the poll does not say how long the command has run: %q", second.Text)
	}
}

// TestAKillByNamePatternIsRefusedAndToldToKillByPid is the rule this machine
// has held since its first agent closed every terminal on the desktop: a
// shell's own command line matches the pattern it names, so the shell dies
// with it. On the fifth game build the model ran pkill -f on its own server
// and got exit code 143, its own shell killed. The call is refused before it
// runs, and the refusal says what to do instead.
func TestAKillByNamePatternIsRefusedAndToldToKillByPid(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))
	for _, command := range []string{
		`pkill -f "python3 -m http.server 8091"`,
		"killall node",
		"pgrep -af nerdgenie | awk '{print $1}' | xargs kill",
		"cd ~/game && pkill -f serve.js; node serve.js",
		`for i in $(seq 1 30); do if ! pgrep -f "node playtest.js" >/dev/null; then echo done; break; fi; sleep 1; done`,
		"while pgrep -f serve.js; do sleep 1; done",
	} {
		_, err := run(t, tool, map[string]any{"command": command})
		if err == nil {
			t.Errorf("%q was run, and a kill by name pattern kills the shell that runs it", command)
			continue
		}
		for _, words := range []string{"its own command line", "exact process id", "ss -ltnp"} {
			if !strings.Contains(err.Error(), words) {
				t.Errorf("the refusal of %q reads %q, want it to say %q", command, err.Error(), words)
			}
		}
	}
	for _, command := range []string{"kill 4242", "kill -9 4242", "echo pkill is a word", "ls pgrep-notes"} {
		if _, err := run(t, tool, map[string]any{"command": command}); err != nil {
			t.Errorf("%q was refused: %v, and a kill by exact id or a mention in passing is ordinary", command, err)
		}
	}
}
