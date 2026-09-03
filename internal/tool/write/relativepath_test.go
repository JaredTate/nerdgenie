package write_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAWriteOfAShortPathLandsInTheFolderTheAgentWorksIn(t *testing.T) {
	tool, root, _ := newTool(t)

	// This is the call the local model made on the first human trial, which the
	// tool refused because the path does not start at the root of the filesystem.
	output, err := run(t, tool, map[string]any{"path": "haiku.txt", "content": "an old silent pond\n"})
	if err != nil {
		t.Fatalf("the short path every model writes was refused: %v", err)
	}
	held, err := os.ReadFile(filepath.Join(root, "haiku.txt"))
	if err != nil {
		t.Fatalf("the file was not written into the folder the agent works in: %v", err)
	}
	if string(held) != "an old silent pond\n" {
		t.Errorf("the file holds %q, want what the call asked for", held)
	}
	if !strings.Contains(output.Text, filepath.Join(root, "haiku.txt")) {
		t.Errorf("the line the model reads back is %q and does not say where the file went", output.Text)
	}
}

func TestAWriteOfAShortPathThatClimbsOutOfTheFolderIsRefused(t *testing.T) {
	tool, root, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"path": "../secrets.txt", "content": "not here"})
	if err == nil {
		t.Fatalf("a short path that climbs out of the folder the agent works in was written")
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("the refusal reads %q and never says the folder a short path is taken from, which is %s", err, root)
	}
}

func TestAWriteOfAPathUnderTheHomeMarkLandsWhereTheHomeSaysItShould(t *testing.T) {
	tool, root, _ := newTool(t)
	underTheHome := filepath.Join("~", filepath.Base(root), "haiku.txt")

	_, err := run(t, tool, map[string]any{"path": underTheHome, "content": "an old silent pond\n"})
	if err != nil {
		t.Fatalf("the path %q, which a model writes whenever it has seen a shell, was refused: %v", underTheHome, err)
	}
	if _, err := os.Stat(filepath.Join(root, "haiku.txt")); err != nil {
		t.Errorf("the file was not written where the home mark points: %v", err)
	}
}

func TestTheWriteToolTellsTheModelWhereAShortPathIsTakenFrom(t *testing.T) {
	tool, _, _ := newTool(t)

	said := tool.Spec().Description
	for _, field := range tool.Spec().Fields {
		if field.Name == "path" {
			said += " " + field.Description
		}
	}
	if !strings.Contains(said, "folder the agent works in") {
		t.Errorf("the model is told %q, which never says where a path that does not start at the root is taken from", said)
	}
}
