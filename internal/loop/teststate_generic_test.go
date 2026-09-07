package loop

import (
	"strings"
	"testing"
)

// theHomeMadeRunnersRun is what the model's own runner, a "zero-dependency
// Node test runner", printed on task 8 of the night of 6 September 2026, word
// for word from result r133, as the shell tool handed it back. The reader
// knew no such runner, so the whole task ran with no test line in the
// situation, no rerun after a change, no stuck-test line and no test-based
// progress.
const theHomeMadeRunnersRun = `finished with exit code 1
  ✓
  ✓
tests/lineClearAnim.test.js ...   ✓
  ✓

──────────────────────────────────────
FAIL tests/engine.test.js :: game ends when the stack reaches the top
     Error: game over detected
         at fail (/home/user/Desktop/Tater Tots Tetrisv1/test.js:25:30)
         at Object.ok (/home/user/Desktop/Tater Tots Tetrisv1/test.js:27:32)

143 tests, 142 passed, 1 failed`

// TestAHomeMadeRunnersSummaryLineIsReadAsATestRun reads the counts and the
// failing test's name off the runner above, and then the shorter shapes of
// the summary line: passed and failed alone, in either order, and a green run.
func TestAHomeMadeRunnersSummaryLineIsReadAsATestRun(t *testing.T) {
	state, found := testStateIn(theHomeMadeRunnersRun)
	if !found {
		t.Fatal("the home-made runner's output was not read as a test run")
	}
	if state.total != 143 || state.failed != 1 {
		t.Errorf("the run reads %d tests and %d failing, want 143 and 1", state.total, state.failed)
	}
	if strings.Join(state.failing, "|") != "game ends when the stack reaches the top" {
		t.Errorf("the failing tests read %v, want the one named after the double colon", state.failing)
	}
	if line := state.line(); line != `tests: 1 failing of 143: game ends when the stack reaches the top (exit 1; read from "143 tests, 142 passed, 1 failed")` {
		t.Errorf("the situation line reads %q", line)
	}
	for _, run := range []struct {
		name, text, line string
	}{
		{"passed then failed", "finished with exit code 1\n5 passed, 1 failed\nexit 1", `tests: 1 failing of 6 (exit 1; read from "5 passed, 1 failed")`},
		{"failed then passed", "finished with exit code 1\n1 failed 5 passed\nexit 1", `tests: 1 failing of 6 (exit 1; read from "1 failed 5 passed")`},
		{"green", "finished with exit code 0\n6 tests, 6 passed, 0 failed\nexit 0", "tests: all 6 passing"},
	} {
		state, found := testStateIn(run.text)
		if !found {
			t.Errorf("%s: %q was not read as a test run", run.name, run.text)
			continue
		}
		if line := state.line(); line != run.line {
			t.Errorf("%s: the situation line reads %q, want %q", run.name, line, run.line)
		}
	}
}

// TestTheGenericReaderIsTriedOnlyWhenNoRunnerMatched holds the last resort to
// its place: a Jest run whose output happens to carry the lines the last
// resort reads, a count line logged from inside a test and a FAIL line with a
// double colon, is still read as Jest, counts and names alike.
func TestTheGenericReaderIsTriedOnlyWhenNoRunnerMatched(t *testing.T) {
	jest := "finished with exit code 1\n  console.log\n    3 passed, 2 failed\n  ✕ clears a full row (3 ms)\n  ✓ spawns\n" +
		"FAIL src/engine.test.js :: not a test's name\nTests:       1 failed, 5 passed, 6 total\nexit 1"
	state, found := testStateIn(jest)
	if !found {
		t.Fatal("a Jest run was not read as a test run")
	}
	if line := state.line(); line != "tests: 1 failing of 6: clears a full row (exit 1)" {
		t.Errorf("the situation line reads %q, want Jest's own counts and name and nothing the last resort read", line)
	}
}

// TestWordsThatLookLikeASummaryAreNotARun keeps the last resort anchored: a
// line that merely contains the words, a listing with passed in a name, a
// sentence such as "all tests passed", a numbered list, and a FAIL line with
// no summary line to stand on are none of them a test run.
func TestWordsThatLookLikeASummaryAreNotARun(t *testing.T) {
	for _, text := range []string{
		"finished with exit code 0\nall tests passed\nexit 0",
		"finished with exit code 0\nthe tests passed and none failed\nexit 0",
		"finished with exit code 0\n3 of the tests passed but 2 failed to load\nexit 0",
		"finished with exit code 0\n12 passed tests were skipped and 1 failed one was not\nexit 0",
		"finished with exit code 0\nsrc  tests  passed.log  failed.log  package.json\nexit 0",
		"finished with exit code 0\n1. passed\n2. failed\nexit 0",
		"finished with exit code 0\n1 - passed the review\n2 - failed the build\nexit 0",
		"finished with exit code 0\n5 passed\nexit 0",
		"finished with exit code 1\nFAIL tests/engine.test.js :: game ends when the stack reaches the top\nexit 1",
	} {
		if state, found := testStateIn(text); found {
			t.Errorf("%q was read as a test run: %+v", text, state)
		}
	}
}
