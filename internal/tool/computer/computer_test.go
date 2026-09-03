package computer_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/computer"
)

// newTool builds the desktop tool over the fake desktop, with one application
// the user has already granted and open.
func newTool(t *testing.T) (*computer.Tool, *testkit.FakeDesktop) {
	t.Helper()
	desktop := testkit.NewFakeDesktop()
	desktop.Grant("a-text-editor")
	tool := computer.New(computer.Settings{Desktop: desktop})
	if _, err := run(t, tool, map[string]any{
		"intent": "open the editor", "action": "launch", "application": "a-text-editor",
		"expectation": "the editor is on the screen",
	}); err != nil {
		t.Fatalf("cannot open the fixture application: %v", err)
	}
	return tool, desktop
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *computer.Tool, fields map[string]any) (contract.ToolOutput, error) {
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

	if spec.Name != contract.ToolComputer {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolComputer)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,action,application,element,text,keys,to,expectation" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a desktop call by intent", names)
	}
	if len(spec.Classes) != 2 {
		t.Errorf("the tool claims the classes %v, and it runs programs and does things that cannot be undone", spec.Classes)
	}
}

func TestEveryActionReachesTheDesktop(t *testing.T) {
	tool, desktop := newTool(t)

	for _, one := range []map[string]any{
		{"intent": "see the screen", "action": "screenshot", "expectation": "the editor is showing"},
		{"intent": "click the button", "action": "click", "element": 1, "expectation": "the file opens"},
		{"intent": "write the note", "action": "type", "text": "a note", "expectation": "the note is in the box"},
		{"intent": "save it", "action": "key", "keys": "ctrl+s", "expectation": "the file is saved"},
		{"intent": "move the block", "action": "drag", "element": 1, "to": 2, "expectation": "the block moved"},
		{"intent": "put it on the clipboard", "action": "set_clipboard", "text": "copied", "expectation": "the clipboard holds it"},
		{"intent": "read the clipboard", "action": "clipboard", "expectation": "the clipboard holds what was copied"},
	} {
		if _, err := run(t, tool, one); err != nil {
			t.Fatalf("the action %v failed: %v", one["action"], err)
		}
	}
	testkit.Golden(t, "the_actions.txt", []byte(strings.Join(desktop.Actions(), "\n")+"\n"))
}

func TestAScreenshotComesBackAsTheNumberedControlsAndNotAsAPicture(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "see the screen", "action": "screenshot", "expectation": "the editor is showing",
	})
	if err != nil {
		t.Fatalf("taking a screenshot failed: %v", err)
	}
	testkit.Golden(t, "a_screenshot.txt", []byte(output.Text))
	if len(output.Text) > 2000 {
		t.Errorf("the screenshot came back as %d bytes, and the model reads the numbers rather than the picture", len(output.Text))
	}
}

func TestAnApplicationTheUserHasNotGrantedIsRefused(t *testing.T) {
	desktop := testkit.NewFakeDesktop()
	tool := computer.New(computer.Settings{Desktop: desktop})

	_, err := run(t, tool, map[string]any{
		"intent": "open something", "action": "launch", "application": "a-banking-app", "expectation": "it opens",
	})
	if err == nil {
		t.Fatalf("an application the user has not granted was opened")
	}
	if !strings.Contains(err.Error(), "granted") {
		t.Errorf("the refusal reads %q and does not say why it was refused", err)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"action": "screenshot", "expectation": "x"},
		{"intent": "do something", "expectation": "x"},
		{"intent": "do something", "action": "dance", "expectation": "x"},
		{"intent": "do something", "action": "launch", "expectation": "x"},
		{"intent": "do something", "action": "click", "expectation": "x"},
		{"intent": "do something", "action": "drag", "element": 1, "expectation": "x"},
		{"intent": "do something", "action": "key", "expectation": "x"},
		{"intent": "do something", "action": "screenshot"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoDesktopWiredInSaysSo(t *testing.T) {
	tool := computer.New(computer.Settings{})

	_, err := run(t, tool, map[string]any{"intent": "see the screen", "action": "screenshot", "expectation": "x"})
	if err == nil {
		t.Fatalf("the desktop was used with no desktop behind the tool")
	}
	if !strings.Contains(err.Error(), "desktop") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
