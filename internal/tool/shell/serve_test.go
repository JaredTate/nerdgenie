package shell_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// aSocketTable writes one Linux socket table with the ports given in the
// LISTEN state, in the shape /proc/net/tcp has: the local address is the
// second field as a hex address, a colon, and a hex port, and the state is
// the fourth field, 0A for LISTEN.
func aSocketTable(t *testing.T, path string, ports ...int) {
	t.Helper()
	lines := []string{"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"}
	for at, port := range ports {
		lines = append(lines, fmt.Sprintf("   %d: 0100007F:%04X 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0", at, port))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("cannot write the socket table: %v", err)
	}
}

// newServingTool builds the tool over the sandbox given, reading the socket
// table at the path returned, which starts with port 22 listening.
func newServingTool(t *testing.T, sandbox contract.Sandbox, clock contract.Clock) (*shell.Tool, string) {
	t.Helper()
	table := filepath.Join(t.TempDir(), "tcp")
	aSocketTable(t, table, 22)
	home := testkit.NewTempHome(t)
	tool := shell.New(shell.Settings{
		Sandbox:          sandbox,
		Permission:       testkit.NewFakePermission(contract.RulingAllow),
		Clock:            clock,
		Home:             home,
		NerdGenieProgram: filepath.Join(t.TempDir(), "nerdgenie"),
		WorkingDirectory: t.TempDir(),
		SocketTables:     []string{table},
	})
	return tool, table
}

// runInTheBackground runs a call from another goroutine and hands the answer
// back on a channel, because a serve waits on the clock.
func runInTheBackground(t *testing.T, tool *shell.Tool, fields map[string]any) <-chan contract.ToolOutput {
	t.Helper()
	answers := make(chan contract.ToolOutput, 1)
	go func() {
		output, err := run(t, tool, fields)
		if err != nil {
			t.Errorf("the call failed: %v", err)
			output = contract.ToolOutput{Text: err.Error()}
		}
		answers <- output
	}()
	return answers
}

// TestAServeAnswersTheMomentItsPortIsListening: the play-test task started a
// server, got the ten-second ticket, and asked whether it was done for the
// rest of the task, 131 poll-only rounds across the day's logs, because a
// server never finishes. A serve watches the socket table instead and answers
// the moment a new port listens.
func TestAServeAnswersTheMomentItsPortIsListening(t *testing.T) {
	sandbox := newSlowSandbox()
	clock := testkit.NewFakeClock(theMoment)
	tool, table := newServingTool(t, sandbox, clock)

	answers := runInTheBackground(t, tool, map[string]any{"action": "serve", "command": "python3 -m http.server 8091"})
	waitForSleepers(t, clock, 1)
	aSocketTable(t, table, 22, 8091)
	clock.Advance(shell.ServeCheckEvery)

	answer := <-answers
	if answer.Text != "listening on 8091 as p1; check it with action check, stop it with kill\n" {
		t.Fatalf("the serve said %q", answer.Text)
	}
	if sandbox.Ran() != 1 {
		t.Errorf("the sandbox ran %d commands, want the one server", sandbox.Ran())
	}
	sandbox.Release()
}

func TestAServeWatchingOnePortIgnoresOthers(t *testing.T) {
	sandbox := newSlowSandbox()
	clock := testkit.NewFakeClock(theMoment)
	tool, table := newServingTool(t, sandbox, clock)

	answers := runInTheBackground(t, tool, map[string]any{"action": "serve", "command": "node server.js", "port": 3000})
	waitForSleepers(t, clock, 1)
	aSocketTable(t, table, 22, 8091)
	clock.Advance(shell.ServeCheckEvery)
	waitForSleepers(t, clock, 1)
	aSocketTable(t, table, 22, 8091, 3000)
	clock.Advance(shell.ServeCheckEvery)

	if answer := <-answers; answer.Text != "listening on 3000 as p1; check it with action check, stop it with kill\n" {
		t.Fatalf("the serve said %q", answer.Text)
	}
	sandbox.Release()
}

func TestAServeThatOpensNoPortInTenSecondsSaysSo(t *testing.T) {
	sandbox := newSlowSandbox()
	clock := testkit.NewFakeClock(theMoment)
	tool, _ := newServingTool(t, sandbox, clock)

	answers := runInTheBackground(t, tool, map[string]any{"action": "serve", "command": "node server.js", "port": 3000})
	for range int(shell.ServeWatchFor / shell.ServeCheckEvery) {
		waitForSleepers(t, clock, 1)
		clock.Advance(shell.ServeCheckEvery)
	}

	if answer := <-answers; answer.Text != "still starting after 10s as p1; nothing listens on 3000 yet; check it later, or tail or kill it\n" {
		t.Fatalf("the serve said %q", answer.Text)
	}
	sandbox.Release()
}

func TestAServeThatExitsEarlyAnswersLikeARun(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardError: []byte("address already in use\n"), ExitCode: 1})
	tool, _ := newServingTool(t, sandbox, testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"action": "serve", "command": "python3 -m http.server 8091"})
	if err != nil {
		t.Fatalf("a serve that exited failed as a call: %v", err)
	}
	if !strings.HasPrefix(output.Text, "finished with exit code 1") || !strings.Contains(output.Text, "address already in use") {
		t.Errorf("a serve that exited said %q, want the exit code and what it wrote", output.Text)
	}
}

func TestAServeNeedsACommand(t *testing.T) {
	tool, _ := newServingTool(t, testkit.NewFakeSandbox(), testkit.NewFakeClock(theMoment))
	if _, err := run(t, tool, map[string]any{"action": "serve"}); err == nil || !strings.Contains(err.Error(), "no command") {
		t.Errorf("a serve with no command was not refused: %v", err)
	}
}
