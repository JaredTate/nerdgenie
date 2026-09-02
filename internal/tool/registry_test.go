package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool"
)

// newRegistry builds an empty registry over a temporary home, which is what
// every test in this file starts from.
func newRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	home := testkit.NewTempHome(t)
	registry, err := tool.NewRegistry(tool.Settings{
		Home:          home,
		TaskID:        "17",
		Configuration: contract.DefaultConfig(),
	})
	if err != nil {
		t.Fatalf("cannot build a registry over a temporary home: %v", err)
	}
	return registry
}

// scripted returns a tool named as asked that answers with one line.
func scripted(name string, description string) contract.Tool {
	return testkit.NewScriptedTool(contract.ToolSpec{
		Name:        name,
		Description: description,
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}, "the scripted answer")
}

func TestTheRegistryHoldsWhatWasAddedAndFindsItByName(t *testing.T) {
	registry := newRegistry(t)
	for _, name := range []string{"first", "second"} {
		if err := registry.Add(scripted(name, "Does one thing and says so.")); err != nil {
			t.Fatalf("cannot add the tool %s: %v", name, err)
		}
	}

	specs := registry.Specs()
	if len(specs) != 2 || specs[0].Name != "first" || specs[1].Name != "second" {
		t.Fatalf("the registry lists %v, want the two tools in the order they were added", specs)
	}
	found, held := registry.Lookup("second")
	if !held {
		t.Fatalf("the registry cannot find the tool it was given")
	}
	output, err := found.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("running the tool the registry handed back failed: %v", err)
	}
	if output.Text != "the scripted answer" {
		t.Errorf("the tool returned %q, want the scripted answer unchanged", output.Text)
	}
	if _, found := registry.Lookup("nothing like this"); found {
		t.Errorf("the registry found a tool it was never given")
	}
}

func TestTheRegistryRefusesADescriptionOverTheWordCap(t *testing.T) {
	registry := newRegistry(t)
	tooLong := strings.TrimSpace(strings.Repeat("word ", contract.MaxToolDescriptionWords+1))

	err := registry.Add(scripted("wordy", tooLong))
	if err == nil {
		t.Fatalf("the registry took a description of %d words, and the cap is %d",
			contract.DescriptionWordCount(tooLong), contract.MaxToolDescriptionWords)
	}
	if !strings.Contains(err.Error(), "wordy") {
		t.Errorf("the refusal says %q and does not name the tool", err)
	}
	if !errors.Is(err, tool.ErrDescriptionTooLong) {
		t.Errorf("the refusal is %v, want the named error for a description over the cap", err)
	}
}

func TestTheRegistryRefusesAToolWithNoNameOrNoDescription(t *testing.T) {
	registry := newRegistry(t)
	for _, broken := range []struct {
		what string
		spec contract.ToolSpec
	}{
		{"no name", contract.ToolSpec{Description: "Does one thing and says so."}},
		{"no description", contract.ToolSpec{Name: "quiet"}},
	} {
		err := registry.Add(testkit.NewScriptedTool(broken.spec, "unused"))
		if err == nil {
			t.Errorf("the registry took a tool with %s", broken.what)
		}
	}
}

func TestTheRegistryRefusesTwoToolsUnderOneName(t *testing.T) {
	registry := newRegistry(t)
	if err := registry.Add(scripted("twice", "Does one thing and says so.")); err != nil {
		t.Fatalf("cannot add the first tool: %v", err)
	}
	if err := registry.Add(scripted("twice", "Does another thing and says so.")); err == nil {
		t.Errorf("the registry took two tools under the name twice, and a name must find one tool")
	}
}

func TestTheRegistryRefusesMoreToolsThanTheCap(t *testing.T) {
	registry := newRegistry(t)
	for at := range tool.MaxTools {
		name := "tool" + string(rune('a'+at%26)) + string(rune('a'+at/26))
		if err := registry.Add(scripted(name, "Does one thing and says so.")); err != nil {
			t.Fatalf("cannot add tool number %d: %v", at+1, err)
		}
	}
	if err := registry.Add(scripted("one too many", "Does one thing and says so.")); err == nil {
		t.Errorf("the registry took more than %d tools, and the model has to read every description", tool.MaxTools)
	}
}
