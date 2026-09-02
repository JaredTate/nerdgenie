package testkit_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAScriptedToolReturnsItsOutputsInOrderAndRecordsItsInputs(t *testing.T) {
	ctx := context.Background()
	tool := testkit.NewScriptedTool(contract.ToolSpec{
		Name:        contract.ToolRead,
		Description: "Reads a file, a folder, or a past result by its id.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}, "the first result", "the second result")

	first, err := tool.Run(ctx, json.RawMessage(`{"path":"notes.md"}`))
	if err != nil {
		t.Fatalf("running the tool failed: %v", err)
	}
	second, err := tool.Run(ctx, json.RawMessage(`{"path":"other.md"}`))
	if err != nil {
		t.Fatalf("running the tool a second time failed: %v", err)
	}

	if first.Text != "the first result" || second.Text != "the second result" {
		t.Errorf("the tool returned %q then %q, want its two scripted outputs", first.Text, second.Text)
	}
	if len(tool.Inputs()) != 2 {
		t.Fatalf("the tool recorded %d inputs, want 2", len(tool.Inputs()))
	}
	if string(tool.Inputs()[0]) != `{"path":"notes.md"}` {
		t.Errorf("the tool recorded the first input as %s, want the one it was given", tool.Inputs()[0])
	}
}

func TestAScriptedToolSaysSoWhenItRunsOutOfOutputs(t *testing.T) {
	tool := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolRead, Description: "Reads."}, "only one")

	if _, err := tool.Run(context.Background(), nil); err != nil {
		t.Fatalf("the first run failed: %v", err)
	}
	if _, err := tool.Run(context.Background(), nil); err == nil {
		t.Fatal("running past the end of the script was reported as a success, want an error saying the script ran out")
	}
}

func TestTheFakeRegistryListsWhatItHoldsAndFindsToolsByName(t *testing.T) {
	registry := testkit.NewFakeToolRegistry()
	registry.Add(testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolRead, Description: "Reads."}, "read"))
	registry.Add(testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWrite, Description: "Writes."}, "written"))

	specs := registry.Specs()
	if len(specs) != 2 {
		t.Fatalf("the registry lists %d tools, want 2", len(specs))
	}
	if specs[0].Name != contract.ToolRead {
		t.Errorf("the registry lists %q first, want the order the tools were added", specs[0].Name)
	}
	if _, found := registry.Lookup(contract.ToolWrite); !found {
		t.Error("the registry could not find the write tool that was just added")
	}
	if _, found := registry.Lookup(contract.ToolComputer); found {
		t.Error("the registry found a tool that was never added")
	}
}

func TestTheFakeRegistryKeepsTheToolRegistryContract(t *testing.T) {
	registry := testkit.NewFakeToolRegistry()
	registry.Add(testkit.NewScriptedTool(contract.ToolSpec{
		Name:        contract.ToolRead,
		Description: "Reads a file, a folder, or a past result by its id.",
	}, "read"))

	if err := testkit.CheckToolRegistry(context.Background(), registry); err != nil {
		t.Fatalf("the fake registry does not keep the tool-registry contract: %v", err)
	}
}
