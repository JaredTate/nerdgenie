package shell_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// TestTheSameCommandWithTheSameOutputIsAnsweredShort: 84 rounds in the day's
// logs ran a command identical to an earlier one and read the same output
// again. The command still runs, so the answer is honest; only the tokens go.
func TestTheSameCommandWithTheSameOutputIsAnsweredShort(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("index.html\nsrc\ntests\n")})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	first, err := run(t, tool, map[string]any{"command": "ls"})
	if err != nil || !strings.Contains(first.Text, "index.html") {
		t.Fatalf("the first run said %q, %v; want the listing", first.Text, err)
	}
	second, err := run(t, tool, map[string]any{"command": "ls"})
	if err != nil {
		t.Fatalf("the second run failed: %v", err)
	}
	if second.Text != "same as the last run of this command, unchanged: exit 0, 3 lines; tail p2 to read it again\n" {
		t.Errorf("the second run said %q", second.Text)
	}
	if len(sandbox.Commands()) != 2 {
		t.Errorf("the sandbox ran %d commands, want both, because the answer is only honest when the command ran", len(sandbox.Commands()))
	}

	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("index.html\nsrc\ntests\nnotes.md\n")})
	third, err := run(t, tool, map[string]any{"command": "ls"})
	if err != nil || !strings.Contains(third.Text, "notes.md") {
		t.Errorf("a run whose output changed said %q, %v; want the new output in full", third.Text, err)
	}
	if tailed, err := run(t, tool, map[string]any{"action": "tail", "id": "p2"}); err != nil || !strings.Contains(tailed.Text, "tests") {
		t.Errorf("tailing the shortened run said %q, %v; want its output", tailed.Text, err)
	}
}

func TestADifferentCommandOrAFailingOneIsNeverAnsweredShort(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardError: []byte("no such file\n"), ExitCode: 2})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	for _, command := range []string{"cat nothing", "cat nothing", "cat nothing"} {
		output, err := run(t, tool, map[string]any{"command": command})
		if err != nil || !strings.Contains(output.Text, "no such file") {
			t.Errorf("a failing command said %q, %v; want its error every time", output.Text, err)
		}
	}
	output, err := run(t, tool, map[string]any{"command": "cat something"})
	if err != nil || strings.Contains(output.Text, "same as") {
		t.Errorf("a different command was answered short: %q, %v", output.Text, err)
	}
}

func TestTheMemoryOfPastRunsIsBounded(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("ok\n")})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	if _, err := run(t, tool, map[string]any{"command": "echo first"}); err != nil {
		t.Fatalf("the first run failed: %v", err)
	}
	for at := range shell.RunsRemembered {
		if _, err := run(t, tool, map[string]any{"command": "echo " + strings.Repeat("x", at+1)}); err != nil {
			t.Fatalf("run %d failed: %v", at, err)
		}
	}
	output, err := run(t, tool, map[string]any{"command": "echo first"})
	if err != nil || strings.Contains(output.Text, "same as") {
		t.Errorf("a command pushed out of the memory was still answered short: %q, %v", output.Text, err)
	}
}
