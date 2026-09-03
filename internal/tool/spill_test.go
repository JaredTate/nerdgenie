package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool"
)

// smallCapRegistry builds a registry whose output cap is small enough that a
// paragraph overflows it, and returns the registry and the home behind it.
func smallCapRegistry(t *testing.T, outputCap int) (*tool.Registry, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	settings := contract.DefaultConfig()
	settings.Caps.ToolOutputBytes = outputCap
	registry, err := tool.NewRegistry(tool.Settings{Home: home, TaskID: "17", Configuration: settings})
	if err != nil {
		t.Fatalf("cannot build a registry with an output cap of %d: %v", outputCap, err)
	}
	return registry, home
}

// runThrough adds a tool that answers with the text given and runs it through
// the registry, which is where the output cap is applied.
func runThrough(t *testing.T, registry *tool.Registry, name string, text string) contract.ToolOutput {
	t.Helper()
	spec := contract.ToolSpec{Name: name, Description: "Answers with what the test scripted."}
	if err := registry.Add(testkit.NewScriptedTool(spec, text)); err != nil {
		t.Fatalf("cannot add the tool %s: %v", name, err)
	}
	found, held := registry.Lookup(name)
	if !held {
		t.Fatalf("the registry cannot find the tool %s it was just given", name)
	}
	output, err := found.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("running the tool %s failed: %v", name, err)
	}
	return output
}

func TestAResultInsideTheCapComesBackUntouched(t *testing.T) {
	registry, _ := smallCapRegistry(t, 500)
	output := runThrough(t, registry, "short", "a line and nothing more")

	if output.Text != "a line and nothing more" {
		t.Errorf("the result reads %q, want the tool's own text unchanged", output.Text)
	}
	if output.SpillPath != "" {
		t.Errorf("a result inside the cap spilled to %q, and nothing should have been written", output.SpillPath)
	}
}

func TestAResultOverTheCapSpillsToAFileTheResultNames(t *testing.T) {
	registry, home := smallCapRegistry(t, 400)
	whole := strings.Repeat("a line of the result\n", 200)
	output := runThrough(t, registry, "long", whole)

	if len(output.Text) > 400 {
		t.Errorf("the result the model sees is %d bytes, and the cap is %d", len(output.Text), 400)
	}
	if output.SpillPath == "" {
		t.Fatalf("a result of %d bytes was cut to %d and named no file to read the rest from", len(whole), len(output.Text))
	}
	if !strings.Contains(output.Text, output.SpillPath) {
		t.Errorf("the result reads %q and does not name the file %q holding the rest", output.Text, output.SpillPath)
	}
	spilled, err := os.ReadFile(output.SpillPath)
	if err != nil {
		t.Fatalf("cannot read the spilled file the result named: %v", err)
	}
	if string(spilled) != whole {
		t.Errorf("the spilled file holds %d bytes, want the whole result of %d bytes", len(spilled), len(whole))
	}
	wanted := filepath.Join(home.RunFolder(), "spill")
	if filepath.Dir(output.SpillPath) != wanted {
		t.Errorf("the result spilled into %s, want the spill folder %s", filepath.Dir(output.SpillPath), wanted)
	}
	if !strings.Contains(filepath.Base(output.SpillPath), "17") {
		t.Errorf("the spilled file is named %s and does not name the task it belongs to", filepath.Base(output.SpillPath))
	}
}

func TestTwoSpilledResultsGetFilesOfTheirOwn(t *testing.T) {
	registry, _ := smallCapRegistry(t, 400)
	first := runThrough(t, registry, "first", strings.Repeat("one\n", 300))
	second := runThrough(t, registry, "second", strings.Repeat("two\n", 300))

	if first.SpillPath == second.SpillPath {
		t.Fatalf("both results spilled into %s, and one file cannot hold two results", first.SpillPath)
	}
	held, err := os.ReadFile(second.SpillPath)
	if err != nil {
		t.Fatalf("cannot read the second spilled file: %v", err)
	}
	if !strings.HasPrefix(string(held), "two") {
		t.Errorf("the second spilled file starts %q, want the second result", string(held[:3]))
	}
}

func TestTheSpillFolderDropsItsOldestFilesOnceItIsFull(t *testing.T) {
	registry, home := smallCapRegistry(t, 400)
	folder := filepath.Join(home.RunFolder(), "spill")
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the spill folder: %v", err)
	}
	stale := filepath.Join(folder, "old.txt")
	if err := os.WriteFile(stale, make([]byte, tool.MaxSpillFolderBytes), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file that fills the spill folder: %v", err)
	}
	if err := os.Chtimes(stale, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatalf("cannot age the file that fills the spill folder: %v", err)
	}

	runThrough(t, registry, "long", strings.Repeat("a line of the result\n", 200))

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("the oldest file is still in the spill folder, which is over its cap of %d bytes", tool.MaxSpillFolderBytes)
	}
}

func TestAResultTooBigToSpillIsCutBeforeItIsWritten(t *testing.T) {
	registry, _ := smallCapRegistry(t, 400)
	output := runThrough(t, registry, "enormous", strings.Repeat("x", tool.MaxResultBytes+1000))

	spilled, err := os.ReadFile(output.SpillPath)
	if err != nil {
		t.Fatalf("cannot read the spilled file: %v", err)
	}
	if len(spilled) > tool.MaxResultBytes {
		t.Errorf("the spilled file holds %d bytes, and one result is capped at %d", len(spilled), tool.MaxResultBytes)
	}
}
