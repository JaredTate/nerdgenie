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

func TestASpilledResultThatHoldsMoreThanOneCharacterIsCutOnACharacterBoundary(t *testing.T) {
	registry, _ := smallCapRegistry(t, 300)
	output := runThrough(t, registry, "many-characters", strings.Repeat("é", 400))

	if !strings.Contains(output.Text, "not shown") {
		t.Fatalf("a long result was not cut: %q", output.Text)
	}
	if !isWholeText(output.Text) {
		t.Errorf("the result was cut in the middle of a character")
	}
}

// isWholeText says whether every character in the text is a whole one.
func isWholeText(text string) bool {
	for _, letter := range text {
		if letter == '�' {
			return false
		}
	}
	return true
}

func TestAResultThatCannotBeSpilledIsReportedRatherThanLost(t *testing.T) {
	registry, home := smallCapRegistry(t, 200)
	if err := os.Chmod(home.RunFolder(), 0o500); err != nil {
		t.Fatalf("cannot shut the run folder: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home.RunFolder(), contract.HomeFolderMode) })

	spec := contract.ToolSpec{Name: "long", Description: "Answers with a great deal of text."}
	if err := registry.Add(testkit.NewScriptedTool(spec, strings.Repeat("x", 5000))); err != nil {
		t.Fatalf("cannot add the tool: %v", err)
	}
	found, _ := registry.Lookup("long")
	if _, err := found.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Errorf("a result that could not be written anywhere was handed back as though nothing was lost")
	}
}

func TestAToolThatFailsHandsItsFailureStraightBack(t *testing.T) {
	registry := newRegistry(t)
	spec := contract.ToolSpec{Name: "empty", Description: "Has no answers scripted for it at all."}
	if err := registry.Add(testkit.NewScriptedTool(spec)); err != nil {
		t.Fatalf("cannot add the tool: %v", err)
	}

	found, _ := registry.Lookup("empty")
	if _, err := found.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Errorf("a tool that failed was treated as a tool that answered")
	}
}

func TestTheSpecOfALookedUpToolIsTheToolsOwn(t *testing.T) {
	registry := newRegistry(t)
	if err := registry.Add(scripted("named", "Does one thing and says so.")); err != nil {
		t.Fatalf("cannot add the tool: %v", err)
	}

	found, _ := registry.Lookup("named")
	if found.Spec().Name != "named" || found.Spec().Description != "Does one thing and says so." {
		t.Errorf("the tool the registry handed back describes itself as %+v", found.Spec())
	}
}

func TestAToolClaimingAPermissionClassNobodyKnowsIsRefused(t *testing.T) {
	registry := newRegistry(t)
	spec := contract.ToolSpec{
		Name: "odd", Description: "Claims a class nobody knows.",
		Classes: []contract.PermissionClass{"Q"},
	}

	err := registry.Add(testkit.NewScriptedTool(spec, "unused"))
	if err == nil {
		t.Fatalf("a tool claiming a permission class nobody knows was registered")
	}
	if !strings.Contains(err.Error(), "R, W, X, N, or I") {
		t.Errorf("the refusal reads %q and does not say which classes there are", err)
	}
}

func TestTheCapsFallBackToTheShippedDefaultsWhenTheConfigurationSaysNothing(t *testing.T) {
	home := testkit.NewTempHome(t)
	registry, err := tool.NewRegistry(tool.Settings{Home: home})
	if err != nil {
		t.Fatalf("cannot build a registry with no caps configured: %v", err)
	}
	if err := registry.Add(testkit.NewScriptedTool(
		contract.ToolSpec{Name: "short", Description: "Answers briefly."}, "a short answer")); err != nil {
		t.Fatalf("cannot add the tool: %v", err)
	}

	found, _ := registry.Lookup("short")
	output, err := found.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("running a tool with no caps configured failed: %v", err)
	}
	if output.SpillPath != "" {
		t.Errorf("a short answer spilled to %q under the shipped default cap", output.SpillPath)
	}
}

func TestAShellCommandRunsInTheFirstFolderTheAgentMayWorkInWhenNoneIsNamed(t *testing.T) {
	settings, _ := wholeSettings(t)
	work := settings.Configuration.SandboxRoots[0]
	settings.WorkingDirectory = ""
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script("/bin/sh -c", contract.SandboxResult{StandardOutput: []byte("done\n")})
	settings.Sandbox = sandbox

	registry, err := tool.New(context.Background(), settings)
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}
	found, _ := registry.Lookup(contract.ToolShell)
	if _, err := found.Run(context.Background(), json.RawMessage(`{"command":"echo done"}`)); err != nil {
		t.Fatalf("running a command failed: %v", err)
	}
	commands := sandbox.Commands()
	if len(commands) != 1 || commands[0].WorkingDirectory != work {
		t.Errorf("the command ran in %q, want the first folder the agent may work in, %q",
			commands[0].WorkingDirectory, work)
	}
}

func TestAUserToolThatOutlivesItsTimeoutIsStopped(t *testing.T) {
	home := testkit.NewTempHome(t)
	writeUserTool(t, home, "slow", `
if [ "$1" = "--describe" ]; then
  echo '{"name":"slow","description":"Never finishes, which is what this test needs. Do not use it for anything real."}'
  exit 0
fi
sleep 30
`)
	settings := contract.DefaultConfig()
	settings.Caps.TimePerTool = 200 * time.Millisecond
	lines := []string{}
	registry, err := tool.NewRegistry(tool.Settings{
		Home: home, TaskID: "17", Configuration: settings,
		Note: func(line string) { lines = append(lines, line) },
	})
	if err != nil {
		t.Fatalf("cannot build the registry: %v", err)
	}
	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("loading the user's own tools failed: %v", err)
	}

	found, held := registry.Lookup("slow")
	if !held {
		t.Fatalf("the registry did not register the slow tool: %v", lines)
	}
	started := time.Now()
	if _, err := found.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Errorf("a tool that never finishes was waited out")
	}
	if time.Since(started) > 25*time.Second {
		t.Errorf("stopping the tool took %s, and its timeout is %s", time.Since(started), settings.Caps.TimePerTool)
	}
}

func TestAFolderInTheToolsFolderIsPassedOver(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.MkdirAll(filepath.Join(home.ToolsFolder(), "a-folder"), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder in the tools folder: %v", err)
	}
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("a folder in the tools folder stopped the whole load: %v", err)
	}
	if len(registry.Specs()) != 0 || len(lines) != 0 {
		t.Errorf("a folder in the tools folder was treated as a tool: %v, %v", registry.Specs(), lines)
	}
}
