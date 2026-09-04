package repair_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/repair"
)

// maxFuzzedTools caps how many pretend tools one fuzzed case sets up, because a
// fuzz target has to stay fast enough to run thousands of times a second.
const maxFuzzedTools = 20

func FuzzFind(f *testing.F) {
	for _, oneCase := range goldenCases {
		f.Add(oneCase.reply.Text, "read,write,edit,search,shell,browser_open", 0)
	}
	f.Add("", "", 0)
	f.Add("<tool_call>", "read", 0)
	f.Add("<tool_call>{", "read", 1)
	f.Add(`{"name":`, "read", 2)
	f.Add("```json\n[{", "read", 0)
	f.Add("read(", "read", 0)
	f.Add("read: {}", "read", 0)
	f.Add(strings.Repeat("{", 64), "read", 0)
	f.Add("<think>", "read", 0)
	f.Add("</think>", "read", 0)
	f.Add("<think><think></think>", "read", 0)
	f.Add("<think>"+`<tool_call>{"name": "read", "arguments": {}}</tool_call>`+"</think>", "read", 0)
	f.Add(strings.Repeat("<think>", 64), "read", 0)
	f.Add(`{"name": "read", "arguments": {"a": "\"}`, "read", 0)
	f.Add("<tool_call>{\"name\": \"read\"}", strings.Repeat("r", 4096), 0)

	f.Fuzz(func(t *testing.T, text string, names string, failedParses int) {
		specs := fuzzedSpecs(names)
		result := repair.Find(contract.Reply{Text: text}, specs, failedParses)
		checkResult(t, result, specs)
	})
}

// fuzzedSpecs turns a comma-separated list into pretend tools, keeping the list
// short so that one fuzzed case stays quick.
func fuzzedSpecs(names string) []contract.ToolSpec {
	specs := []contract.ToolSpec{}
	for _, name := range strings.Split(names, ",") {
		if name == "" || len(specs) >= maxFuzzedTools {
			continue
		}
		specs = append(specs, contract.ToolSpec{Name: name, Description: "A tool."})
	}
	return specs
}

// checkResult holds every promise Find makes, whatever it was given.
func checkResult(t *testing.T, result repair.Result, specs []contract.ToolSpec) {
	t.Helper()

	if result.Problem != "" && len(result.Calls) > 0 {
		t.Fatalf("the problem %q came back with %d calls, want one or the other", result.Problem, len(result.Calls))
	}
	if len(result.Calls) > contract.DefaultConfig().Caps.IdenticalCallWindow {
		t.Fatalf("%d calls came back, which is past the cap", len(result.Calls))
	}

	real := []string{}
	for _, spec := range specs {
		real = append(real, spec.Name)
	}
	for _, call := range result.Calls {
		if call.ID == "" {
			t.Fatalf("the call to %q has no identifier, so no result could name it", call.Name)
		}
		if !slices.Contains(real, call.Name) {
			t.Fatalf("the call names %q, which is not one of the real tools", call.Name)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(call.Input, &fields); err != nil {
			t.Fatalf("the arguments of the call to %q are not a JSON object: %s", call.Name, call.Input)
		}
	}
	for _, repaired := range result.Repairs {
		if !slices.Contains(real, repaired.RealName) {
			t.Fatalf("the repair says %q is a real tool, and it is not", repaired.RealName)
		}
	}
}
