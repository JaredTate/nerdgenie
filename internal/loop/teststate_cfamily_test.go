package loop

import (
	"strings"
	"testing"
)

// TestTheTestStateIsReadOffTheCFamilyAndRubyRunners is the reader on the
// runners of C, C++ and Ruby, which it did not know until 6 September 2026:
// on such a project the harness reran the tests after every write and read
// nothing off them, so the situation line, the stuck-test line and the
// meter's test-based progress never fired. The CTest sample is a real run on
// this machine, tab and all; the others are the runners' documented output,
// because none of them is installed here.
func TestTheTestStateIsReadOffTheCFamilyAndRubyRunners(t *testing.T) {
	for _, run := range []struct {
		name, text, line string
	}{
		{"googletest", "[==========] Running 3 tests from 1 test suite.\n[----------] 3 tests from Board\n[ RUN      ] Board.StartsEmpty\n[       OK ] Board.StartsEmpty (0 ms)\n[ RUN      ] Board.FullRowClears\nboard_test.cc:14: Failure\nExpected equality of these values:\n  rows\n    Which is: 1\n  0\n[  FAILED  ] Board.FullRowClears (0 ms)\n[ RUN      ] Board.PieceSpawns\n[       OK ] Board.PieceSpawns (0 ms)\n[----------] 3 tests from Board (0 ms total)\n[==========] 3 tests from 1 test suite ran. (0 ms total)\n[  PASSED  ] 2 tests.\n[  FAILED  ] 1 test, listed below:\n[  FAILED  ] Board.FullRowClears\n\n 1 FAILED TEST\nexit 1",
			"tests: 1 failing of 3: Board.FullRowClears"},
		{"googletest green", "[==========] Running 2 tests from 1 test suite.\n[ RUN      ] Board.StartsEmpty\n[       OK ] Board.StartsEmpty (0 ms)\n[ RUN      ] Board.PieceSpawns\n[       OK ] Board.PieceSpawns (0 ms)\n[==========] 2 tests from 1 test suite ran. (1 ms total)\n[  PASSED  ] 2 tests.\nexit 0",
			"tests: all 2 passing"},
		{"ctest", "Test project /home/x/build\n    Start 1: board_clears\n1/3 Test #1: board_clears .....................   Passed    0.00 sec\n    Start 2: piece_spawns\n2/3 Test #2: piece_spawns .....................***Failed    0.00 sec\n    Start 3: row_scores\n3/3 Test #3: row_scores .......................   Passed    0.00 sec\n\n67% tests passed, 1 tests failed out of 3\n\nTotal Test time (real) =   0.00 sec\n\nThe following tests FAILED:\n\t  2 - piece_spawns (Failed)\nErrors while running CTest\nexit 8",
			"tests: 1 failing of 3: piece_spawns"},
		{"ctest green", "1/2 Test #1: board_clears .....................   Passed    0.00 sec\n2/2 Test #2: row_scores .......................   Passed    0.00 sec\n\n100% tests passed, 0 tests failed out of 2\n\nTotal Test time (real) =   0.01 sec\nexit 0",
			"tests: all 2 passing"},
		{"ctest timeout", "1/2 Test #1: board_clears .....................   Passed    0.00 sec\n2/2 Test #2: slow_render ......................***Timeout  10.01 sec\n\n50% tests passed, 1 tests failed out of 2\n\nThe following tests FAILED:\n\t  2 - slow_render (Timeout)\nexit 8",
			"tests: 1 failing of 2: slow_render"},
		// Catch2 names a failing case only in a dashed header block; the
		// counts are read and the names are not.
		{"catch2", "-------------------------------------------------------------------------------\nfull row clears\n-------------------------------------------------------------------------------\nboard_test.cpp:14\n...............................................................................\n\nboard_test.cpp:16: FAILED:\n  REQUIRE( rows == 0 )\nwith expansion:\n  1 == 0\n\n===============================================================================\ntest cases: 3 | 2 passed | 1 failed\nassertions: 5 | 4 passed | 1 failed\nexit 1",
			"tests: 1 failing of 3"},
		{"catch2 green", "===============================================================================\nAll tests passed (5 assertions in 3 test cases)\nexit 0",
			"tests: all 3 passing"},
		{"unity", "tests/test_board.c:9:test_board_starts_empty:PASS\ntests/test_board.c:14:test_full_row_clears:FAIL: Expected 0 Was 1\ntests/test_board.c:21:test_piece_spawns:PASS\n\n-----------------------\n3 Tests 1 Failures 0 Ignored \nFAIL\nexit 1",
			"tests: 1 failing of 3: test_full_row_clears"},
		{"unity green", "tests/test_board.c:9:test_board_starts_empty:PASS\ntests/test_board.c:21:test_piece_spawns:PASS\n\n-----------------------\n2 Tests 0 Failures 0 Ignored \nOK\nexit 0",
			"tests: all 2 passing"},
		{"check", "Running suite(s): core\n66%: Checks: 3, Failures: 1, Errors: 0\ntest_board.c:14:F:core:test_full_row_clears:0: Assertion 'rows == 0' failed: rows == 1, 0 == 0\nexit 1",
			"tests: 1 failing of 3: core:test_full_row_clears"},
		{"check green", "Running suite(s): core\n100%: Checks: 3, Failures: 0, Errors: 0\nexit 0",
			"tests: all 3 passing"},
		{"minitest", "Run options: --seed 4242\n\n# Running:\n\n.F.\n\nFinished in 0.001234s, 2431.1 runs/s, 4052.0 assertions/s.\n\n  1) Failure:\nTestBoard#test_full_row_clears [test/board_test.rb:12]:\nExpected: 0\n  Actual: 1\n\n3 runs, 5 assertions, 1 failures, 0 errors, 0 skips\nexit 1",
			"tests: 1 failing of 3: TestBoard#test_full_row_clears"},
		{"minitest green", "Run options: --seed 4242\n\n# Running:\n\n...\n\nFinished in 0.001234s, 2431.1 runs/s, 4052.0 assertions/s.\n\n3 runs, 5 assertions, 0 failures, 0 errors, 0 skips\nexit 0",
			"tests: all 3 passing"},
		{"rspec", "Board\n  starts empty\n  clears a full row (FAILED - 1)\n  spawns a piece\n\nFailures:\n\n  1) Board clears a full row\n     Failure/Error: expect(board.rows).to eq(0)\n\n       expected: 0\n            got: 1\n\nFinished in 0.01 seconds (files took 0.1 seconds to load)\n3 examples, 1 failure\n\nFailed examples:\n\nrspec ./spec/board_spec.rb:12 # Board clears a full row\nexit 1",
			"tests: 1 failing of 3: Board clears a full row"},
		{"rspec green", "Finished in 0.01 seconds (files took 0.1 seconds to load)\n3 examples, 0 failures\nexit 0",
			"tests: all 3 passing"},
		{"rspec pending", "Finished in 0.01 seconds (files took 0.1 seconds to load)\n4 examples, 1 failure, 1 pending\n\nFailed examples:\n\nrspec ./spec/board_spec.rb:12 # Board clears a full row\nexit 1",
			"tests: 1 failing of 4: Board clears a full row"},
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

// TestOutputWithTheRunnersWordsIsNotATestRun keeps the new patterns anchored:
// a listing with Failed in a file name, a build log, a numbered list, and
// prose that borrows a runner's words are none of them a test run.
func TestOutputWithTheRunnersWordsIsNotATestRun(t *testing.T) {
	for _, text := range []string{
		"finished with exit code 0\nbuild/  Failed.log  test-results/  Makefile\nexit 0",
		"finished with exit code 2\nmake: *** [Makefile:12: all] Error 1\nexit 2",
		"finished with exit code 0\n[INFO] Build Failed on 3 targets, see the log\nexit 0",
		"finished with exit code 0\nThe pipeline has 3 runs, and the docs make 5 assertions about it.\nexit 0",
		"finished with exit code 0\n12 - README.md (draft)\n13 - notes.md (final)\nexit 0",
		"finished with exit code 0\ntest: build\n\tgo test ./...\nexit 0",
		"finished with exit code 0\n100% of the checks in the list are done\nexit 0",
		"finished with exit code 0\nrspec ./spec is where the specs live\nexit 0",
	} {
		if state, found := testStateIn(text); found {
			t.Errorf("%q was read as a test run: %+v", text, state)
		}
	}
}

// TestAFailingNamePrintedTwiceByGoogleTestIsNamedOnce: GoogleTest prints a
// failing test as it runs and again in the summary list.
func TestAFailingNamePrintedTwiceByGoogleTestIsNamedOnce(t *testing.T) {
	state, found := testStateIn("[  FAILED  ] Board.FullRowClears (0 ms)\n[  FAILED  ] Board.FullRowClears\n 1 FAILED TEST\nexit 1")
	if !found || len(state.failing) != 1 || strings.Count(state.line(), "Board.FullRowClears") != 1 {
		t.Errorf("the run reads %+v with the line %q, want the name once", state, state.line())
	}
}
