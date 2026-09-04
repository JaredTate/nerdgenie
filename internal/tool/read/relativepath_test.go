package read_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestAReadOfAShortPathFindsTheFileInTheFolderTheAgentWorksIn(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	if err := os.WriteFile(filepath.Join(root, "haiku.txt"), []byte("an old silent pond\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file to read: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": "haiku.txt"})
	if err != nil {
		t.Fatalf("the short path every model writes was refused: %v", err)
	}
	if !strings.Contains(output.Text, "an old silent pond") {
		t.Errorf("the read came back with %q, want the file in the folder the agent works in", output.Text)
	}
}

func TestAReadOfAShortPathThatClimbsOutOfTheFolderIsRefused(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	secret := aSecretOutside(t, root)

	output, err := run(t, tool, map[string]any{"path": filepath.Join("..", filepath.Base(secret))})
	if err == nil {
		t.Fatalf("a short path that climbs out of the folder the agent works in was read as %q", output.Text)
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("the refusal reads %q and never says the folder a short path is taken from, which is %s", err, root)
	}
}

func TestAReadOfAPathUnderTheHomeMarkFindsTheFile(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	if err := os.WriteFile(filepath.Join(root, "haiku.txt"), []byte("an old silent pond\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file to read: %v", err)
	}
	underTheHome := filepath.Join("~", filepath.Base(root), "haiku.txt")

	output, err := run(t, tool, map[string]any{"path": underTheHome})
	if err != nil {
		t.Fatalf("the path %q, which a model writes whenever it has seen a shell, was refused: %v", underTheHome, err)
	}
	if !strings.Contains(output.Text, "an old silent pond") {
		t.Errorf("the read came back with %q, want the file the home mark points at", output.Text)
	}
}

func TestAReadOfAPathUnderTheHomeButOutsideTheRootsSaysHowTheMarkWasRead(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	userHome := filepath.Dir(root)

	// This is the path the local model wrote on the first human trial.
	_, err := run(t, tool, map[string]any{"path": "~/Desktop/Tater Tots Tetrisv1/src/config.js"})
	if err == nil {
		t.Fatalf("a path under the home but outside every folder the agent may work in was read")
	}
	if !strings.Contains(err.Error(), "~") || !strings.Contains(err.Error(), userHome) {
		t.Errorf("the refusal reads %q and does not say that ~ is read as the user's home folder, %s", err, userHome)
	}
}

func TestTheReadToolTellsTheModelWhereAShortPathIsTakenFrom(t *testing.T) {
	tool, _ := newTool(t, nil, nil)

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
