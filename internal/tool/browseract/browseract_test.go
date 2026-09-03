package browseract_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/browseract"
)

// newTool builds the act tool over the fake browser worker, on the simple
// fixture page.
func newTool(t *testing.T) (*browseract.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	return browseract.New(browseract.Settings{Browser: worker}), worker
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browseract.Tool, fields map[string]any) (contract.ToolOutput, error) {
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

	if spec.Name != contract.ToolBrowserAct {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserAct)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,steps" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a batch by intent", names)
	}
}

func TestABatchOfStepsRunsAndEachChangeComesBack(t *testing.T) {
	tool, worker := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "fill in the form and follow the link",
		"steps": []any{
			map[string]any{"method": "type", "element": testkit.FixtureUsernameRef, "text": "Jared", "expectation": "the box holds the name"},
			map[string]any{"method": "click", "element": testkit.FixtureChangeLinkRef, "expectation": "the page says it changed"},
		},
	})
	if err != nil {
		t.Fatalf("running a batch of steps failed: %v", err)
	}
	if typed := worker.TypedInto(testkit.FixtureUsernameRef); len(typed) != 1 {
		t.Errorf("the batch typed %v into the box", typed)
	}
	testkit.Golden(t, "a_batch.txt", []byte(output.Text))
}

func TestABatchStopsAtTheFirstStepThatDoesNotDoWhatWasExpected(t *testing.T) {
	tool, worker := newTool(t)
	worker.NextActionChangesNothing()

	output, err := run(t, tool, map[string]any{
		"intent": "follow the link twice",
		"steps": []any{
			map[string]any{"method": "click", "element": testkit.FixtureChangeLinkRef, "expectation": "a receipt appears"},
			map[string]any{"method": "click", "element": testkit.FixtureChangeLinkRef, "expectation": "a second receipt appears"},
		},
	})
	if err != nil {
		t.Fatalf("running a batch of steps failed: %v", err)
	}
	if strings.Count(output.Text, "step ") != 1 {
		t.Errorf("the batch ran past the step that failed: %q", output.Text)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	tooMany := []any{}
	for range browseract.MaxSteps + 1 {
		tooMany = append(tooMany, map[string]any{"method": "click", "element": "e1", "expectation": "something"})
	}
	for _, broken := range []map[string]any{
		{"steps": []any{map[string]any{"method": "click", "element": "e1", "expectation": "x"}}},
		{"intent": "do things"},
		{"intent": "do things", "steps": []any{}},
		{"intent": "do things", "steps": tooMany},
		{"intent": "do things", "steps": []any{map[string]any{"method": "dance", "element": "e1", "expectation": "x"}}},
		{"intent": "do things", "steps": []any{map[string]any{"method": "click", "expectation": "x"}}},
		{"intent": "do things", "steps": []any{map[string]any{"method": "click", "element": "e1"}}},
		{"intent": "do things", "steps": []any{map[string]any{"method": "press", "expectation": "x"}}},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoBrowserWiredInSaysSo(t *testing.T) {
	tool := browseract.New(browseract.Settings{})

	_, err := run(t, tool, map[string]any{
		"intent": "do things",
		"steps":  []any{map[string]any{"method": "click", "element": "e1", "expectation": "x"}},
	})
	if err == nil {
		t.Fatalf("a batch ran with no browser behind the tool")
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

// TestAScrollStepRidesInABatch covers the one method the worker and the fake
// already run that the model could not ask for: a scroll, with its direction
// and how far, runs inside a batch; a direction the browser does not know, or
// an amount past the cap, is refused with the rule named.
func TestAScrollStepRidesInABatch(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "read further down the page",
		"steps": []any{
			map[string]any{"method": "scroll", "direction": "down", "amount": 3, "expectation": "more of the page shows"},
			map[string]any{"method": "scroll", "direction": "up", "expectation": "the top of the page shows"},
		},
	})
	if err != nil {
		t.Fatalf("a batch with a scroll in it failed: %v", err)
	}
	if !strings.Contains(output.Text, "scroll") {
		t.Errorf("the output says nothing about the scroll: %q", output.Text)
	}

	for _, broken := range []map[string]any{
		{"method": "scroll", "direction": "sideways", "expectation": "the page moves"},
		{"method": "scroll", "direction": "down", "amount": browseract.MaxScrollAmount + 1, "expectation": "the page moves"},
	} {
		_, err := run(t, tool, map[string]any{"intent": "read further", "steps": []any{broken}})
		if err == nil || !strings.Contains(err.Error(), "scroll") {
			t.Errorf("the scroll step %v was not refused with the rule named: %v", broken, err)
		}
	}
}
