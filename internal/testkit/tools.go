package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ScriptedTool returns the outputs a test gave it, one per call, and records
// every input it was given.
type ScriptedTool struct {
	guard   sync.Mutex
	spec    contract.ToolSpec
	outputs []string
	used    int
	inputs  []json.RawMessage
}

// NewScriptedTool returns a tool that answers with the outputs in order.
func NewScriptedTool(spec contract.ToolSpec, outputs ...string) *ScriptedTool {
	return &ScriptedTool{spec: spec, outputs: outputs}
}

// Spec is what the model is told about this tool.
func (tool *ScriptedTool) Spec() contract.ToolSpec {
	return tool.spec
}

// Run returns the next scripted output, and fails clearly when there are none
// left, because a test that runs past its script is testing nothing.
func (tool *ScriptedTool) Run(_ context.Context, input json.RawMessage) (contract.ToolOutput, error) {
	tool.guard.Lock()
	defer tool.guard.Unlock()
	tool.inputs = append(tool.inputs, input)

	if tool.used >= len(tool.outputs) {
		return contract.ToolOutput{}, fmt.Errorf("the script for the tool %q has %d outputs and this is call %d, so add another output",
			tool.spec.Name, len(tool.outputs), tool.used+1)
	}
	text := tool.outputs[tool.used]
	tool.used++
	return contract.ToolOutput{Text: text}, nil
}

// Inputs is every input the tool was given, in order.
func (tool *ScriptedTool) Inputs() []json.RawMessage {
	tool.guard.Lock()
	defer tool.guard.Unlock()
	copied := make([]json.RawMessage, len(tool.inputs))
	copy(copied, tool.inputs)
	return copied
}

// FakeToolRegistry holds the tools a test added, in the order they were added.
type FakeToolRegistry struct {
	guard sync.Mutex
	order []string
	tools map[string]contract.Tool
}

// NewFakeToolRegistry returns a registry holding the tools given.
func NewFakeToolRegistry(tools ...contract.Tool) *FakeToolRegistry {
	registry := &FakeToolRegistry{tools: map[string]contract.Tool{}}
	for _, tool := range tools {
		registry.Add(tool)
	}
	return registry
}

// Add puts one tool in the registry.
func (registry *FakeToolRegistry) Add(tool contract.Tool) {
	registry.guard.Lock()
	defer registry.guard.Unlock()
	name := tool.Spec().Name
	if _, held := registry.tools[name]; !held {
		registry.order = append(registry.order, name)
	}
	registry.tools[name] = tool
}

// Specs is what goes into the model's system prompt, in the order the tools were
// added.
func (registry *FakeToolRegistry) Specs() []contract.ToolSpec {
	registry.guard.Lock()
	defer registry.guard.Unlock()
	specs := make([]contract.ToolSpec, 0, len(registry.order))
	for _, name := range registry.order {
		specs = append(specs, registry.tools[name].Spec())
	}
	return specs
}

// Lookup finds one tool by name.
func (registry *FakeToolRegistry) Lookup(name string) (contract.Tool, bool) {
	registry.guard.Lock()
	defer registry.guard.Unlock()
	tool, found := registry.tools[name]
	return tool, found
}
