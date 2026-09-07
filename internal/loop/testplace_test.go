package loop

import (
	"os"
	"path/filepath"
	"testing"
)

// aProjectWith makes a temporary project folder holding the files named, each
// empty, with their folders.
func aProjectWith(t *testing.T, files ...string) string {
	t.Helper()
	folder := t.TempDir()
	for _, file := range files {
		path := filepath.Join(folder, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return folder
}

// TestTheTestsFirstLineNamesTheTestFileToWrite: the line names the test file
// that covers the source, found by the project's own naming, or the file the
// project's convention gives when there is none yet, or the language's
// default, placed in the project's test folder when it has one.
func TestTheTestsFirstLineNamesTheTestFileToWrite(t *testing.T) {
	opening := "tests first: no failing test covers this change; "
	for _, shape := range []struct {
		name    string
		files   []string
		written string
		want    string
	}{
		{"a test beside the source", []string{"src/game.js", "src/game.test.js"}, "src/game.js", opening + "add a failing test to src/game.test.js first"},
		{"a spec beside the source", []string{"src/game.ts", "src/game.spec.ts"}, "src/game.ts", opening + "add a failing test to src/game.spec.ts first"},
		{"a test in the test folder", []string{"game.js", "tests/game.test.js"}, "game.js", opening + "add a failing test to tests/game.test.js first"},
		{"a python test in the test folder", []string{"app/board.py", "tests/test_board.py"}, "app/board.py", opening + "add a failing test to tests/test_board.py first"},
		{"a go test beside the source", []string{"pkg/board.go", "pkg/board_test.go"}, "pkg/board.go", opening + "add a failing test to pkg/board_test.go first"},
		{"the neighbours' convention", []string{"game.js", "tests/board.test.js"}, "game.js", opening + "write it first in tests/game.test.js"},
		{"the neighbours' spec convention", []string{"src/game.ts", "src/board.spec.ts"}, "src/game.ts", opening + "write it first in src/game.spec.ts"},
		{"the neighbours' python convention", []string{"app/board.py", "tests/test_other.py"}, "app/board.py", opening + "write it first in tests/test_board.py"},
		{"no test yet in javascript", []string{"src/game.js"}, "src/game.js", opening + "write it first in src/game.test.js"},
		{"no test yet with an empty test folder", []string{"src/game.js", "tests/.keep"}, "src/game.js", opening + "write it first in tests/game.test.js"},
		{"no test yet in go", []string{"pkg/board.go"}, "pkg/board.go", opening + "write it first in pkg/board_test.go"},
		{"no test yet in python", []string{"app/board.py"}, "app/board.py", opening + "write it first in app/test_board.py"},
		{"no test yet in ruby with a spec folder", []string{"lib/board.rb", "spec/.keep"}, "lib/board.rb", opening + "write it first in spec/board_spec.rb"},
		{"no test yet in java", []string{"src/Board.java"}, "src/Board.java", opening + "write it first in src/BoardTest.java"},
		{"no test yet in c", []string{"src/board.c"}, "src/board.c", opening + "write it first in src/board_test.c"},
		{"rust tests live in the file", []string{"src/lib.rs"}, "src/lib.rs", opening + "add a #[test] to src/lib.rs first"},
		{"a file that is not there yet", []string{"src/board.js"}, "src/game.js", opening + "write it first in src/game.test.js"},
	} {
		folder := aProjectWith(t, shape.files...)
		for _, written := range []string{shape.written, filepath.Join(folder, shape.written)} {
			if got := testsFirstLineFor(folder, written); got != shape.want {
				t.Errorf("%s: writing %q gives the line %q, want %q", shape.name, written, got, shape.want)
			}
		}
	}
}

// TestAFileOutsideTheFolderGetsThePlainLine: a path under no known folder
// has no project to read a convention from, so the line is the plain one.
func TestAFileOutsideTheFolderGetsThePlainLine(t *testing.T) {
	folder := aProjectWith(t, "src/game.js")
	for _, written := range []string{"/game/src/engine.js", "../elsewhere/engine.js"} {
		if got := testsFirstLineFor(folder, written); got != TheTestsFirstLine {
			t.Errorf("writing %q gives the line %q, want the plain line", written, got)
		}
	}
	if got := testsFirstLineFor("", "src/game.js"); got != TheTestsFirstLine {
		t.Errorf("with no folder the line reads %q, want the plain line", got)
	}
}

// TestTheConventionSearchIsBounded: a project of more files than the search
// reads still answers, with the language's default when no test file was
// among the files read.
func TestTheConventionSearchIsBounded(t *testing.T) {
	files := []string{"src/game.js"}
	for at := range MaxFilesReadForATestConvention + 20 {
		files = append(files, filepath.Join("node_modules/pkg", "file"+string(rune('a'+at%26))+".js"))
		files = append(files, filepath.Join("src/parts", "part"+string(rune('a'+at%26))+string(rune('a'+at/26%26))+".js"))
	}
	folder := aProjectWith(t, files...)
	if got := testsFirstLineFor(folder, "src/game.js"); got != "tests first: no failing test covers this change; write it first in src/game.test.js" {
		t.Errorf("the bounded search gives %q", got)
	}
}
