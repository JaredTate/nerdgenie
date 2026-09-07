package loop

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
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

// TheTestsFirstLine opens the result of a code change no failing test covers,
// when the harness cannot tell where the test goes. When it can, the line
// names the file: "add a failing test to tests/game.test.js first" for a test
// file the project already has, "write it first in tests/game.test.js" for
// the file the project's own naming gives, and for Rust, whose tests live in
// the file, "add a #[test] to src/lib.rs first".
const TheTestsFirstLine = theTestsFirstOpening + "write it first"

// theTestsFirstOpening is what every shape of the line begins with.
const theTestsFirstOpening = "tests first: no failing test covers this change; "

// MaxFilesReadForATestConvention bounds the walk that learns how a project
// names its tests. A project with more files than this has its convention
// read off the first files, and the language's default stands otherwise.
const MaxFilesReadForATestConvention = 500

// testFolderNames are the folders a project keeps its tests in, at its root,
// in the order they are looked for.
var testFolderNames = []string{"test", "tests", "spec", "specs", "__tests__"}

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
	return testsFirstLineFor(running.folder(), written)
}

// testsFirstLineFor is the line for a source file in the project folder: it
// names the test file that covers the source when the project has one, else
// the file the project's own naming gives, in the project's test folder when
// it has one. A file outside the folder gets the plain line.
func testsFirstLineFor(folder string, written string) string {
	relative, inside := insideTheFolder(folder, written)
	if !inside {
		return TheTestsFirstLine
	}
	extension := strings.ToLower(path.Ext(relative))
	if extension == ".rs" {
		return theTestsFirstOpening + "add a #[test] to " + relative + " first"
	}
	base := strings.TrimSuffix(path.Base(relative), path.Ext(relative))
	shapes := testNameShapes[extension]
	if existing := theTestFileOf(folder, relative, base, shapes); existing != "" {
		return theTestsFirstOpening + "add a failing test to " + existing + " first"
	}
	shape, placed := theTestConventionOf(folder, extension)
	if shape == nil {
		shape = shapes[0]
	}
	testFolder := placed
	if testFolder == "" {
		testFolder = theTestFolderOf(folder)
	}
	where := path.Dir(relative)
	if testFolder != "" {
		where = testFolder
	}
	return theTestsFirstOpening + "write it first in " + path.Join(where, shape(base, extension))
}

// insideTheFolder gives the file's path relative to the project folder with
// forward slashes, and says whether the file is under the folder at all.
func insideTheFolder(folder string, written string) (string, bool) {
	if folder == "" || written == "" {
		return "", false
	}
	relative := written
	if filepath.IsAbs(written) {
		under, err := filepath.Rel(folder, written)
		if err != nil {
			return "", false
		}
		relative = under
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", false
	}
	return relative, true
}

// testNameShape makes a test file's name out of a source file's base name and
// extension, the way one convention does.
type testNameShape func(base string, extension string) string

// The shapes a test file's name takes, each recognised in an existing file
// and used to name a new one.
var (
	dotTest     = func(base, extension string) string { return base + ".test" + extension }
	dotSpec     = func(base, extension string) string { return base + ".spec" + extension }
	underTest   = func(base, extension string) string { return base + "_test" + extension }
	testUnder   = func(base, extension string) string { return "test_" + base + extension }
	underSpec   = func(base, extension string) string { return base + "_spec" + extension }
	suffixTest  = func(base, extension string) string { return base + "Test" + extension }
	suffixTests = func(base, extension string) string { return base + "Tests" + extension }
	cTest       = func(base, extension string) string { return base + "_test" + strings.Replace(extension, ".h", ".c", 1) }
)

// testNameShapes are the shapes each language's tests take, the language's
// own convention first, then the others a project might have chosen.
var testNameShapes = map[string][]testNameShape{
	".js": {dotTest, dotSpec, underTest}, ".mjs": {dotTest, dotSpec, underTest}, ".cjs": {dotTest, dotSpec, underTest},
	".jsx": {dotTest, dotSpec, underTest}, ".ts": {dotTest, dotSpec, underTest}, ".tsx": {dotTest, dotSpec, underTest},
	".go":    {underTest},
	".py":    {testUnder, underTest},
	".rb":    {underSpec, underTest, testUnder},
	".java":  {suffixTest, suffixTests},
	".kt":    {suffixTest, suffixTests},
	".php":   {suffixTest},
	".swift": {suffixTests, suffixTest},
	".c":     {cTest, testUnder}, ".h": {cTest, testUnder}, ".cc": {underTest, testUnder}, ".cpp": {underTest, testUnder},
}

// theTestFileOf finds the test file the project already has for the source,
// beside it, in a __tests__ folder beside it, or in a test folder at the root,
// by every shape the language takes, and gives its path relative to the
// project, or nothing.
func theTestFileOf(folder string, relative string, base string, shapes []testNameShape) string {
	extension := strings.ToLower(path.Ext(relative))
	folders := []string{path.Dir(relative), path.Join(path.Dir(relative), "__tests__")}
	folders = append(folders, testFolderNames...)
	for _, where := range folders {
		for _, shape := range shapes {
			candidate := path.Join(where, shape(base, extension))
			if _, err := os.Stat(filepath.Join(folder, filepath.FromSlash(candidate))); err == nil {
				return candidate
			}
		}
	}
	return ""
}

// theTestConventionOf walks the project for its first test file in the
// language and reads the shape its name takes and, when it lives in a test
// folder at the root, that folder. The walk is bounded and skips what a
// package manager or a build wrote.
func theTestConventionOf(folder string, extension string) (testNameShape, string) {
	var shape testNameShape
	placed := ""
	seen := 0
	_ = filepath.WalkDir(folder, func(walked string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if walked != folder && (foldersLeftOut[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if seen > MaxFilesReadForATestConvention {
			return filepath.SkipAll
		}
		name := entry.Name()
		if strings.ToLower(path.Ext(name)) != extension {
			return nil
		}
		found := shapeOfATestName(name, extension)
		if found == nil {
			return nil
		}
		shape = found
		relative, _ := filepath.Rel(folder, walked)
		top := strings.Split(filepath.ToSlash(relative), "/")[0]
		for _, testFolder := range testFolderNames {
			if top == testFolder {
				placed = top
			}
		}
		return filepath.SkipAll
	})
	return shape, placed
}

// shapeOfATestName reads which shape a test file's name takes, or nothing
// when the name is not a test's.
func shapeOfATestName(name string, extension string) testNameShape {
	base := strings.TrimSuffix(name, name[len(name)-len(extension):])
	switch {
	case strings.HasSuffix(base, ".test"):
		return dotTest
	case strings.HasSuffix(base, ".spec"):
		return dotSpec
	case strings.HasSuffix(base, "_test"):
		return underTest
	case strings.HasPrefix(base, "test_"):
		return testUnder
	case strings.HasSuffix(base, "_spec"):
		return underSpec
	case strings.HasSuffix(base, "Tests"):
		return suffixTests
	case strings.HasSuffix(base, "Test"):
		return suffixTest
	}
	return nil
}

// theTestFolderOf is the test folder at the project's root, or nothing.
func theTestFolderOf(folder string) string {
	for _, name := range testFolderNames {
		if info, err := os.Stat(filepath.Join(folder, name)); err == nil && info.IsDir() {
			return name
		}
	}
	return ""
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
