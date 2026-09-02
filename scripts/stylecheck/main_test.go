package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithNoArgumentsSaysHowToUseIt(t *testing.T) {
	var output, problems bytes.Buffer

	code := run(nil, &output, &problems)

	if code != 2 {
		t.Errorf("running with no arguments returned %d, want 2 for a wrong command line", code)
	}
	if !strings.Contains(problems.String(), "usage:") {
		t.Errorf("running with no arguments printed %q, want a usage line", problems.String())
	}
}

func TestRunOnACleanFolderPrintsNothingAndSucceeds(t *testing.T) {
	folder := t.TempDir()
	writeGoFile(t, filepath.Join(folder, "doc.go"), "// Package example is clean.\npackage example\n")

	var output, problems bytes.Buffer
	code := run([]string{folder + "/..."}, &output, &problems)

	if code != 0 {
		t.Errorf("checking a clean folder returned %d, want 0. It printed:\n%s%s", code, output.String(), problems.String())
	}
	if output.Len() != 0 {
		t.Errorf("checking a clean folder printed %q, want nothing", output.String())
	}
}

func TestRunOnAFolderWithAViolationPrintsItAndFails(t *testing.T) {
	folder := t.TempDir()
	writeGoFile(t, filepath.Join(folder, "doc.go"), "// Package example holds an undocumented name.\npackage example\n")
	writeGoFile(t, filepath.Join(folder, "bad.go"), "package example\n\nfunc Count() int { return 0 }\n")

	var output, problems bytes.Buffer
	code := run([]string{folder}, &output, &problems)

	if code != 1 {
		t.Errorf("checking a folder with a violation returned %d, want 1", code)
	}
	if !strings.Contains(output.String(), "doc-comment") {
		t.Errorf("the violation printed as %q, want the rule name in it", output.String())
	}
	if !strings.Contains(output.String(), "1 style violations") {
		t.Errorf("the summary line is missing from %q", output.String())
	}
}

func TestRunOnAFolderThatIsNotThereSaysSo(t *testing.T) {
	var output, problems bytes.Buffer

	code := run([]string{filepath.Join(t.TempDir(), "nowhere")}, &output, &problems)

	if code != 1 {
		t.Errorf("checking a folder that is not there returned %d, want 1", code)
	}
	if problems.Len() == 0 {
		t.Error("checking a folder that is not there printed nothing, want an error naming the folder")
	}
}

func TestFolderOfTurnsAPatternIntoAFolder(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"./...", "."},
		{".", "."},
		{"...", "."},
		{"internal/...", "internal"},
		{"internal/lint", "internal/lint"},
	}
	for _, test := range tests {
		if got := folderOf(test.pattern); got != test.want {
			t.Errorf("folderOf(%q) is %q, want %q", test.pattern, got, test.want)
		}
	}
}

// writeGoFile writes one Go file for a test, failing the test when it cannot.
func writeGoFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}
