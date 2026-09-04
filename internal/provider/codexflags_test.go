package provider

import (
	"slices"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheCodexProgramsOwnToolsAreSwitchedOff holds what the live suite found:
// with codex's own shell tool on, GPT sometimes wrote the file itself inside
// the program's read-only sandbox instead of asking for the harness's write
// tool, so the write never happened and never went past the permission
// function or the log. The claude side already runs with no tools of its own.
func TestTheCodexProgramsOwnToolsAreSwitchedOff(t *testing.T) {
	model := &commandLineModel{alias: contract.ModelAlias{Name: "codex", ModelName: "gpt-5.5", Program: "codex"}}
	arguments, err := model.codexArguments(t.TempDir(), "the instructions", contract.ThinkDefault)
	if err != nil {
		t.Fatalf("building the codex command line failed: %v", err)
	}
	for _, feature := range []string{"shell_tool", "unified_exec"} {
		at := slices.Index(arguments, feature)
		if at < 1 || arguments[at-1] != "--disable" {
			t.Errorf("the codex command line does not switch off %s: %v", feature, arguments)
		}
	}
}
