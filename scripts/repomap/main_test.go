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
