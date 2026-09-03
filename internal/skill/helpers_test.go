package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/testkit"
)

// startOfTheTests is the time every fake clock in these tests starts at, so
// that a changelog entry a test reads is the same every time it runs.
var startOfTheTests = time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)

// harness is one skill store with everything it was built from, so that a test
// can look at what the store told the permission function and the screen.
type harness struct {
	store      *skill.Store
	home       contract.Home
	clock      *testkit.FakeClock
	tools      *testkit.FakeToolRegistry
	permission *testkit.FakePermission
	channel    *testkit.FakeChannel
}

// newHarness builds a store over a temporary home, with fakes for everything
// outside it and the channel wired to the function that asks the user.
func newHarness(t *testing.T, tools ...contract.Tool) *harness {
	t.Helper()
	built := &harness{
		home:       testkit.NewTempHome(t),
		clock:      testkit.NewFakeClock(startOfTheTests),
		tools:      testkit.NewFakeToolRegistry(tools...),
		permission: testkit.NewFakePermission(contract.RulingAllow),
		channel:    testkit.NewFakeChannel("terminal"),
	}
	store, err := skill.New(skill.Options{
		Home:       built.home,
		Clock:      built.clock,
		Tools:      built.tools,
		Permission: built.permission,
		Ask:        built.channel.ShowPreview,
	})
	if err != nil {
		t.Fatalf("cannot build the skill store: %v", err)
	}
	built.store = store
	return built
}

// writeSkill copies one of the folders under testdata into the temporary home's
// skills folder, under the name given.
func (built *harness) writeSkill(t *testing.T, from string, name string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatalf("cannot read the test folder %s: %v", from, err)
	}
	into := built.home.SkillFolder(name)
	if err := os.MkdirAll(into, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder %s: %v", into, err)
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(from, entry.Name()))
		if err != nil {
			t.Fatalf("cannot read %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(into, entry.Name()), content, contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", entry.Name(), err)
		}
	}
}

// writeFiles writes one skill folder from the files given, without going
// through Save, which is how a test builds a folder that Save would have
// refused.
func (built *harness) writeFiles(t *testing.T, name string, files map[string]string) {
	t.Helper()
	into := built.home.SkillFolder(name)
	if err := os.MkdirAll(into, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder %s: %v", into, err)
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(into, file), []byte(content), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", file, err)
		}
	}
}

// readSkillFile reads one file out of a skill folder in the temporary home.
func (built *harness) readSkillFile(t *testing.T, name string, file string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(built.home.SkillFolder(name), file))
	if err != nil {
		t.Fatalf("cannot read the %s of the skill %q: %v", file, name, err)
	}
	return string(content)
}

// echoTool is a tool that hands back whatever its "say" field holds, which is
// enough for a test to prove that a step ran and what it saw.
type echoTool struct {
	name  string
	calls int
}

// Spec is what the model would be told about the echo tool.
func (tool *echoTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: tool.name, Description: "Says back whatever it is given, for a test"}
}

// failingTool is a tool that always goes wrong, which is how a test sees what a
// replay says when a step's tool fails rather than returning the wrong thing.
type failingTool struct {
	name string
}

// Spec is what the model would be told about the failing tool.
func (tool *failingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: tool.name, Description: "Always goes wrong, for a test"}
}

// Run always fails, saying so in the words a tool would use.
func (tool *failingTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	return contract.ToolOutput{}, errors.New("this tool goes wrong every time it is asked")
}

// Run hands back the "say" field, or the whole input when there is none.
func (tool *echoTool) Run(_ context.Context, input json.RawMessage) (contract.ToolOutput, error) {
	tool.calls++
	fields := map[string]any{}
	if err := json.Unmarshal(input, &fields); err == nil {
		if said, held := fields["say"].(string); held {
			return contract.ToolOutput{Text: said}, nil
		}
	}
	return contract.ToolOutput{Text: string(input)}, nil
}
