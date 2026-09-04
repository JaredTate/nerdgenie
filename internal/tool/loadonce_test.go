package tool_test

import (
	"context"
	"os"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

func TestTheUserToolsAreAskedOnceAndSharedByEveryRegistryAfterwards(t *testing.T) {
	home := testkit.NewTempHome(t)
	tally := describingTools(t, home, 3, true)
	settings := tool.Settings{Home: home, TaskID: "17", Configuration: contract.DefaultConfig(), Note: func(string) {}}

	asked, err := tool.LoadUserTools(context.Background(), settings)
	if err != nil {
		t.Fatalf("asking the tools folder what it holds failed: %v", err)
	}
	if len(asked) != 3 {
		t.Fatalf("the folder answered with %d tools, want the three in it", len(asked))
	}

	settings.UserTools = asked
	for task := 0; task < 4; task++ {
		registry, err := tool.NewRegistry(settings)
		if err != nil {
			t.Fatalf("cannot build the registry for task %d: %v", task, err)
		}
		if err := registry.AddUserTools(context.Background()); err != nil {
			t.Fatalf("adding the tools the program already asked for failed: %v", err)
		}
		if len(registry.Specs()) != 3 {
			t.Fatalf("registry %d holds %d tools, want the three the program asked for once", task, len(registry.Specs()))
		}
	}
	if runs := timesAsked(t, tally); runs != 3 {
		t.Errorf("the three programs were run %d times in all, want three: one registry per task must not ask them again", runs)
	}
}

func TestAHomeWithNoToolsFolderAnswersWithAnEmptyListRatherThanNone(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.ToolsFolder()); err != nil {
		t.Fatalf("cannot remove the tools folder: %v", err)
	}

	asked, err := tool.LoadUserTools(context.Background(), tool.Settings{Home: home, Configuration: contract.DefaultConfig()})
	if err != nil {
		t.Fatalf("a home with no tools folder was treated as a failure: %v", err)
	}
	if asked == nil {
		t.Errorf("the answer was nothing at all, and a task would then read the folder again for itself")
	}
	if len(asked) != 0 {
		t.Errorf("a home with no tools folder answered with %d tools", len(asked))
	}
}
