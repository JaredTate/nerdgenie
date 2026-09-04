package tool_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

// wholeSettings wires every fake in, which is what the program does for real
// when it starts, and returns the settings and the home behind them.
func wholeSettings(t *testing.T) (tool.Settings, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	userHome := filepath.Dir(home.Root)
	work := filepath.Join(userHome, contract.WorkFolderName)
	if err := os.MkdirAll(work, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the work folder: %v", err)
	}
	settings := contract.DefaultConfig()
	settings.SandboxRoots = []string{work}

	return tool.Settings{
		Configuration:    settings,
		Home:             home,
		UserHome:         userHome,
		TaskID:           "17",
		Log:              testkit.NewFakeStore(),
		Sandbox:          testkit.NewFakeSandbox(),
		Permission:       testkit.NewFakePermission(contract.RulingAllow),
		Memory:           testkit.NewFakeMemory(),
		Skills:           testkit.NewFakeSkill(),
		Jobs:             testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(1700000000, 0).UTC())),
		Browser:          testkit.NewFakeBrowserWorker(),
		Desktop:          testkit.NewFakeDesktop(),
		Clock:            testkit.NewFakeClock(time.Unix(1700000000, 0).UTC()),
		WorkingDirectory: work,
	}, home
}

func TestTheRegistryHoldsTheEighteenBuiltInTools(t *testing.T) {
	settings, _ := wholeSettings(t)
	registry, err := tool.New(context.Background(), settings)
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	specs := registry.Specs()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	wanted := contract.BuiltInToolNames()
	if strings.Join(names, ",") != strings.Join(wanted, ",") {
		t.Fatalf("the registry holds %v, want the eighteen built-in tools in the order the design lists them: %v", names, wanted)
	}
	for _, name := range wanted {
		if _, held := registry.Lookup(name); !held {
			t.Errorf("the registry cannot find the built-in tool %q", name)
		}
	}
}

func TestEveryBuiltInDescriptionFitsInTheCapAndSaysWhenNotToUseTheTool(t *testing.T) {
	settings, _ := wholeSettings(t)
	registry, err := tool.New(context.Background(), settings)
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	written := &strings.Builder{}
	for _, spec := range registry.Specs() {
		words := contract.DescriptionWordCount(spec.Description)
		if words > contract.MaxToolDescriptionWords {
			t.Errorf("the description of %q is %d words, and the cap is %d", spec.Name, words, contract.MaxToolDescriptionWords)
		}
		if len(spec.Classes) == 0 {
			t.Errorf("the tool %q claims no permission class, and every tool has at least one", spec.Name)
		}
		fmt.Fprintf(written, "%-18s %2d words  %s\n", spec.Name, words, spec.Description)
	}
	testkit.Golden(t, "the_eighteen_descriptions.txt", []byte(written.String()))
}

func TestTheUsersOwnToolsAreAddedAfterTheBuiltInOnes(t *testing.T) {
	settings, home := wholeSettings(t)
	writeUserTool(t, home, "weather", `
if [ "$1" = "--describe" ]; then
  echo '{"name":"weather","description":"Reports the weather where the user lives. Do not use it for a forecast anywhere else."}'
  exit 0
fi
echo "it is raining"
`)
	registry, err := tool.New(context.Background(), settings)
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	specs := registry.Specs()
	if len(specs) != len(contract.BuiltInToolNames())+1 {
		t.Fatalf("the registry holds %d tools, want the eighteen and the user's one", len(specs))
	}
	if specs[len(specs)-1].Name != "weather" {
		t.Errorf("the last tool is %q, and the user's own tools come after the built-in ones", specs[len(specs)-1].Name)
	}
}

func TestARegistryWithNothingWiredInStillShowsEveryToolAndRefusesPlainly(t *testing.T) {
	home := testkit.NewTempHome(t)
	registry, err := tool.New(context.Background(), tool.Settings{Home: home, Configuration: contract.DefaultConfig()})
	if err != nil {
		t.Fatalf("building a registry with nothing wired in failed: %v", err)
	}

	if len(registry.Specs()) != len(contract.BuiltInToolNames()) {
		t.Fatalf("the registry holds %d tools, and every model sees all eighteen on every call", len(registry.Specs()))
	}
	found, held := registry.Lookup(contract.ToolMemory)
	if !held {
		t.Fatalf("the registry cannot find the memory tool")
	}
	if _, err := found.Run(context.Background(), []byte(`{"action":"search","query":"anything"}`)); err == nil {
		t.Errorf("a tool with nothing behind it ran instead of saying what was missing")
	}
}

func TestTheRegistryNeedsAHomeFolder(t *testing.T) {
	if _, err := tool.New(context.Background(), tool.Settings{}); err == nil {
		t.Errorf("a registry was built with no home folder to spill into")
	}
}

// recordOfAsk is the record of a running task as the registry's tools read it:
// it holds one ask and takes no writes.
type recordOfAsk struct {
	ask string
}

// Record returns a record carrying the held ask.
func (held recordOfAsk) Record() contract.Record {
	return contract.Record{Goal: contract.Goal{Ask: held.ask}}
}

// Apply is never called in these tests.
func (recordOfAsk) Apply(context.Context, record.Update) error { return nil }

// TestTheJobToolIsHandedTheRunningTasksRecord pins the wiring: the registry
// builds the job tool over the same record the task tool writes through, so a
// create that names no ask carries the running task's ask word for word.
func TestTheJobToolIsHandedTheRunningTasksRecord(t *testing.T) {
	settings, _ := wholeSettings(t)
	settings.Records = recordOfAsk{ask: "build the whole game, every feature the user listed"}
	registry, err := tool.New(context.Background(), settings)
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}

	found, held := registry.Lookup(contract.ToolJob)
	if !held {
		t.Fatalf("the registry cannot find the job tool")
	}
	if _, err := found.Run(context.Background(), []byte(`{"action":"create","name":"The Game","why":"the user wants it","text":"the first task"}`)); err != nil {
		t.Fatalf("creating a job with no ask through the registry was refused: %v", err)
	}
	loaded, err := settings.Jobs.Load(context.Background(), "1")
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if loaded.Goal.Ask != "build the whole game, every feature the user listed" {
		t.Errorf("the job's ask is %q, want the running task's, which the registry hands the job tool", loaded.Goal.Ask)
	}
}
