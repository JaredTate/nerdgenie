package browseropen_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browseropen"
)

// newTool builds the open tool over the fake browser worker.
func newTool(t *testing.T) (*browseropen.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	return browseropen.New(browseropen.Settings{Browser: worker}), worker
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browseropen.Tool, fields map[string]any) (contract.ToolOutput, error) {
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

	if spec.Name != contract.ToolBrowserOpen {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserOpen)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,url" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces this call by url and intent", names)
	}
}

func TestAPageIsOpenedAndComesBackAsATree(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"intent": "start at the simple page", "url": testkit.FixtureSimplePage})
	if err != nil {
		t.Fatalf("opening a page failed: %v", err)
	}
	testkit.Golden(t, "an_opened_page.txt", []byte(output.Text))
}

func TestAPageTheBrowserCannotOpenSaysSo(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"intent": "go somewhere", "url": "https://fixture.test/nowhere"})
	if err == nil {
		t.Fatalf("a page the browser has no idea about was opened")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("the failure reads %q and does not name the page", err)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"url": testkit.FixtureSimplePage},
		{"intent": "go somewhere"},
		{"intent": "go somewhere", "url": "not a web address"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoBrowserWiredInSaysSo(t *testing.T) {
	tool := browseropen.New(browseropen.Settings{})

	_, err := run(t, tool, map[string]any{"intent": "go somewhere", "url": testkit.FixtureSimplePage})
	if err == nil {
		t.Fatalf("a page was opened with no browser behind the tool")
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
