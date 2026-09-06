package loop

import (
	"strconv"
	"strings"
)

// scriptingRunners reads the summaries of the scripting runners a project may
// use beside Jest, Vitest, node:test, pytest and Go: Python's own unittest,
// Mocha, Bun's test runner and Deno's. Each line reader is anchored to the
// exact shape its runner prints, so that a stray word in a listing, a build
// log or a page of prose cannot make a test run appear. What proves a run is
// a summary line; a name line only adds to a run some summary line proved.
type scriptingRunners struct {
	// unittestRan says unittest's "Ran N tests" line was seen, which is what
	// lets a bare "OK" line count as its verdict.
	unittestRan bool
	// mochaPassing is Mocha's passing count, kept until the failing count is
	// known, because Mocha never prints a total.
	mochaPassing int
	// mochaSeen says a Mocha summary was seen, so the numbered failure list at
	// the end of the run is worth reading for names.
	mochaSeen bool
}

// readLine reads one trimmed line, records what it says on the state, and
// says whether the line proves a test run.
func (runners *scriptingRunners) readLine(line string, state *testState) bool {
	if count, ok := unittestRan(line); ok {
		state.total, runners.unittestRan = count, true
		return true
	}
	if count, ok := unittestFailed(line); ok {
		state.failed = count
		return true
	}
	if line == "OK" && runners.unittestRan {
		return true
	}
	if name := unittestName(line); name != "" {
		state.addFailing(name)
		return false
	}
	if count, word, ok := mochaCount(line); ok {
		if word == "passing" {
			runners.mochaPassing, runners.mochaSeen = count, true
			return true
		}
		state.failed = count
		return runners.mochaSeen
	}
	if count, ok := bunRan(line); ok {
		state.total = count
		return true
	}
	if count, word, ok := bunCount(line); ok && word == "fail" {
		state.failed = count
		return false
	}
	if name := bunName(line); name != "" {
		state.addFailing(name)
		return false
	}
	if failed, total, ok := denoSummary(line); ok {
		state.failed, state.total = failed, total
		return true
	}
	if name := denoName(line); name != "" {
		state.addFailing(name)
		return false
	}
	return false
}

// finish settles what a runner only says at the end: Mocha's total, which is
// its passing and failing counts together, and the names in its numbered
// failure list.
func (runners *scriptingRunners) finish(text string, state *testState) {
	if !runners.mochaSeen {
		return
	}
	state.total = runners.mochaPassing + state.failed
	for _, name := range mochaNames(text) {
		state.addFailing(name)
	}
}

// unittestRan reads unittest's "Ran 3 tests in 0.000s" and gives the count.
func unittestRan(line string) (int, bool) {
	words := strings.Fields(line)
	if len(words) < 4 || words[0] != "Ran" || (words[2] != "tests" && words[2] != "test") || words[3] != "in" {
		return 0, false
	}
	count, err := strconv.Atoi(words[1])
	return count, err == nil
}

// unittestFailed reads unittest's "FAILED (failures=1, errors=1)": the
// failures and the errors together, because an error is a test that did not
// pass, and every part must be a count for the line to be unittest's.
func unittestFailed(line string) (int, bool) {
	if !strings.HasPrefix(line, "FAILED (") || !strings.HasSuffix(line, ")") {
		return 0, false
	}
	failed, parts := 0, 0
	for _, part := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "FAILED ("), ")"), ",") {
		key, value, has := strings.Cut(strings.TrimSpace(part), "=")
		count, err := strconv.Atoi(value)
		if !has || err != nil {
			return 0, false
		}
		parts++
		if key == "failures" || key == "errors" {
			failed += count
		}
	}
	return failed, parts > 0
}

// unittestName reads unittest's failure header, "FAIL: test_x
// (tests.test_mod.Case.test_x)" or "ERROR: test_y (tests.test_mod.Case)", and
// gives the dotted id in the brackets, which is what makes the line a test's
// and not a build log's.
func unittestName(line string) string {
	rest, has := strings.CutPrefix(line, "FAIL: ")
	if !has {
		rest, has = strings.CutPrefix(line, "ERROR: ")
	}
	if !has || !strings.HasSuffix(rest, ")") {
		return ""
	}
	open := strings.LastIndex(rest, " (")
	if open < 0 {
		return ""
	}
	id := rest[open+2 : len(rest)-1]
	if !strings.Contains(id, ".") || strings.ContainsAny(id, " \t") {
		return ""
	}
	return id
}

// mochaCount reads Mocha's "2 passing (2ms)" and "2 failing": the count and
// which word it was. The passing line carries a timing in brackets, and that
// is what tells it from prose.
func mochaCount(line string) (int, string, bool) {
	words := strings.Fields(line)
	if len(words) < 2 {
		return 0, "", false
	}
	count, err := strconv.Atoi(words[0])
	if err != nil {
		return 0, "", false
	}
	switch {
	case words[1] == "passing" && len(words) == 3 && strings.HasPrefix(words[2], "(") && strings.HasSuffix(words[2], ")"):
		return count, "passing", true
	case words[1] == "failing" && len(words) == 2:
		return count, "failing", true
	}
	return 0, "", false
}

// mochaNames reads the numbered failure list at the end of a Mocha run: "1)
// Board", then the suites inside it if any, then the test on a line ending in
// a colon, joined with the arrow the record uses for a test inside a suite.
// The same number and test in the tree above the summary has no colon after
// it and is left alone.
func mochaNames(text string) []string {
	names := []string{}
	lines := strings.Split(text, "\n")
	for at, raw := range lines {
		number, suite, has := strings.Cut(strings.TrimSpace(raw), ") ")
		if !has || number == "" || strings.Trim(number, "0123456789") != "" {
			continue
		}
		parts := []string{suite}
		for next := at + 1; next < len(lines) && next <= at+4; next++ {
			following := strings.TrimSpace(lines[next])
			if following == "" {
				break
			}
			if strings.HasSuffix(following, ":") {
				parts = append(parts, strings.TrimSuffix(following, ":"))
				names = append(names, strings.Join(parts, " › "))
				break
			}
			parts = append(parts, following)
		}
	}
	return names
}

// bunRan reads Bun's "Ran 3 tests across 1 file. [48.00ms]" and gives the count.
func bunRan(line string) (int, bool) {
	words := strings.Fields(line)
	if len(words) < 4 || words[0] != "Ran" || (words[2] != "tests" && words[2] != "test") || words[3] != "across" {
		return 0, false
	}
	count, err := strconv.Atoi(words[1])
	return count, err == nil
}

// bunCount reads Bun's " 2 pass" and " 1 fail": two words, a count and which.
func bunCount(line string) (int, string, bool) {
	words := strings.Fields(line)
	if len(words) != 2 || (words[1] != "pass" && words[1] != "fail") {
		return 0, "", false
	}
	count, err := strconv.Atoi(words[0])
	return count, words[1], err == nil
}

// bunName reads Bun's "(fail) Board > clears a full row [0.82ms]", without the
// timing.
func bunName(line string) string {
	name, has := strings.CutPrefix(line, "(fail) ")
	if !has {
		return ""
	}
	if open := strings.LastIndex(name, " ["); open > 0 && strings.HasSuffix(name, "]") {
		name = name[:open]
	}
	return strings.TrimSpace(name)
}

// denoSummary reads Deno's "ok | 3 passed | 0 failed (4ms)" and "FAILED | 2
// passed | 1 failed (5ms)": the failed count, and the total as the passed and
// the failed together, because Deno never says it.
func denoSummary(line string) (int, int, bool) {
	parts := strings.Split(line, " | ")
	if len(parts) < 3 || (parts[0] != "ok" && parts[0] != "FAILED") {
		return 0, 0, false
	}
	passed, failed, seen := 0, 0, 0
	for _, part := range parts[1:] {
		words := strings.Fields(part)
		if len(words) < 2 {
			continue
		}
		count, err := strconv.Atoi(words[0])
		if err != nil {
			continue
		}
		switch words[1] {
		case "passed":
			passed, seen = count, seen+1
		case "failed":
			failed, seen = count, seen+1
		}
	}
	if seen < 2 {
		return 0, 0, false
	}
	return failed, passed + failed, true
}

// denoName reads Deno's per-test line "Board clears a full row ... FAILED (2ms)".
func denoName(line string) string {
	name, rest, has := strings.Cut(line, " ... FAILED")
	if !has || (rest != "" && !strings.HasPrefix(rest, " (")) {
		return ""
	}
	return strings.TrimSpace(name)
}
