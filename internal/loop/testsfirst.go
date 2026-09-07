package loop

import (
	"path"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Tests first is the first rule of every work order, and words do not hold a
// small model to it; what is in front of it does. So a write or edit to a
// code file, made while the newest test run the harness holds is green, opens
// with one line saying that no failing test covers the change. The line rides
// first on the change's own result, where the record's one-line summary shows
// it, and costs no round: the harness already knows the state of the suite.
// A test file is the right first move and gets no line; a file that is not
// code is not a change a test covers; a write after a red run has its failing
// test; and a write before any test run is a scaffold, written freely.

// TheTestsFirstLine opens the result of a code change no failing test covers.
const TheTestsFirstLine = "tests first: no failing test covers this change; write it first"

// codeExtensions are the file endings the rule reads as code. A page, a
// stylesheet, a manifest or a document is not on it on purpose.
var codeExtensions = map[string]bool{
	".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".tsx": true, ".jsx": true,
	".go": true, ".py": true, ".rb": true, ".java": true, ".c": true, ".cc": true,
	".cpp": true, ".h": true, ".rs": true, ".swift": true, ".kt": true, ".php": true,
}

// testFolders are the folder names that hold tests in the languages the rule
// knows, so that a file under one of them is a test whatever its name.
var testFolders = map[string]bool{"test": true, "tests": true, "spec": true, "specs": true, "__tests__": true}

// testsFirstLine is the line a write or edit opens with when no failing test
// covers it, or nothing when the rule does not apply to this call.
func (running *run) testsFirstLine(call contract.ToolCall, failed bool) string {
	if failed || !running.lastTestsGreen || (call.Name != contract.ToolWrite && call.Name != contract.ToolEdit) {
		return ""
	}
	written := fieldOfCall(call, "path")
	if !isCode(written) || isATestFile(written) {
		return ""
	}
	return TheTestsFirstLine
}

// isCode says whether the path ends the way a code file ends.
func isCode(written string) bool {
	return codeExtensions[strings.ToLower(path.Ext(written))]
}

// isATestFile says whether the path is a test by its name, which holds "test"
// or "spec", or by a folder on its way that holds tests.
func isATestFile(written string) bool {
	name := strings.ToLower(path.Base(written))
	if strings.Contains(name, "test") || strings.Contains(name, "spec") {
		return true
	}
	for _, folder := range strings.Split(strings.ToLower(path.Dir(written)), "/") {
		if testFolders[folder] {
			return true
		}
	}
	return false
}

// noteTestState keeps the newest test run's line for the situation and
// whether it was green, which the tests-first rule reads.
func (running *run) noteTestState(state testState) {
	running.testsFact = state.line()
	running.lastTestsGreen = state.failed == 0
}
