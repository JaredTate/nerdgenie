package browserresize_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browserresize"
)

// newTool builds the resize tool over the fake browser worker, already on the
// simple fixture page.
func newTool(t *testing.T) (*browserresize.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	return browserresize.New(browserresize.Settings{Browser: worker}), worker
}

func run(t *testing.T, tool *browserresize.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

// TestThePageIsSetToTheSizeAskedForAndReadAgain is the tool's job: the visual
// QA task of the fresh game build had five sizes to check and no way to set
// one, and reached for the desktop tool to drag the window by hand.
func TestThePageIsSetToTheSizeAskedForAndReadAgain(t *testing.T) {
	tool, worker := newTool(t)

	output, err := run(t, tool, map[string]any{"intent": "check the phone layout", "width": 480, "height": 640})
	if err != nil {
		t.Fatalf("setting the page's size failed: %v", err)
	}
	if !strings.HasPrefix(output.Text, "the page is now 480 by 640 pixels\n") {
		t.Errorf("the answer does not open with the size:\n%s", output.Text)
	}
	if !strings.Contains(output.Text, "A simple page") {
		t.Errorf("the answer does not carry the page read again:\n%s", output.Text)
	}
	if width, height := worker.Size(); width != 480 || height != 640 {
		t.Errorf("the fake browser was set to %d by %d, want 480 by 640", width, height)
	}
}

func TestASizeNoScreenHasIsRefusedWithTheBounds(t *testing.T) {
	tool, _ := newTool(t)

	for _, fields := range []map[string]any{
		{"intent": "too narrow", "width": 10, "height": 640},
		{"intent": "too tall", "width": 480, "height": 9000},
		{"intent": "not whole", "width": 480.5, "height": 640},
		{"intent": "missing", "width": 480},
	} {
		_, err := run(t, tool, fields)
		if err == nil || !strings.Contains(err.Error(), "whole number of pixels between") {
			t.Errorf("%v was not refused with the bounds: %v", fields, err)
		}
	}
}

func TestACallWithNoIntentOrNoBrowserIsRefused(t *testing.T) {
	tool, _ := newTool(t)
	if _, err := run(t, tool, map[string]any{"width": 480, "height": 640}); err == nil || !strings.Contains(err.Error(), "intent") {
		t.Errorf("a call with no intent was not refused: %v", err)
	}
	unwired := browserresize.New(browserresize.Settings{})
	if _, err := run(t, unwired, map[string]any{"intent": "x", "width": 480, "height": 640}); err == nil || !strings.Contains(err.Error(), "no browser") {
		t.Errorf("a tool with no browser was not refused: %v", err)
	}
	if _, err := unwired.Run(context.Background(), json.RawMessage(`{"intent": 5`)); err == nil {
		t.Errorf("arguments that are not JSON were not refused")
	}
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	spec := browserresize.New(browserresize.Settings{}).Spec()
	if spec.Name != contract.ToolBrowserResize {
		t.Errorf("the tool is named %q, want %q", spec.Name, contract.ToolBrowserResize)
	}
	if words := len(strings.Fields(spec.Description)); words >= 40 {
		t.Errorf("the description is %d words long, and it has to stay under forty", words)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,width,height" {
		t.Errorf("the fields are %v, want intent, width, height", names)
	}
}
