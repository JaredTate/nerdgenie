package memory_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/memory"
)

// theFacts are what the fake memory holds in every test here.
func theFacts() []contract.Fact {
	return []contract.Fact{
		{ID: "f1", Text: "The anniversary is in January.", Source: "the user", Recorded: time.Unix(1700000000, 0).UTC()},
		{ID: "f2", Text: "The build runs with make check.", Source: "a command", Recorded: time.Unix(1700000100, 0).UTC()},
	}
}

// newTool builds the memory tool over the fake memory.
func newTool(t *testing.T) (*memory.Tool, *testkit.FakeMemory) {
	t.Helper()
	held := testkit.NewFakeMemory(theFacts()...)
	return memory.New(memory.Settings{Memory: held}), held
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *memory.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolMemory {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolMemory)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "action,query,name,text,source" {
		t.Errorf("the tool takes the fields %v, and the fixed names are action, query, and name", names)
	}
	if len(spec.Classes) != 2 {
		t.Errorf("the tool claims the classes %v, and it both reads and writes", spec.Classes)
	}
}

func TestASearchFindsTheFactsThatMatch(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"action": "search", "query": "anniversary"})
	if err != nil {
		t.Fatalf("searching memory failed: %v", err)
	}
	testkit.Golden(t, "a_search.txt", []byte(output.Text))
}

func TestGettingOneFactByItsNameBringsItBack(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"action": "get", "name": "f2"})
	if err != nil {
		t.Fatalf("getting one fact failed: %v", err)
	}
	if !strings.Contains(output.Text, "make check") {
		t.Errorf("getting f2 returned %q, want the fact it holds", output.Text)
	}
}

func TestSavingAFactPutsItInMemory(t *testing.T) {
	tool, held := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"action": "save", "text": "The anniversary party is on the tenth.", "source": "the user",
	}); err != nil {
		t.Fatalf("saving a fact failed: %v", err)
	}
	found, err := held.Search(context.Background(), "party", 10)
	if err != nil {
		t.Fatalf("cannot search the fake memory: %v", err)
	}
	if len(found) != 1 || found[0].Source != "the user" {
		t.Errorf("memory holds %v after the save, want the one fact with its source", found)
	}
}

func TestASearchThatMatchesNothingSaysSo(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"action": "search", "query": "nothing like this"})
	if err != nil {
		t.Fatalf("searching memory failed: %v", err)
	}
	if !strings.Contains(output.Text, "nothing") {
		t.Errorf("a search that found nothing said %q", output.Text)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"action": "dance"},
		{"action": "search"},
		{"action": "get"},
		{"action": "save"},
		{"action": "get", "name": "f99"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoMemoryWiredInSaysSo(t *testing.T) {
	tool := memory.New(memory.Settings{})

	_, err := run(t, tool, map[string]any{"action": "search", "query": "anything"})
	if err == nil {
		t.Fatalf("memory was searched with no memory behind the tool")
	}
	if !strings.Contains(err.Error(), "memory") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
