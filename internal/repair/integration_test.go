//go:build integration

package repair_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/repair"
	"github.com/JaredTate/coeus/internal/testkit"
)

// builtInSpecs are the eighteen real tools, which is the set the forty-step
// fixture calls and the set a repaired name has to land in.
func builtInSpecs() []contract.ToolSpec {
	specs := []contract.ToolSpec{}
	for _, name := range contract.BuiltInToolNames() {
		specs = append(specs, contract.ToolSpec{Name: name, Description: "A built-in tool."})
	}
	return specs
}

// textShapes are the ways a model with no tool interface writes the same call.
var textShapes = map[string]func(name string, arguments string) string{
	"tag block": func(name string, arguments string) string {
		return contract.ToolCallOpenTag + `{"name": "` + name + `", "arguments": ` + arguments + "}" + contract.ToolCallCloseTag
	},
	"tag block with no closing tag": func(name string, arguments string) string {
		return "I will use a tool now.\n" + contract.ToolCallOpenTag + `{"name": "` + name + `", "arguments": ` + arguments + "}"
	},
	"a fence with a language tag": func(name string, arguments string) string {
		return "```json\n" + `{"name": "` + name + `", "input": ` + arguments + "}" + "\n```"
	},
	"a bare object": func(name string, arguments string) string {
		return "Here it is.\n" + `{"name": "` + name + `", "arguments": ` + arguments + "}"
	},
	"a function-call line": func(name string, arguments string) string {
		return name + "(" + arguments + ")"
	},
}

func TestEveryCallInTheFortyStepFixtureSurvivesBeingWrittenAsText(t *testing.T) {
	task, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("loading the forty-step fixture failed: %v", err)
	}
	specs := builtInSpecs()
	checked := 0

	for _, round := range task.Rounds {
		if round.ToolName == "" {
			continue
		}
		arguments, err := json.Marshal(round.ToolInput)
		if err != nil {
			t.Fatalf("round %d: writing the arguments as JSON failed: %v", round.Number, err)
		}
		for shape, write := range textShapes {
			reply := contract.Reply{Text: write(round.ToolName, string(arguments))}
			result := repair.Find(reply, specs, 0)
			checkOneCall(t, result, round.ToolName, arguments, shape, round.Number)
			checked++
		}
	}

	if wanted := 39 * len(textShapes); checked != wanted {
		t.Errorf("%d calls were checked, want the %d rounds that call a tool in each of the %d shapes",
			checked, wanted/len(textShapes), len(textShapes))
	}
}

// checkOneCall holds what every shape of one round's call has to come back as.
func checkOneCall(t *testing.T, result repair.Result, name string, arguments []byte, shape string, round int) {
	t.Helper()

	if result.Problem != "" {
		t.Fatalf("round %d written as %s was refused with %q", round, shape, result.Problem)
	}
	if len(result.Calls) != 1 {
		t.Fatalf("round %d written as %s produced %d calls, want one", round, shape, len(result.Calls))
	}
	if result.Calls[0].Name != name {
		t.Errorf("round %d written as %s was read as the tool %q, want %q", round, shape, result.Calls[0].Name, name)
	}
	if !sameJSON(t, result.Calls[0].Input, arguments) {
		t.Errorf("round %d written as %s came back with the arguments %s, want %s",
			round, shape, result.Calls[0].Input, arguments)
	}
}

func TestTheCallsTheFakeModelPlaysComeBackUnchanged(t *testing.T) {
	task, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("loading the forty-step fixture failed: %v", err)
	}
	specs := builtInSpecs()
	model := testkit.NewFakeModel(task.Script())
	request := contract.Request{
		Messages: []contract.Message{{Role: contract.RoleUser, Text: task.Ask + "\n" + task.Correction}},
		Tools:    specs,
	}

	for _, round := range task.Rounds {
		// Past the stop, the script expects to see the user's reply that let the
		// task resume, the way the harness would carry it.
		if round.Number == task.StopRound+1 {
			request.Messages = append(request.Messages, contract.Message{Role: contract.RoleUser, Text: task.UserReplyAfterStop})
		}
		reply, err := model.Send(context.Background(), request, nil)
		if err != nil {
			t.Fatalf("the fake model refused the request: %v", err)
		}
		result := repair.Find(reply, specs, 0)
		if result.Problem != "" {
			t.Fatalf("a reply the fake model played was refused with %q", result.Problem)
		}
		if len(result.Calls) != len(reply.ToolCalls) {
			t.Fatalf("a reply with %d calls came back with %d", len(reply.ToolCalls), len(result.Calls))
		}
		for at, call := range reply.ToolCalls {
			if result.Calls[at].ID != call.ID || result.Calls[at].Name != call.Name {
				t.Errorf("the call %+v came back as %+v", call, result.Calls[at])
			}
		}
	}
	if left := model.StepsLeft(); left != 0 {
		t.Errorf("%d steps of the script were never played", left)
	}
}

// sameJSON says whether two pieces of JSON hold the same object, whatever order
// the fields were written in.
func sameJSON(t *testing.T, left []byte, right []byte) bool {
	t.Helper()
	return sortedJSON(t, left) == sortedJSON(t, right)
}

// sortedJSON writes a JSON object back out with its fields in one order, which
// is what lets two spellings of the same arguments be compared.
func sortedJSON(t *testing.T, raw []byte) string {
	t.Helper()
	fields := map[string]any{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("cannot read %s as a JSON object: %v", raw, err)
	}
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write %s back out as JSON: %v", raw, err)
	}
	return string(written)
}
