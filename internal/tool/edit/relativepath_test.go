package edit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestAnEditOfAShortPathChangesTheFileInTheFolderTheAgentWorksIn(t *testing.T) {
	tool, root, _ := newTool(t)
	path := filepath.Join(root, "haiku.txt")
	if err := os.WriteFile(path, []byte("an old silent pond\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file to edit: %v", err)
	}

	_, err := run(t, tool, map[string]any{"path": "haiku.txt", "old": "silent", "new": "quiet"})
	if err != nil {
		t.Fatalf("the short path every model writes was refused: %v", err)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file back: %v", err)
	}
	if string(held) != "an old quiet pond\n" {
		t.Errorf("the file holds %q, want the edit the call asked for", held)
	}
}

func TestAnEditOfAShortPathThatClimbsOutOfTheFolderIsRefused(t *testing.T) {
	tool, root, _ := newTool(t)
	secret := aSecretOutside(t, root)

	_, err := run(t, tool, map[string]any{"path": filepath.Join("..", filepath.Base(secret)), "old": "hunter2", "new": "taken"})
	if err == nil {
		t.Fatalf("a short path that climbs out of the folder the agent works in was edited")
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("the refusal reads %q and never says the folder a short path is taken from, which is %s", err, root)
	}
}

func TestAnEditOfAPathUnderTheHomeMarkChangesTheFileTheMarkPointsAt(t *testing.T) {
	tool, root, _ := newTool(t)
	path := filepath.Join(root, "haiku.txt")
	if err := os.WriteFile(path, []byte("an old silent pond\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file to edit: %v", err)
	}
	underTheHome := filepath.Join("~", filepath.Base(root), "haiku.txt")

	_, err := run(t, tool, map[string]any{"path": underTheHome, "old": "silent", "new": "quiet"})
	if err != nil {
		t.Fatalf("the path %q, which a model writes whenever it has seen a shell, was refused: %v", underTheHome, err)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file back: %v", err)
	}
	if string(held) != "an old quiet pond\n" {
		t.Errorf("the file holds %q, want the edit the call asked for", held)
	}
}

func TestTheEditToolTellsTheModelWhereAShortPathIsTakenFrom(t *testing.T) {
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
