package loop

import (
	"strings"
	"testing"
)

// theNodeTestRun is what Node's own test runner prints on a run with two
// failures, as the shell tool hands it back, exit line and all.
const theNodeTestRun = `finished with exit code 1
✔ board starts empty (0.5ms)
✖ dragon triggers when the roll is below DRAGON_CHANCE (0.6ms)
✔ pieces rotate (0.2ms)
✖ yeti pushes the piece as far left as it can go (0.1ms)
ℹ tests 51
ℹ suites 0
ℹ pass 49
ℹ fail 2
ℹ cancelled 0
ℹ skipped 0
ℹ todo 0
ℹ duration_ms 120
✖ failing tests:
✖ dragon triggers when the roll is below DRAGON_CHANCE (0.6ms)
✖ yeti pushes the piece as far left as it can go (0.1ms)
exit 1`

// TestTheTestStateIsReadOffANodeTestRun reads the counts and the names off
// Node's test runner. The names come once each, although the runner prints the
// failing ones twice.
func TestTheTestStateIsReadOffANodeTestRun(t *testing.T) {
	state, found := testStateIn(theNodeTestRun)
	if !found {
		t.Fatal("a Node test run was not read as a test run")
	}
	if state.total != 51 || state.failed != 2 {
		t.Errorf("the run reads %d tests and %d failing, want 51 and 2", state.total, state.failed)
	}
	wanted := []string{"dragon triggers when the roll is below DRAGON_CHANCE", "yeti pushes the piece as far left as it can go"}
	if strings.Join(state.failing, "|") != strings.Join(wanted, "|") {
		t.Errorf("the failing tests read %v, want %v, each once and without its timing", state.failing, wanted)
	}
	if line := state.line(); line != "tests: 2 failing of 51: dragon triggers when the roll is below DRAGON_CHANCE; yeti pushes the piece as far left as it can go" {
		t.Errorf("the situation line reads %q", line)
	}
}

// TestTheTestStateIsReadOffJestPytestAndGo holds the three other runners a
// project on this machine is likely to use, on their summary lines.
func TestTheTestStateIsReadOffJestPytestAndGo(t *testing.T) {
	for _, run := range []struct {
		name, text, line string
	}{
		{"jest", "  ✕ clears a full row (3 ms)\n  ✓ spawns\nTests:       1 failed, 5 passed, 6 total\nexit 1",
			"tests: 1 failing of 6: clears a full row"},
		{"pytest", "FAILED tests/test_game.py::test_draw - AssertionError\n===== 1 failed, 5 passed in 0.12s =====\nexit 1",
			"tests: 1 failing of 6: tests/test_game.py::test_draw"},
		{"go", "--- FAIL: TestTheBoardClears (0.00s)\n    board_test.go:12: the row stayed\nFAIL\nFAIL\tgame\t0.004s\nexit 1",
			"tests: 1 failing: TestTheBoardClears"},
		{"go green", "ok  \tgame\t0.004s\nexit 0", "tests: all passing"},
		{"node green", "✔ board starts empty (0.5ms)\nℹ tests 51\nℹ pass 51\nℹ fail 0\nexit 0", "tests: all 51 passing"},
	} {
		state, found := testStateIn(run.text)
		if !found {
			t.Errorf("%s: the run was not read as a test run", run.name)
			continue
		}
		if line := state.line(); line != run.line {
			t.Errorf("%s: the situation line reads %q, want %q", run.name, line, run.line)
		}
	}
}

// TestOrdinaryCommandOutputIsNotATestRun keeps the reader from seeing a test
// run in a directory listing or a file, where a stray word would otherwise put
// a false line in the record.
func TestOrdinaryCommandOutputIsNotATestRun(t *testing.T) {
	for _, text := range []string{
		"finished with exit code 0\nsrc tests package.json\nexit 0",
		"finished with exit code 0\n// the tests pass when the board is empty\nexit 0",
		"",
	} {
		if _, found := testStateIn(text); found {
			t.Errorf("%q was read as a test run", text)
		}
	}
}

// TestTheFailingNamesAreBounded keeps the situation to one line: at most
// MaxFailingTestsNamed names, and a count of the rest.
func TestTheFailingNamesAreBounded(t *testing.T) {
	lines := []string{"finished with exit code 1"}
	for at := 1; at <= MaxFailingTestsNamed+3; at++ {
		lines = append(lines, "✖ case "+strings.Repeat("x", at)+" (1ms)")
	}
	lines = append(lines, "ℹ tests 20", "ℹ pass 12", "ℹ fail 8", "exit 1")
	state, found := testStateIn(strings.Join(lines, "\n"))
	if !found || len(state.failing) != MaxFailingTestsNamed+3 {
		t.Fatalf("the run reads %+v, want %d failing names found", state, MaxFailingTestsNamed+3)
	}
	line := state.line()
	if !strings.HasSuffix(line, "and 3 more") || strings.Count(line, "case ") != MaxFailingTestsNamed {
		t.Errorf("the situation line reads %q, want %d names and a count of the rest", line, MaxFailingTestsNamed)
	}
}
