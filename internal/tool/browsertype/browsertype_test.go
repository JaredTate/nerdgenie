package browsertype_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browsertype"
)

// newTool builds the type tool over the fake browser worker, on the simple
// fixture page.
func newTool(t *testing.T) (*browsertype.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	return browsertype.New(browsertype.Settings{Browser: worker}), worker
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browsertype.Tool, fields map[string]any) (contract.ToolOutput, error) {
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

	if spec.Name != contract.ToolBrowserType {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserType)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,element,text,expectation" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces typing by intent, element, and text", names)
	}
}

func TestTypingPutsTheWordsInTheBox(t *testing.T) {
	tool, worker := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "fill in the name", "element": testkit.FixtureUsernameRef,
		"text": "Jared", "expectation": "the box holds the name",
	})
	if err != nil {
		t.Fatalf("typing failed: %v", err)
	}
	if typed := worker.TypedInto(testkit.FixtureUsernameRef); len(typed) != 1 || typed[0] != "Jared" {
		t.Errorf("what was typed into the box is %v, want the one word the model wrote", typed)
	}
	testkit.Golden(t, "a_typing.txt", []byte(output.Text))
}

func TestTypingNothingAtAllIsAllowedBecauseAFieldIsSometimesCleared(t *testing.T) {
	tool, worker := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"intent": "clear the box", "element": testkit.FixtureUsernameRef, "text": "", "expectation": "the box is empty",
	}); err != nil {
		t.Fatalf("clearing a box failed: %v", err)
	}
	if typed := worker.TypedInto(testkit.FixtureUsernameRef); len(typed) != 1 || typed[0] != "" {
		t.Errorf("clearing the box wrote %v", typed)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"element": "e2", "text": "x", "expectation": "something"},
		{"intent": "type", "text": "x", "expectation": "something"},
		{"intent": "type", "element": "e2", "text": "x"},
		{"intent": "type", "element": "e2", "text": strings.Repeat("x", browsertype.MaxTextRunes+1), "expectation": "something"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoBrowserWiredInSaysSo(t *testing.T) {
	tool := browsertype.New(browsertype.Settings{})

	_, err := run(t, tool, map[string]any{"intent": "type", "element": "e2", "text": "x", "expectation": "y"})
	if err == nil {
		t.Fatalf("words were typed with no browser behind the tool")
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
