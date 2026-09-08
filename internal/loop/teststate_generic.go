package loop

import (
	"strconv"
	"strings"
)

// This file is the test-state reader's last resort, tried only when no known
// runner matched. On task 8 of the night of 6 September 2026 the model wrote
// its own runner, a "zero-dependency Node test runner", and its output was
// read as no test run for the whole task: no test line in the situation, no
// rerun after a change, no stuck-test line and no test-based progress. Its
// summary line was "143 tests, 142 passed, 1 failed" and its failure line
// "FAIL tests/engine.test.js :: game ends when the stack reaches the top". As
// with every runner here, the summary line is what proves a run, and a name
// line only adds to a run the summary proved, so a FAIL line in a build log
// with no counts under it is not a run.

// genericTestStateIn reads a home-made runner's output: the counts off a
// summary line made of nothing but counts, and the failing tests off its FAIL
// lines. It says whether a summary line was there.
func genericTestStateIn(text string) (testState, bool) {
	state, found := testState{}, false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if name := genericFailingName(line); name != "" {
			state.addFailing(name)
			continue
		}
		if total, failed, isSummary := genericCounts(line); isSummary {
			state.total, state.failed, state.summary, found = total, failed, line, true
		}
	}
	if !found {
		return testState{}, false
	}
	return state, true
}

// genericSummaryIn finds the counts line of a home-made runner in a result
// another reader already read by its marks, so the counts can stand beside
// the names.
func genericSummaryIn(text string) (testState, bool) {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if total, failed, isSummary := genericCounts(line); isSummary {
			return testState{total: total, failed: failed, summary: line}, true
		}
	}
	return testState{}, false
}

// genericCounts reads a line made of nothing but counts and the words tests,
// passed and failed, in any order, with a comma or a space between them: "143
// tests, 142 passed, 1 failed", "5 passed, 1 failed" or "1 failed 5 passed".
// Both passed and failed must be there, because a line with one of them is a
// sentence or a listing, and any other word on the line makes it prose: "3 of
// the tests passed but 2 failed to load" is not a run. The total is the passed
// and the failed together when the line does not say it.
func genericCounts(line string) (total int, failed int, isSummary bool) {
	// A full stop or a comma after the last word is punctuation, not a word.
	words := strings.Fields(strings.NewReplacer(",", " ", ".", " ", ";", " ").Replace(line))
	if len(words) < 4 || len(words)%2 != 0 {
		return 0, 0, false
	}
	passed, sawPassed, sawFailed := 0, false, false
	for at := 0; at < len(words); at += 2 {
		count, err := strconv.Atoi(words[at])
		if err != nil || count < 0 {
			return 0, 0, false
		}
		switch words[at+1] {
		case "passed":
			passed, sawPassed = count, true
		case "failed":
			failed, sawFailed = count, true
		case "tests", "test", "total":
			total = count
		case "skipped", "pending", "todo":
			// A count of what did not run is not a failure and not a pass.
		default:
			return 0, 0, false
		}
	}
	if !sawPassed || !sawFailed {
		return 0, 0, false
	}
	if total == 0 {
		total = passed + failed
	}
	return total, failed, true
}

// genericFailingName reads a home-made runner's failure line, "FAIL
// tests/engine.test.js :: game ends when the stack reaches the top", and gives
// the test's name, the part after the double colon. A FAIL line without one is
// Go's or Jest's, which the readers before this one know, or a build log's,
// and names nothing.
func genericFailingName(line string) string {
	rest, isFailLine := strings.CutPrefix(line, "FAIL ")
	if !isFailLine {
		return ""
	}
	if _, name, hasMark := strings.Cut(rest, " :: "); hasMark {
		return strings.TrimSpace(name)
	}
	// Run 27's runner wrote "FAIL logic: a full board with no line is a
	// draw" with no double colon: a FAIL line that names no file is the
	// test's name; one that names a file is a file's line, which the readers
	// before this one know.
	name := strings.TrimSpace(withoutItsTiming(rest))
	if name == "" || looksLikeAFilePath(name) {
		return ""
	}
	return name
}

// looksLikeAFilePath says whether a FAIL line's text is a file rather than a
// test's name: it has a slash, or a test file's ending.
func looksLikeAFilePath(text string) bool {
	first := strings.Fields(text)[0]
	return strings.Contains(first, "/") || strings.Contains(first, ".test.") || strings.Contains(first, ".spec.") || strings.HasSuffix(first, "_test.go")
}
