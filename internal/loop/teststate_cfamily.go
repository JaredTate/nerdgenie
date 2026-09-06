package loop

import (
	"strconv"
	"strings"
)

// This file reads the test runners of C, C++ and Ruby: GoogleTest, CTest,
// Catch2, Unity, Check, minitest and RSpec. The reader knew only the runners
// of JavaScript, Python and Go until 6 September 2026, and on any other code
// base the harness reran the tests after every write and read nothing off
// them, so the situation line, the stuck-test line and the meter's test-based
// progress never fired. Every pattern here is anchored to a shape a listing,
// a build log or a line of prose cannot take by accident.

// readOtherRunners reads one trimmed line of output as one of these runners
// would print it, and says whether it was one of theirs.
func (state *testState) readOtherRunners(line string) bool {
	return state.readGoogleTest(line) || state.readCTest(line) || state.readCatch2(line) ||
		state.readUnity(line) || state.readCheck(line) || state.readMinitest(line) || state.readRSpec(line)
}

// readGoogleTest reads GoogleTest, which prints a failing test as it runs and
// again in its summary list, both as "[  FAILED  ] Suite.Name".
func (state *testState) readGoogleTest(line string) bool {
	switch {
	case strings.HasPrefix(line, "[  FAILED  ] "):
		rest := strings.TrimPrefix(line, "[  FAILED  ] ")
		if count, isCount := leadingNumber(rest); isCount {
			state.failed = count
			return true
		}
		state.addFailing(withoutItsTiming(rest))
		return true
	case strings.HasPrefix(line, "[==========] ") && strings.Contains(line, " ran."):
		state.total = numberAfter(line, "[==========] ")
		return true
	case strings.HasPrefix(line, "[  PASSED  ] "), strings.HasPrefix(line, "[       OK ] "):
		return true
	case strings.HasSuffix(line, " FAILED TEST") || strings.HasSuffix(line, " FAILED TESTS"):
		count, isCount := leadingNumber(line)
		if isCount {
			state.failed = count
		}
		return isCount
	}
	return false
}

// readCTest reads CTest: one line per test ending in Passed or in a starred
// verdict, a percentage line with the counts, and a list of the failed tests
// as "N - name (Failed)".
func (state *testState) readCTest(line string) bool {
	switch {
	case strings.Contains(line, " Test #") && strings.Contains(line, " sec"):
		if strings.Contains(line, "***") {
			state.addFailing(ctestName(line))
		}
		return true
	case strings.Contains(line, "% tests passed, ") && strings.Contains(line, " tests failed out of "):
		state.failed = numberBefore(line, " tests failed out of ")
		state.total = numberFollowing(line, " tests failed out of ")
		return true
	case line == "The following tests FAILED:":
		return true
	}
	if name, ok := ctestListedFailure(line); ok {
		state.addFailing(name)
		return true
	}
	return false
}

// ctestName is the name in "2/3 Test #2: piece_spawns ...***Failed 0.00 sec".
func ctestName(line string) string {
	_, rest, _ := strings.Cut(line, ": ")
	name, _, _ := strings.Cut(rest, " .")
	return strings.TrimSpace(name)
}

// theCTestVerdicts are the words CTest puts in brackets after a failed test in
// its closing list, which is what keeps a numbered list of anything else out.
var theCTestVerdicts = []string{"Failed", "Timeout", "Exception", "Not Run", "Subprocess aborted", "SEGFAULT", "ILLEGAL", "Child aborted", "Failed  Required regular expression not found"}

// ctestListedFailure reads "2 - piece_spawns (Failed)".
func ctestListedFailure(line string) (string, bool) {
	number, rest, found := strings.Cut(line, " - ")
	if !found || !strings.HasSuffix(rest, ")") {
		return "", false
	}
	if _, err := strconv.Atoi(number); err != nil {
		return "", false
	}
	name, verdict, found := strings.Cut(rest, " (")
	if !found {
		return "", false
	}
	verdict = strings.TrimSuffix(verdict, ")")
	for _, known := range theCTestVerdicts {
		if verdict == known {
			return name, true
		}
	}
	return "", false
}

// readCatch2 reads Catch2's closing counts, "test cases: 3 | 2 passed | 1
// failed". Catch2 names a failing case only in a dashed header block some
// lines above its assertion, so the names are not read.
func (state *testState) readCatch2(line string) bool {
	switch {
	case strings.HasPrefix(line, "test cases: "):
		state.failed, state.total = 0, numberAfter(line, "test cases: ")
		for _, part := range strings.Split(line, "|") {
			words := strings.Fields(part)
			if len(words) == 2 && words[1] == "failed" {
				state.failed, _ = strconv.Atoi(words[0])
			}
		}
		return true
	case strings.HasPrefix(line, "All tests passed (") && strings.Contains(line, " test case"):
		state.total = numberBefore(line, " test case")
		return true
	}
	return false
}

// readUnity reads Unity, "file:line:test:PASS" or "file:line:test:FAIL:
// message", and its closing "3 Tests 1 Failures 0 Ignored".
func (state *testState) readUnity(line string) bool {
	parts := strings.Split(line, ":")
	if len(parts) >= 4 {
		if _, err := strconv.Atoi(parts[1]); err == nil {
			switch parts[3] {
			case "FAIL":
				state.addFailing(parts[2])
				return true
			case "PASS", "IGNORE":
				return true
			}
		}
	}
	words := strings.Fields(line)
	if len(words) == 6 && words[1] == "Tests" && words[3] == "Failures" && words[5] == "Ignored" {
		state.total, _ = strconv.Atoi(words[0])
		state.failed, _ = strconv.Atoi(words[2])
		return true
	}
	return line == "OK"
}

// readCheck reads Check, "100%: Checks: 3, Failures: 0, Errors: 0" and a
// failure as "file:line:F:suite:test:iteration: message".
func (state *testState) readCheck(line string) bool {
	if strings.Contains(line, "%: Checks: ") {
		state.total = numberFollowing(line, "Checks: ")
		state.failed = numberFollowing(line, "Failures: ") + numberFollowing(line, "Errors: ")
		return true
	}
	parts := strings.Split(line, ":")
	if len(parts) >= 6 {
		if _, err := strconv.Atoi(parts[1]); err == nil && len(parts[2]) == 1 && strings.Contains("FEP", parts[2]) {
			if parts[2] != "P" {
				state.addFailing(parts[3] + ":" + parts[4])
			}
			return true
		}
	}
	return false
}

// readMinitest reads minitest's "3 runs, 5 assertions, 1 failures, 0 errors,
// 0 skips" and the "TestName#test_x [test/x_test.rb:12]:" line under each
// numbered failure.
func (state *testState) readMinitest(line string) bool {
	if strings.Contains(line, " runs, ") && strings.Contains(line, " assertions, ") && strings.Contains(line, " failures, ") {
		state.total = numberBefore(line, " runs, ")
		state.failed = numberBefore(line, " failures, ") + numberBefore(line, " errors")
		return true
	}
	name, where, found := strings.Cut(line, " [")
	if found && strings.HasSuffix(where, "]:") && strings.Contains(name, "#") && !strings.Contains(name, " ") && strings.Contains(where, ".rb:") {
		state.addFailing(name)
		return true
	}
	return false
}

// readRSpec reads RSpec's "3 examples, 1 failure" and the "rspec ./spec/x.rb:12
// # description" lines that name the failed examples.
func (state *testState) readRSpec(line string) bool {
	if (strings.Contains(line, " examples, ") || strings.Contains(line, " example, ")) && strings.Contains(line, " failure") {
		if _, isCount := leadingNumber(line); !isCount {
			return false
		}
		state.total, _ = leadingNumber(line)
		state.failed = numberBefore(line, " failure")
		return true
	}
	if strings.HasPrefix(line, "rspec ./") && strings.Contains(line, " # ") {
		_, name, _ := strings.Cut(line, " # ")
		state.addFailing(strings.TrimSpace(name))
		return true
	}
	return false
}

// leadingNumber is the number a line starts with, when it starts with one.
func leadingNumber(line string) (int, bool) {
	words := strings.Fields(line)
	if len(words) == 0 {
		return 0, false
	}
	count, err := strconv.Atoi(strings.TrimRight(words[0], ",.;:)"))
	return count, err == nil
}

// numberBefore is the number that ends just before the label, such as the 1
// in "1 tests failed out of 3" before " tests failed".
func numberBefore(line string, label string) int {
	before, _, found := strings.Cut(line, label)
	if !found {
		return 0
	}
	words := strings.Fields(before)
	if len(words) == 0 {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimLeft(words[len(words)-1], "("))
	if err != nil {
		return 0
	}
	return count
}

// numberFollowing is the number that comes right after the label, wherever
// the label sits in the line, such as the 3 in "1 tests failed out of 3".
func numberFollowing(line string, label string) int {
	_, after, found := strings.Cut(line, label)
	if !found {
		return 0
	}
	count, _ := leadingNumber(after)
	return count
}
