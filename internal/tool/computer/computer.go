package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The eight things one call can ask the desktop for.
const (
	// ActionLaunch opens an application or brings it forward.
	ActionLaunch = "launch"
	// ActionScreenshot lists the numbered controls on the screen.
	ActionScreenshot = "screenshot"
	// ActionClick clicks the control with a number.
	ActionClick = "click"
	// ActionType types text at human pacing.
	ActionType = "type"
	// ActionKey presses a key combination.
	ActionKey = "key"
	// ActionDrag drags from one numbered control to another.
	ActionDrag = "drag"
	// ActionClipboard reads what is on the clipboard.
	ActionClipboard = "clipboard"
	// ActionSetClipboard puts text on the clipboard.
	ActionSetClipboard = "set_clipboard"
)

// MaxTextRunes is how much may be typed in one call.
const MaxTextRunes = 20000

// Settings is what the desktop tool needs to do its work.
type Settings struct {
	// Desktop is the worker driving the screen, the mouse, and the keyboard.
	Desktop contract.Desktop
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for.
	Intent string `json:"intent"`
	// Action is which of the eight this call is.
	Action string `json:"action"`
	// Application is the program to open, when the action is launch.
	Application string `json:"application"`
	// Element is the number of the control to act on, from the last screenshot.
	Element int `json:"element"`
	// Text is what to type or to put on the clipboard.
	Text string `json:"text"`
	// Keys is the key combination to press, such as ctrl+s.
	Keys string `json:"keys"`
	// To is the number of the control to drag to.
	To int `json:"to"`
	// Expectation says what should happen because of the step.
	Expectation string `json:"expectation"`
}

// Tool is the desktop tool.
type Tool struct {
	settings Settings
}

// New returns the desktop tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolComputer,
		Description: "Drives the screen, the mouse, and the keyboard of this machine: launch, screenshot, click, type, key, drag, clipboard. " +
			"The last resort; if the browser can do it, use the browser.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "action", Type: "string", Description: "One of launch, screenshot, click, type, key, drag, clipboard, set_clipboard.", Required: true},
			{Name: "application", Type: "string", Description: "The program to open, when the action is launch."},
			{Name: "element", Type: "integer", Description: "The number of the control to act on, from the last screenshot."},
			{Name: "text", Type: "string", Description: "What to type, or what to put on the clipboard."},
			{Name: "keys", Type: "string", Description: "The key combination to press, such as ctrl+s."},
			{Name: "to", Type: "integer", Description: "The number of the control to drag to."},
			{Name: "expectation", Type: "string", Description: "What should happen because of this step.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassExecute, contract.ClassIrreversible},
	}
}

// Run does what the call asks the desktop for.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an intent, an action, and an expectation in it: %w", err)
		}
	}
	if err := checkCall(asked); err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Desktop == nil {
		return contract.ToolOutput{}, errors.New("this tool has no desktop behind it, so wire the desktop worker in before using it")
	}
	return tool.doIt(ctx, asked)
}

// doIt calls the one method the action means and says what happened.
func (tool *Tool) doIt(ctx context.Context, asked input) (contract.ToolOutput, error) {
	desktop := tool.settings.Desktop
	switch asked.Action {
	case ActionLaunch:
		return said(asked, desktop.Launch(ctx, asked.Application))
	case ActionScreenshot:
		return tool.screenshot(ctx)
	case ActionClick:
		return said(asked, desktop.Click(ctx, asked.Element))
	case ActionType:
		return said(asked, desktop.Type(ctx, asked.Text))
	case ActionKey:
		return said(asked, desktop.Press(ctx, asked.Keys))
	case ActionDrag:
		return said(asked, desktop.Drag(ctx, asked.Element, asked.To))
	case ActionSetClipboard:
		return said(asked, desktop.SetClipboard(ctx, asked.Text))
	default:
		held, err := desktop.Clipboard(ctx)
		if err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read the clipboard: %w", err)
		}
		return contract.ToolOutput{Text: "the clipboard holds: " + held + "\n"}, nil
	}
}

// screenshot lists the numbered controls on the screen, which is what the model
// reads in place of the picture.
func (tool *Tool) screenshot(ctx context.Context) (contract.ToolOutput, error) {
	picture, err := tool.settings.Desktop.Screenshot(ctx)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot take a picture of the screen: %w", err)
	}
	written := &strings.Builder{}
	for _, mark := range picture.Marks {
		fmt.Fprintf(written, "%d %s %q\n", mark.Number, mark.Role, mark.Name)
	}
	if len(picture.Marks) == 0 {
		written.WriteString("nothing on the screen can be clicked\n")
	}
	return contract.ToolOutput{Text: written.String()}, nil
}

// said turns the desktop's answer into the line the model reads.
func said(asked input, err error) (contract.ToolOutput, error) {
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot %s on the desktop: %w", asked.Action, err)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("%s done; take a screenshot to see whether %s\n",
		asked.Action, oneLine(asked.Expectation))}, nil
}

// oneLine puts text on a single line, because the answer keeps one.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
