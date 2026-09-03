package browserclick_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/browserclick"
)

// newTool builds the click tool over the fake browser worker, on the simple
// fixture page.
func newTool(t *testing.T) (*browserclick.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	return browserclick.New(browserclick.Settings{Browser: worker}), worker
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browserclick.Tool, fields map[string]any) (contract.ToolOutput, error) {
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

	if spec.Name != contract.ToolBrowserClick {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserClick)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,element,expectation" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a click by intent and element", names)
	}
}

func TestAClickReturnsWhatChangedOnThePage(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent":      "go to the page the link points at",
		"element":     testkit.FixtureChangeLinkRef,
		"expectation": "the page says it changed",
	})
	if err != nil {
		t.Fatalf("clicking failed: %v", err)
	}
	testkit.Golden(t, "a_click.txt", []byte(output.Text))
}

func TestAClickThatChangesNothingSaysSo(t *testing.T) {
	tool, worker := newTool(t)
	worker.NextActionChangesNothing()

	output, err := run(t, tool, map[string]any{
		"intent": "try the link", "element": testkit.FixtureChangeLinkRef, "expectation": "a receipt appears",
	})
	if err != nil {
		t.Fatalf("clicking failed: %v", err)
	}
	if !strings.Contains(output.Text, "not what was expected") {
		t.Errorf("a click that changed nothing said %q", output.Text)
	}
}

func TestAnElementThatIsNotOnThePageIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"intent": "click something", "element": "e99", "expectation": "something happens",
	}); err == nil {
		t.Errorf("an element that is not on the page was clicked")
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"element": "e1", "expectation": "something"},
		{"intent": "click", "expectation": "something"},
		{"intent": "click", "element": "e1"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoBrowserWiredInSaysSo(t *testing.T) {
	tool := browserclick.New(browserclick.Settings{})

	_, err := run(t, tool, map[string]any{"intent": "click", "element": "e1", "expectation": "something"})
	if err == nil {
		t.Fatalf("an element was clicked with no browser behind the tool")
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
