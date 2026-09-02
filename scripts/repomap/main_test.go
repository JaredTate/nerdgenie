package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsTheMapForTheRepositoryItIsGiven(t *testing.T) {
	root := newFixtureRepository(t, "README.md")

	var output, problems bytes.Buffer
	code := run([]string{"--root", root}, &output, &problems)

	if code != 0 {
		t.Fatalf("running the generator returned %d, want 0. It printed:\n%s", code, problems.String())
	}
	if !strings.Contains(output.String(), "README.md") {
		t.Errorf("the printed map does not list README.md:\n%s", output.String())
	}
}

func TestRunWithNoRootUsesTheCurrentFolder(t *testing.T) {
	var output, problems bytes.Buffer

	code := run(nil, &output, &problems)

	if code != 0 {
		t.Fatalf("running the generator with no arguments returned %d, want 0. It printed:\n%s", code, problems.String())
	}
	if !strings.Contains(output.String(), "# Repository Map") {
		t.Errorf("the printed map has no heading:\n%s", output.String())
	}
	// This test runs in scripts/repomap, so the generator's own files are at the
	// top of the tree. A run that used the folder above would list them under
	// "repomap/" instead.
	tree := treeOf(t, output.String())
	if !strings.Contains(tree, "\ngenerate.go\n") {
		t.Errorf("the map does not list generate.go at the top of the tree, so it was not made for this folder:\n%s", tree)
	}
	if strings.Contains(tree, "repomap/generate.go") {
		t.Errorf("the map lists repomap/generate.go, so it was made for the folder above this one:\n%s", tree)
	}
}

func TestRunSaysSoWhenTheRootIsNotThere(t *testing.T) {
	var output, problems bytes.Buffer

	code := run([]string{"--root", filepath.Join(t.TempDir(), "nowhere")}, &output, &problems)

	if code != 1 {
		t.Errorf("running the generator on a folder that is not there returned %d, want 1", code)
	}
	if problems.Len() == 0 {
		t.Error("running the generator on a folder that is not there printed nothing, want an error naming it")
	}
}

// closedOutput stands in for a pipe whose other end has already gone away.
type closedOutput struct{}

// Write always fails, the way a write to a closed pipe does.
func (closedOutput) Write([]byte) (int, error) {
	return 0, errClosedOutput
}

// errClosedOutput is what a closed pipe reports.
var errClosedOutput = closedPipeError{}

// closedPipeError is the error a closed pipe reports.
type closedPipeError struct{}

// Error says what went wrong and what to do about it.
func (closedPipeError) Error() string {
	return "the output is closed, so send the map somewhere that is still open"
}

func TestRunSaysSoWhenItCannotPrintTheMap(t *testing.T) {
	root := newFixtureRepository(t, "README.md")

	var problems bytes.Buffer
	code := run([]string{"--root", root}, closedOutput{}, &problems)

	if code != 1 {
		t.Errorf("printing to a closed output returned %d, want 1", code)
	}
	if problems.Len() == 0 {
		t.Error("printing to a closed output said nothing, want an error saying what went wrong")
	}
}

func TestRunSaysHowToUseItWhenTheArgumentsAreWrong(t *testing.T) {
	tests := [][]string{
		{"--root"},
		{"--nonsense"},
	}
	for _, arguments := range tests {
		var output, problems bytes.Buffer
		if code := run(arguments, &output, &problems); code != 2 {
			t.Errorf("running the generator with %v returned %d, want 2 for a wrong command line", arguments, code)
		}
		if !strings.Contains(problems.String(), "usage:") {
			t.Errorf("running the generator with %v printed %q, want a usage line", arguments, problems.String())
		}
	}
}
