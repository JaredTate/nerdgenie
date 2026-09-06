package loop

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// MaxFailingTestsNamed is how many failing tests the record names on one line
// before it says how many more there are, so that a suite with a hundred red
// tests is one line and not a page.
const MaxFailingTestsNamed = 5

// MaxChangedFilesNamed is how many changed files a failure names as its cause
// before it says how many more there were, by their last path element, so
// that a lesson stays one readable line: the live run's first failure named
// eight files by their full paths and ran to three hundred characters.
const MaxChangedFilesNamed = 3

// testState is what a test runner's own summary says: how many tests ran, how
// many failed, and which ones. The harness reads it off a shell result for
// itself, because the live game build listed forty-five test runs in its
// record as "finished with exit code 0", the model having piped the runner
// through grep, and so had no memory of what was red but the raw output in the
// window.
type testState struct {
	// total is how many tests the runner said it ran, or zero when it did not say.
	total int
	// failed is how many the runner said failed.
	failed int
	// failing names the failed tests in the order they were printed, each once.
	failing []string
	// filesFailed and filesTotal are Vitest's count of test files, which is
	// the only count a file that could not be loaded appears in.
	filesFailed, filesTotal int
}

// line is the one line the situation and the result carry: the counts, and the
// first few names.
func (state testState) line() string {
	if state.failed == 0 {
		if state.total > 0 {
			return fmt.Sprintf("tests: all %d passing", state.total)
		}
		return "tests: all passing"
	}
	said := fmt.Sprintf("tests: %d failing", state.failed)
	if state.total > 0 {
		said += fmt.Sprintf(" of %d", state.total)
	}
	if len(state.failing) == 0 {
		return said
	}
	named := state.failing
	if len(named) > MaxFailingTestsNamed {
		named = named[:MaxFailingTestsNamed]
	}
	said += ": " + strings.Join(named, "; ")
	if rest := len(state.failing) - len(named); rest > 0 {
		said += fmt.Sprintf(" and %d more", rest)
	}
	return said
}

// key is what tells one red run from another: the names when the runner gave
// them, and the count when it did not.
func (state testState) key() string {
	if state.failed == 0 {
		return ""
	}
	if len(state.failing) == 0 {
		return strconv.Itoa(state.failed)
	}
	return strings.Join(state.failing, "\n")
}

// testStateIn reads a test runner's summary out of a shell result, and says
// whether it found one. It knows the runners a project is likely to use:
// Node's own test runner, Jest, Vitest, pytest and Go's here, and Python's
// unittest, Mocha, Bun and Deno in teststate_scripting.go. A result with
// no summary line and no marked test is not a test run, however the word
// "tests" turns up in it, so a directory listing never puts a line in the
// record.
func testStateIn(text string) (testState, bool) {
	state, found := testState{}, false
	scripting := scriptingRunners{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "✖ ") && !strings.HasSuffix(line, ":"):
			state.addFailing(withoutItsTiming(strings.TrimPrefix(line, "✖ ")))
			found = true
		case strings.HasPrefix(line, "✕ "):
			state.addFailing(withoutItsTiming(strings.TrimPrefix(line, "✕ ")))
			found = true
		// Jest without --verbose names a failing test only in its failure
		// header, "● suite › test"; "● Console" and the like carry no arrow.
		case strings.HasPrefix(line, "● ") && strings.Contains(line, " › "):
			state.addFailing(strings.TrimPrefix(line, "● "))
			found = true
		case strings.HasPrefix(line, "✔ ") || strings.HasPrefix(line, "✓ "):
			found = true
		case strings.HasPrefix(line, "ℹ tests "):
			state.total, found = numberAfter(line, "ℹ tests "), true
		case strings.HasPrefix(line, "ℹ fail "):
			state.failed, found = numberAfter(line, "ℹ fail "), true
		case strings.HasPrefix(line, "Tests:"):
			state.readJestSummary(line)
			found = true
		case strings.HasPrefix(line, "Tests ") && strings.Contains(line, "("):
			state.failed, state.total = readVitestCounts(strings.TrimPrefix(line, "Tests "))
			found = true
		case strings.HasPrefix(line, "Test Files ") && strings.Contains(line, "("):
			state.filesFailed, state.filesTotal = readVitestCounts(strings.TrimPrefix(line, "Test Files "))
			found = true
		case strings.HasPrefix(line, "=") && (strings.Contains(line, " passed") || strings.Contains(line, " failed")):
			state.readPytestSummary(line)
			found = true
		case strings.HasPrefix(line, "FAILED ") && strings.Contains(line, "::"):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "FAILED "), " - ")
			state.addFailing(strings.TrimSpace(name))
			found = true
		case strings.HasPrefix(line, "--- FAIL: "):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "--- FAIL: "), " ")
			state.addFailing(name)
			found = true
		case line == "PASS" || line == "FAIL" || strings.HasPrefix(line, "ok  \t") || strings.HasPrefix(line, "FAIL\t"):
			found = true
		// Python's unittest, Mocha, Bun and Deno, in teststate_scripting.go.
		case scripting.readLine(line, &state):
			found = true
		default: // The runners of C, C++ and Ruby, in teststate_cfamily.go.
			found = state.readOtherRunners(line) || found
		}
	}
	scripting.finish(text, &state)
	if !found {
		return testState{}, false
	}
	state.settle()
	return state, true
}

// settle squares the counts once every line is read: the failed count is at
// least the number of names given, and a test file Vitest could not load ran
// no test, so the suite is red by the files even when every test that ran
// passed.
func (state *testState) settle() {
	if state.failed < len(state.failing) {
		state.failed = len(state.failing)
	}
	if state.failed == 0 && state.filesFailed > 0 {
		state.failed, state.total = state.filesFailed, state.filesTotal
	}
}

// readVitestCounts reads "16 failed | 56 passed (72)", Vitest's shape for both
// its tests and its files: the failed count, and the total in the brackets.
func readVitestCounts(text string) (failed int, total int) {
	words := strings.Fields(text)
	for at := 0; at+1 < len(words); at++ {
		if count, err := strconv.Atoi(words[at]); err == nil && words[at+1] == "failed" {
			failed = count
		}
	}
	if len(words) > 0 {
		total, _ = strconv.Atoi(strings.Trim(words[len(words)-1], "()"))
	}
	return failed, total
}

// addFailing writes down one failing test, once, however many times the
// runner prints it.
func (state *testState) addFailing(name string) {
	if name == "" {
		return
	}
	for _, known := range state.failing {
		if known == name {
			return
		}
	}
	state.failing = append(state.failing, name)
}

// readJestSummary reads "Tests: 1 failed, 5 passed, 6 total".
func (state *testState) readJestSummary(line string) {
	for _, part := range strings.Split(strings.TrimPrefix(line, "Tests:"), ",") {
		words := strings.Fields(part)
		if len(words) < 2 {
			continue
		}
		count, err := strconv.Atoi(words[0])
		if err != nil {
			continue
		}
		switch words[1] {
		case "failed":
			state.failed = count
		case "total":
			state.total = count
		}
	}
}

// readPytestSummary reads "==== 1 failed, 5 passed in 0.12s ====": the total is
// the failed and the passed together, because pytest never says it.
func (state *testState) readPytestSummary(line string) {
	passed := 0
	words := strings.Fields(strings.Trim(line, "= "))
	for at := 0; at+1 < len(words); at++ {
		count, err := strconv.Atoi(words[at])
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(words[at+1], ",") {
		case "failed":
			state.failed = count
		case "passed":
			passed = count
		}
	}
	state.total = state.failed + passed
}

// numberAfter reads the count that follows a label, or zero.
func numberAfter(line string, label string) int {
	words := strings.Fields(strings.TrimPrefix(line, label))
	if len(words) == 0 {
		return 0
	}
	count, err := strconv.Atoi(words[0])
	if err != nil {
		return 0
	}
	return count
}

// withoutItsTiming takes the "(0.5ms)" a runner writes after a test's name off
// the end of it.
func withoutItsTiming(name string) string {
	name = strings.TrimSpace(name)
	if at := strings.LastIndex(name, " ("); at > 0 && strings.HasSuffix(name, ")") {
		return strings.TrimSpace(name[:at])
	}
	return name
}

// StuckTestRuns is how many test runs in a row the same failing set is seen
// on before the model is told so: the fresh Tetris build's yeti task on the
// morning of 6 September spent forty rounds on one failing test, every probe
// counted as progress, and nothing said what a person watching would have.
const StuckTestRuns = 12

// TheStuckTestLine opens the line said when the same tests have failed on
// StuckTestRuns runs in a row.
const TheStuckTestLine = "The same tests have failed on the last "

// theStuckTestLine names the failing tests and the three ways out.
func theStuckTestLine(runs int, state testState) string {
	return fmt.Sprintf("%s%d test runs: %s. Write a failure with its cause, then take one of three ways out: change the approach, fix the test itself if its expectation is wrong, or write the failure, leave the step unmarked and go on to the next.",
		TheStuckTestLine, runs, strings.TrimPrefix(state.line(), "tests: "))
}

// writeWhatTheTestsShow reads a test runner's summary off a shell result and
// writes it into the record: the state into the situation on the next round,
// and, when the run is red, what is failing changed, and a file was changed
// since the last run, a failure with that change as its cause. The record's
// failures are what keep a small model from trying the same thing twice, and
// after a hundred and forty-five rounds the live record held none, because the
// model never wrote one. A red run with no change before it is the state the
// task found, not something it did, and the same red run seen again is the
// same failure still standing. A failure the record will not take, which a
// record at its size cap is, costs the line and nothing more.
func (running *run) writeWhatTheTestsShow(ctx context.Context, call contract.ToolCall, text string, label string) {
	if call.Name != contract.ToolShell || running.keeper == nil {
		return
	}
	state, found := testStateIn(text)
	if !found {
		return
	}
	running.testsFact = state.line()
	running.noteTheTestsImprovedOrNot(state)
	changed := running.changedSinceTheLastRun
	running.changedSinceTheLastRun = nil
	key := state.key()
	sameAsBefore := key == running.lastFailingSet
	running.lastFailingSet = key
	running.noteAStuckTest(state, sameAsBefore)
	if state.failed == 0 || len(changed) == 0 || sameAsBefore {
		return
	}
	running.hadFailure = true
	named := shortNamesOf(changed)
	_ = running.keeper.Apply(ctx, record.Update{Failure: &record.NewFailure{
		Text:  strings.TrimPrefix(state.line(), "tests: ") + " after changing " + named,
		Cause: "the change to " + named + " before the run " + label,
	}})
}

// noteAStuckTest counts the test runs in a row on which the same tests have
// failed, and says so at StuckTestRuns and every StuckTestRuns after.
func (running *run) noteAStuckTest(state testState, sameAsBefore bool) {
	if state.failed == 0 || !sameAsBefore {
		running.sameFailingRuns = 0
		if state.failed > 0 {
			running.sameFailingRuns = 1
		}
		return
	}
	running.sameFailingRuns++
	if running.sameFailingRuns%StuckTestRuns == 0 {
		running.remember(contract.Message{Role: contract.RoleUser, Text: theStuckTestLine(running.sameFailingRuns, state)})
	}
}

// shortNamesOf names files by their last path element, at most
// MaxChangedFilesNamed of them, and says how many more there were.
func shortNamesOf(paths []string) string {
	return shortNames(paths, MaxChangedFilesNamed)
}

// shortNames names files by their last path element, at most the number given,
// and says how many more there were.
func shortNames(paths []string, most int) string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, path[strings.LastIndex(path, "/")+1:])
	}
	if len(names) <= most {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:most], ", "), len(names)-most)
}
