package shell_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheSameCommandWithTheSameOutputIsAnsweredInFull: the tool used to answer
// a command that wrote the same thing as the time before with "same as the
// last run of this command, unchanged: exit 0, 3 lines; tail p2 to read it
// again". On task 8 of the night of 6 September 2026 the model wanted the lines
// it had asked for, never used the pointer, and asked seventeen more times. A
// repeated command is answered in full, like any other; stopping a model that
// reruns a command is the same-call guard's job.
func TestTheSameCommandWithTheSameOutputIsAnsweredInFull(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{StandardOutput: []byte("index.html\nsrc\ntests\n")})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	want := "finished with exit code 0\nindex.html\nsrc\ntests\n"
	for round := 1; round <= 2; round++ {
		output, err := run(t, tool, map[string]any{"command": "ls"})
		if err != nil {
			t.Fatalf("run %d failed: %v", round, err)
		}
		if output.Text != want {
			t.Errorf("run %d said %q; want the whole output with \"finished with exit code 0\" on the first line, %q", round, output.Text, want)
		}
	}
	if len(sandbox.Commands()) != 2 {
		t.Errorf("the sandbox ran %d commands, want both", len(sandbox.Commands()))
	}
}

// TestACommandThatWritesNothingIsAnsweredInFullBothTimes: the harness's own
// syntax check runs "node --check" through this tool after every write or
// edit, and a file that parses writes nothing. The second check of any file
// used to get the short answer, whose first line said "exit 0" and not "exit
// code 0", so the checker read a file that parsed as one that did not: 34 of
// 46 writes on task 8 of the night of 6 September 2026, one correct file
// rewritten byte for byte twenty-one times.
func TestACommandThatWritesNothingIsAnsweredInFullBothTimes(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script(theShellPrefix(), contract.SandboxResult{})
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	for round := 1; round <= 2; round++ {
		output, err := run(t, tool, map[string]any{"command": "node --check 'game.js'"})
		if err != nil {
			t.Fatalf("check %d failed: %v", round, err)
		}
		if output.Text != "finished with exit code 0\n" {
			t.Errorf("check %d said %q; want \"finished with exit code 0\" and nothing else, which is what the syntax checker reads", round, output.Text)
		}
	}
}
