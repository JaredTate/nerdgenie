package loop

import "testing"

// TestTheTestStateIsReadOffUnittestMochaBunAndDeno is the reader's reach into
// the scripting runners a project may use: Python's own unittest, Mocha, Bun's
// test runner and Deno's. On a runner the reader does not know, the record's
// situation, the stuck-test line and the meter's progress on test results all
// stay silent, which is the gap the owner named on 6 September 2026: the
// harness has to work on any code base. The unittest, Mocha and Bun samples
// were captured from real runs on this machine; the Deno sample is written
// from the runner's documented output, because Deno is not installed here.
func TestTheTestStateIsReadOffUnittestMochaBunAndDeno(t *testing.T) {
	for _, run := range []struct {
		name, text, line string
	}{
		{"unittest red", "FE.\n======================================================================\nERROR: test_spawns (tests.test_mod.BoardCase.test_spawns)\n----------------------------------------------------------------------\nTraceback (most recent call last):\n  File \"tests/test_mod.py\", line 11, in test_spawns\n    raise RuntimeError(\"no piece\")\nRuntimeError: no piece\n\n======================================================================\nFAIL: test_clears_a_full_row (tests.test_mod.BoardCase.test_clears_a_full_row)\n----------------------------------------------------------------------\nTraceback (most recent call last):\n  File \"tests/test_mod.py\", line 8, in test_clears_a_full_row\n    self.assertEqual(1, 2)\nAssertionError: 1 != 2\n\n----------------------------------------------------------------------\nRan 3 tests in 0.000s\n\nFAILED (failures=1, errors=1)\nexit 1",
			"tests: 2 failing of 3: tests.test_mod.BoardCase.test_spawns; tests.test_mod.BoardCase.test_clears_a_full_row"},
		{"unittest green", "..\n----------------------------------------------------------------------\nRan 2 tests in 0.000s\n\nOK\nexit 0",
			"tests: all 2 passing"},
		// An older Python names the case class and not the test again.
		{"unittest older", "F\n======================================================================\nFAIL: test_draw (tests.test_game.GameCase)\n----------------------------------------------------------------------\nAssertionError\n\n----------------------------------------------------------------------\nRan 1 test in 0.000s\n\nFAILED (failures=1)\nexit 1",
			"tests: 1 failing of 1: tests.test_game.GameCase"},
		{"mocha red", "\n\n  Board\n    ✔ starts empty\n    1) clears a full row\n\n  Pieces\n    ✔ spawns a piece\n    2) rotates\n\n\n  2 passing (2ms)\n  2 failing\n\n  1) Board\n       clears a full row:\n\n      AssertionError [ERR_ASSERTION]: 1 == 2\n      + expected - actual\n\n      -1\n      +2\n      \n      at Context.<anonymous> (test/board.test.js:4:48)\n\n  2) Pieces\n       rotates:\n     Error: no rotation\n      at Context.<anonymous> (test/board.test.js:8:37)\n\n\n\nexit 2",
			"tests: 2 failing of 4: Board › clears a full row; Pieces › rotates"},
		{"mocha green", "\n\n  Board\n    ✔ starts empty\n    ✔ clears a full row\n\n\n  2 passing (3ms)\n\nexit 0",
			"tests: all 2 passing"},
		{"bun red", "bun test v1.3.14 (0d9b296a)\n\nboard.test.js:\n5 |   test(\"clears a full row\", () => { expect(1).toBe(2); });\n                                                  ^\nerror: expect(received).toBe(expected)\n\nExpected: 2\nReceived: 1\n\n      at <anonymous> (board.test.js:5:47)\n(fail) Board > clears a full row [0.82ms]\n\n 2 pass\n 1 fail\n 3 expect() calls\nRan 3 tests across 1 file. [48.00ms]\nexit 1",
			"tests: 1 failing of 3: Board > clears a full row"},
		{"bun green", "bun test v1.3.14 (0d9b296a)\n\n 2 pass\n 0 fail\n 2 expect() calls\nRan 2 tests across 1 file. [5.00ms]\nexit 0",
			"tests: all 2 passing"},
		{"deno red", "running 3 tests from ./board_test.ts\nBoard starts empty ... ok (1ms)\nBoard clears a full row ... FAILED (2ms)\nspawns ... ok (0ms)\n\n ERRORS \n\nBoard clears a full row => ./board_test.ts:6:6\nerror: AssertionError: Values are not equal.\n\n FAILURES \n\nBoard clears a full row => ./board_test.ts:6:6\n\nFAILED | 2 passed | 1 failed (5ms)\n\nerror: Test failed\nexit 1",
			"tests: 1 failing of 3: Board clears a full row"},
		{"deno green", "running 3 tests from ./board_test.ts\nBoard starts empty ... ok (1ms)\nBoard clears a full row ... ok (0ms)\nspawns ... ok (0ms)\n\nok | 3 passed | 0 failed (4ms)\nexit 0",
			"tests: all 3 passing"},
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

// TestWordsOfTheScriptingRunnersInOrdinaryOutputAreNotATestRun keeps the new
// readers from seeing a run in a listing, a build log or a page of prose that
// happens to carry their words.
func TestWordsOfTheScriptingRunnersInOrdinaryOutputAreNotATestRun(t *testing.T) {
	for name, text := range map[string]string{
		"an OK on its own":      "checking the disk\nOK\nexit 0",
		"prose with counts":     "the build had 1 failing job and 2 passing lanes, so we ran 3 tests in the morning\nexit 0",
		"a listing with pass":   "2 pass\nnotes.txt\n(fail) means the check did not pass\nexit 0",
		"a build log":           "ERROR: could not open (file)\nFAILED to link\nFAILED | the pipeline\nexit 1",
		"a bare unittest label": "FAIL: the plan (see above)\nexit 1",
	} {
		if state, found := testStateIn(text); found {
			t.Errorf("%s was read as a test run: %q", name, state.line())
		}
	}
}
