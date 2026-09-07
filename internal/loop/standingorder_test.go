package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestAWorkFoldersAgentsFileReachesTheModelOnEveryCall: the standing order
// of the folder the task works in is read once at the task's start and rides
// in every request the task makes.
func TestAWorkFoldersAgentsFileReachesTheModelOnEveryCall(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		anEditRound(1),
		answerStep("Done. What changed: the board. What I checked: the tests. What is left: nothing."),
	}, scriptedTool(contract.ToolEdit, "edited /game/src/engine.js by 1 line"))
	order := "# The game\n\n- npm test runs the suite.\n- Port 8090 is taken; use 8091.\n"
	if err := os.WriteFile(filepath.Join(built.workFolder, "NERDGENIE.md"), []byte(order), 0o644); err != nil {
		t.Fatalf("cannot write NERDGENIE.md: %v", err)
	}

	built.ask(t, "build the board")

	requests := built.model.Requests()
	if len(requests) < 2 {
		t.Fatalf("the model was called %d times, want at least two", len(requests))
	}
	for at, request := range requests {
		text := wholeRequestText(request)
		if !strings.Contains(text, "The project's standing order (NERDGENIE.md, 4 lines):") || !strings.Contains(text, "Port 8090 is taken; use 8091.") {
			t.Errorf("request %d does not carry the standing order", at)
		}
	}
}

// TestAWorkFolderWithoutAnAgentsFileSaysNothingOfOne holds the other side:
// no file, no block, and the prompt is what it was.
func TestAWorkFolderWithoutAnAgentsFileSaysNothingOfOne(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		answerStep("Nothing to do. What changed: nothing. What I checked: nothing. What is left: nothing."),
	})
	built.ask(t, "look around")
	if strings.Contains(wholeRequestText(built.model.Requests()[0]), "standing order") {
		t.Errorf("a folder with no NERDGENIE.md put a standing order in front of the model")
	}
}
