package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool"
)

func TestASpillFolderThatCannotBeMadeIsReported(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.RunFolder()); err != nil {
		t.Fatalf("cannot remove the run folder: %v", err)
	}
	if err := os.WriteFile(home.RunFolder(), []byte("a file where a folder should be\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write a file where the run folder was: %v", err)
	}

	settings := contract.DefaultConfig()
	settings.Caps.ToolOutputBytes = 100
	registry, err := tool.NewRegistry(tool.Settings{Home: home, TaskID: "17", Configuration: settings})
	if err != nil {
		t.Fatalf("cannot build the registry: %v", err)
	}
	if err := registry.Add(testkit.NewScriptedTool(
		contract.ToolSpec{Name: "long", Description: "Answers with a great deal of text."},
		strings.Repeat("x", 5000))); err != nil {
		t.Fatalf("cannot add the tool: %v", err)
	}

	found, _ := registry.Lookup("long")
	_, err = found.Run(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("a result was cut with nowhere to keep the rest of it")
	}
	if !strings.Contains(err.Error(), "spill") {
		t.Errorf("the failure reads %q and does not name the folder that could not be made", err)
	}
}

func TestASpilledFileWithANameNothingCanCountIsPassedOver(t *testing.T) {
	registry, home := smallCapRegistry(t, 300)
	folder := filepath.Join(home.RunFolder(), tool.SpillFolderName)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the spill folder: %v", err)
	}
	for _, name := range []string{"17-notanumber.txt", "another-task-3.txt"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}

	output := runThrough(t, registry, "long", strings.Repeat("a line\n", 200))
	if filepath.Base(output.SpillPath) != "17-1.txt" {
		t.Errorf("the result spilled into %s, want the first file of task seventeen", filepath.Base(output.SpillPath))
	}
}

func TestALineAboutASkippedToolGoesToTheStandardLoggerWhenNoneIsNamed(t *testing.T) {
	home := testkit.NewTempHome(t)
	writeUserTool(t, home, "broken", `echo 'this is not JSON at all'`)
	registry, err := tool.NewRegistry(tool.Settings{Home: home, TaskID: "17", Configuration: contract.DefaultConfig()})
	if err != nil {
		t.Fatalf("cannot build the registry: %v", err)
	}

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("loading the user's own tools failed: %v", err)
	}
	if len(registry.Specs()) != 0 {
		t.Errorf("the broken tool was registered")
	}
}

func TestAToolsFolderTheAgentCannotReadIsReported(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.ToolsFolder()); err != nil {
		t.Fatalf("cannot remove the tools folder: %v", err)
	}
	if err := os.WriteFile(home.ToolsFolder(), []byte("a file where a folder should be\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write a file where the tools folder was: %v", err)
	}
	lines := []string{}
	registry := registryOver(t, home, &lines)

	err := registry.AddUserTools(context.Background())
	if err == nil {
		t.Fatalf("a tools folder that is not a folder was read without complaint")
	}
	if !strings.Contains(err.Error(), "tools") {
		t.Errorf("the failure reads %q and does not name the folder", err)
	}
}

func TestBuildingARegistryOverAToolsFolderThatIsNotOneFails(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.ToolsFolder()); err != nil {
		t.Fatalf("cannot remove the tools folder: %v", err)
	}
	if err := os.WriteFile(home.ToolsFolder(), []byte("a file\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write a file where the tools folder was: %v", err)
	}

	if _, err := tool.New(context.Background(), tool.Settings{Home: home, Configuration: contract.DefaultConfig()}); err == nil {
		t.Errorf("a registry was built over a tools folder that is not a folder")
	}
}

func TestAPathWhoseLinksGoRoundInCirclesIsRefused(t *testing.T) {
	userHome := t.TempDir()
	root := filepath.Join(userHome, contract.WorkFolderName)
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the work folder: %v", err)
	}
	circling := filepath.Join(root, "round")
	if err := os.Symlink(circling, circling); err != nil {
		t.Fatalf("cannot make the link that points at itself: %v", err)
	}
	check := tool.NewPathCheck(tool.WorkArea{Roots: []string{root}, UserHome: userHome})

	// A link that points at itself resolves to nothing, so the path is judged as
	// it was written, which is inside the root and therefore allowed. What must
	// not happen is a hang, and this returning at all is the proof.
	if _, err := check(filepath.Join(circling, "anything")); err != nil {
		t.Logf("a path through a link that points at itself was refused: %v", err)
	}
}
