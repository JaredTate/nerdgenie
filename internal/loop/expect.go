package loop

import (
	"context"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The expect line is the harness keeping the books. A shell, write or edit
// call may say in a few words what its result should show, and the harness
// checks it with four plain rules, in order, the first that applies: a test
// count ("tests 62 to 63", "63 tests", "all passing", "2 failing"), an exit
// code ("exit 0"), a contained text ("contains v24"), or "parses". A hit is
// said on the result's first line and counts as progress; a miss is written
// into the record as a failure with its cause, so a rewound model reads what
// happened rather than what the model remembered. On 6 September 2026, 297
// rounds of 2,173 did nothing but write the record, and 47 test rounds reran
// tests the harness had just run to learn what an expectation would have said.

// TheExpectationCouldNotBeChecked is said once a task when an expectation is
// in words none of the four rules can read.
const TheExpectationCouldNotBeChecked = "the expectation could not be checked; write it as tests N to M, exit N, contains X, or parses"

// checkTheExpectation reads the call's expect field against the result and
// returns the text with the verdict on its first line.
func (running *run) checkTheExpectation(ctx context.Context, call contract.ToolCall, text string, failed bool) string {
	expectation := strings.TrimSpace(fieldOfCall(call, "expect"))
	if expectation == "" || failed || !takesAnExpectation(call.Name) {
		return text
	}
	verdict, readable := judge(expectation, text)
	if !readable {
		if running.saidHowToExpect {
			return text
		}
		running.saidHowToExpect = true
		return text + "\n" + TheExpectationCouldNotBeChecked
	}
	if verdict == "" {
		running.noteProgress()
		return withTheFirstLine(text, firstLine(text)+" (as expected)")
	}
	failure := "expected " + expectation + ", got " + verdict
	_ = running.keeper.Apply(ctx, record.Update{Failure: &record.NewFailure{
		Text:  failure,
		Cause: "the " + call.Name + " call " + whatTheCallSays(call),
	}})
	running.hadFailure = true
	return "not as expected: " + failure + "\n" + text
}

// takesAnExpectation says which tools carry an expect field: the ones that
// change the world or run a command in it.
func takesAnExpectation(name string) bool {
	return name == contract.ToolShell || name == contract.ToolWrite || name == contract.ToolEdit
}

// judge checks one expectation against a result. It returns what came
// instead when the expectation was missed, empty when it was met, and false
// when the expectation is in no form the rules read.
func judge(expectation string, text string) (string, bool) {
	lower := strings.ToLower(expectation)
	switch {
	case strings.HasPrefix(lower, "contains "):
		if strings.Contains(text, strings.TrimSpace(expectation[len("contains "):])) {
			return "", true
		}
		return "a result without it", true
	case lower == "parses":
		return judgeParsing(text)
	case strings.HasPrefix(lower, "exit "):
		return judgeTheExitCode(strings.TrimSpace(lower[len("exit "):]), text), true
	}
	wanted, readable := readTheExpectedTests(lower)
	if !readable {
		return "", false
	}
	return judgeTheTests(wanted, text), true
}

// judgeParsing reads the syntax check's line on a write or an edit. A file no
// checker reads, such as a JSON manifest, cannot be said to parse or not, so
// "parses" on it is an expectation the harness could not check rather than a
// miss; the ninth fresh run wrote a false failure into the record for one.
func judgeParsing(text string) (string, bool) {
	if lineStartingWith(text, TheFileDoesNotParse) != "" {
		return "a file that does not parse", true
	}
	if lineStartingWith(text, TheFileParses) != "" {
		return "", true
	}
	return "", false
}

// judgeTheExitCode reads the command's exit line.
func judgeTheExitCode(wanted string, text string) string {
	code, found := exitCodeIn(text)
	if !found {
		return "no exit code"
	}
	if code == wanted {
		return ""
	}
	return "exit " + code
}

// expectedTests is what a test expectation asks for: how many tests in all,
// or nought when it does not say, and how many failing.
type expectedTests struct {
	total  int
	failed int
}

// readTheExpectedTests reads the test forms: "tests N to M" and "M tests"
// mean M tests, all passing; "all passing" means none failing; "N failing"
// means N failing.
func readTheExpectedTests(lower string) (expectedTests, bool) {
	words := strings.Fields(strings.NewReplacer(",", "", ".", "").Replace(lower))
	switch {
	case len(words) == 4 && words[0] == "tests" && words[2] == "to" && isWholeNumber(words[3]):
		total, _ := strconv.Atoi(words[3])
		return expectedTests{total: total}, true
	case len(words) == 2 && words[1] == "tests" && isWholeNumber(words[0]):
		total, _ := strconv.Atoi(words[0])
		return expectedTests{total: total}, true
	case len(words) == 2 && words[0] == "all" && words[1] == "passing":
		return expectedTests{}, true
	case len(words) == 2 && words[1] == "failing" && isWholeNumber(words[0]):
		failed, _ := strconv.Atoi(words[0])
		return expectedTests{failed: failed}, true
	}
	return expectedTests{}, false
}

// judgeTheTests reads the test state off the result, either a runner's own
// output or the line the harness added after a change, and compares it.
func judgeTheTests(wanted expectedTests, text string) string {
	state, found := testStateIn(text)
	if !found {
		state, found = stateFromTheChangeLine(lineStartingWith(text, TheTestsAfterAChange))
	}
	if !found {
		return "no test run in this result"
	}
	if state.failed != wanted.failed || (wanted.total > 0 && state.total != wanted.total) {
		return strings.TrimPrefix(state.line(), "tests: ")
	}
	return ""
}

// stateFromTheChangeLine reads the counts back off the line the harness put
// on a change: "tests after this change: all 10 passing" or "... 2 failing of
// 10: names".
func stateFromTheChangeLine(line string) (testState, bool) {
	if line == "" {
		return testState{}, false
	}
	said := strings.TrimPrefix(line, TheTestsAfterAChange)
	said, _, _ = strings.Cut(said, ":")
	words := strings.Fields(said)
	state := testState{}
	switch {
	case len(words) >= 2 && words[0] == "all" && isWholeNumber(words[1]):
		state.total, _ = strconv.Atoi(words[1])
	case len(words) >= 2 && words[0] == "all" && words[1] == "passing":
	case len(words) >= 2 && words[1] == "failing" && isWholeNumber(words[0]):
		state.failed, _ = strconv.Atoi(words[0])
		if len(words) >= 4 && words[2] == "of" && isWholeNumber(words[3]) {
			state.total, _ = strconv.Atoi(words[3])
		}
	default:
		return testState{}, false
	}
	return state, true
}

// withTheFirstLine puts a new first line on a text.
func withTheFirstLine(text string, first string) string {
	_, rest, more := strings.Cut(text, "\n")
	if !more {
		return first
	}
	return first + "\n" + rest
}
