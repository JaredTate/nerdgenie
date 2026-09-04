package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

// writeUserTool puts one executable shell script in the tools folder and returns
// its path.
func writeUserTool(t *testing.T, home contract.Home, name string, script string) string {
	t.Helper()
	path := filepath.Join(home.ToolsFolder(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatalf("cannot write the user tool %s: %v", name, err)
	}
	return path
}

// registryOver builds a registry over a home and keeps every line it logged.
func registryOver(t *testing.T, home contract.Home, lines *[]string) *tool.Registry {
	t.Helper()
	registry, err := tool.NewRegistry(tool.Settings{
		Home:          home,
		TaskID:        "17",
		Configuration: contract.DefaultConfig(),
		Note:          func(line string) { *lines = append(*lines, line) },
	})
	if err != nil {
		t.Fatalf("cannot build a registry over the temporary home: %v", err)
	}
	return registry
}

func TestAUserToolIsDescribedAtStartupAndRunsEndToEnd(t *testing.T) {
	home := testkit.NewTempHome(t)
	writeUserTool(t, home, "weather", `
if [ "$1" = "--describe" ]; then
  echo '{"name":"weather","description":"Reports the weather where the user lives. Do not use it for a forecast anywhere else.","fields":[{"name":"town","type":"string","description":"The town to report on.","required":true}],"classes":["N"]}'
  exit 0
fi
read -r line
echo "the weather in $line"
`)
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("loading the user's own tools failed: %v", err)
	}
	specs := registry.Specs()
	if len(specs) != 1 || specs[0].Name != "weather" {
		t.Fatalf("the registry lists %v, want the one user tool", specs)
	}
	if len(specs[0].Fields) != 1 || specs[0].Fields[0].Name != "town" {
		t.Errorf("the user tool's fields are %v, want the one field it described", specs[0].Fields)
	}

	found, held := registry.Lookup("weather")
	if !held {
		t.Fatalf("the registry cannot find the user tool it registered")
	}
	output, err := found.Run(context.Background(), json.RawMessage(`{"town":"Boise"}`))
	if err != nil {
		t.Fatalf("running the user tool failed: %v", err)
	}
	if !strings.Contains(output.Text, "Boise") {
		t.Errorf("the user tool returned %q, and it was given the input on its standard input", output.Text)
	}
}

func TestAUserToolWithABrokenDescriptionIsSkippedWithOneLoggedLine(t *testing.T) {
	home := testkit.NewTempHome(t)
	path := writeUserTool(t, home, "broken", `echo 'this is not JSON at all'`)
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("one broken user tool stopped the whole load: %v", err)
	}
	if len(registry.Specs()) != 0 {
		t.Errorf("the registry took the broken tool, and it should have been skipped")
	}
	if len(lines) != 1 {
		t.Fatalf("the registry logged %d lines about a broken tool, want exactly one: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], path) {
		t.Errorf("the logged line reads %q and does not name the file %q", lines[0], path)
	}
	if !strings.Contains(lines[0], "JSON") {
		t.Errorf("the logged line reads %q and does not say what the problem was", lines[0])
	}
}

func TestAUserToolWithADescriptionOverTheCapIsSkipped(t *testing.T) {
	home := testkit.NewTempHome(t)
	wordy := strings.TrimSpace(strings.Repeat("word ", contract.MaxToolDescriptionWords+5))
	writeUserTool(t, home, "wordy", `echo '{"name":"wordy","description":"`+wordy+`"}'`)
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("a wordy user tool stopped the whole load: %v", err)
	}
	if len(registry.Specs()) != 0 {
		t.Errorf("the registry took a user tool whose description is over the cap")
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "words") {
		t.Errorf("the registry logged %v, want one line saying the description is too long", lines)
	}
}

func TestAFileInTheToolsFolderThatCannotRunIsSkipped(t *testing.T) {
	home := testkit.NewTempHome(t)
	notes := filepath.Join(home.ToolsFolder(), "NOTES.md")
	if err := os.WriteFile(notes, []byte("a note to myself\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note in the tools folder: %v", err)
	}
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("a file that is not a program stopped the whole load: %v", err)
	}
	if len(registry.Specs()) != 0 {
		t.Errorf("the registry took a file that is not a program")
	}
}

func TestNoToolsFolderIsNotAProblem(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.ToolsFolder()); err != nil {
		t.Fatalf("cannot remove the tools folder: %v", err)
	}
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Errorf("a home with no tools folder was treated as a failure: %v", err)
	}
}

func TestAUserToolThatFailsSaysSoWithWhatItWroteOnItsErrorOutput(t *testing.T) {
	home := testkit.NewTempHome(t)
	writeUserTool(t, home, "grumpy", `
if [ "$1" = "--describe" ]; then
  echo '{"name":"grumpy","description":"Always refuses, which is what the test needs. Do not use it for anything real."}'
  exit 0
fi
echo "the town is not one I know" >&2
exit 3
`)
	lines := []string{}
	registry := registryOver(t, home, &lines)
	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("loading the user's own tools failed: %v", err)
	}

	found, _ := registry.Lookup("grumpy")
	_, err := found.Run(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("a user tool that quit with an error was treated as a success")
	}
	if !strings.Contains(err.Error(), "not one I know") {
		t.Errorf("the failure reads %q and does not say what the tool complained about", err)
	}
}
