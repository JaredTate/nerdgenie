package computer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/loose"
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
	Intent string
	// WroteIntent says the call said what the step is for at all.
	WroteIntent bool
	// Action is which of the eight this call is, folded to the one it means.
	Action string
	// Application is the program to open, when the action is launch.
	Application string
	// Element is the number of the control to act on, from the last screenshot.
	Element int
	// Text is what to type or to put on the clipboard.
	Text string
	// Keys is the key combination to press, such as ctrl+s.
	Keys string
	// To is the number of the control to drag to.
	To int
	// Expectation says what should happen because of the step.
	Expectation string
	// WroteExpectation says the call said what should happen at all.
	WroteExpectation bool
	// Fields is the call as the model wrote it, for a refusal that names the
	// field it did not write.
	Fields *loose.Fields
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
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
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
		return said(asked, desktop.Launch(ctx, asked.Application, asked.Expectation))
	case ActionScreenshot:
		return tool.screenshot(ctx)
	case ActionClick:
		return said(asked, desktop.Click(ctx, asked.Element, asked.Expectation))
	case ActionType:
		return said(asked, desktop.Type(ctx, asked.Text, asked.Expectation))
	case ActionKey:
		return said(asked, desktop.Press(ctx, asked.Keys, asked.Expectation))
	case ActionDrag:
		return said(asked, desktop.Drag(ctx, asked.Element, asked.To, asked.Expectation))
	case ActionSetClipboard:
		return said(asked, desktop.SetClipboard(ctx, asked.Text))
	default:
		held, err := desktop.Clipboard(ctx)
		if err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read the clipboard: %w", err)
		}
		return contract.ToolOutput{Text: "the clipboard holds: " + oneLine(held) + "\n"}, nil
	}
}

// screenshot says what is on the screen, which is what the model reads in place
// of the picture: the windows by title, then the numbered controls of the open
// application, or a line saying to launch one when none is open.
func (tool *Tool) screenshot(ctx context.Context) (contract.ToolOutput, error) {
	picture, err := tool.settings.Desktop.Screenshot(ctx)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot take a picture of the screen: %w", err)
	}
	written := &strings.Builder{}
	written.WriteString(windowsLine(picture.Windows))
	switch {
	case picture.Application == "":
		written.WriteString("no application is open, so no control is numbered; launch one by its program name or its window title to click or type in it\n")
	case len(picture.Marks) == 0:
		fmt.Fprintf(written, "nothing in %s can be clicked\n", picture.Application)
	default:
		fmt.Fprintf(written, "controls in %s:\n", picture.Application)
		for _, mark := range picture.Marks {
			fmt.Fprintf(written, "%d %s %q\n", mark.Number, mark.Role, mark.Name)
		}
	}
	return contract.ToolOutput{Text: written.String()}, nil
}

// windowsLine names the windows on the screen on one line, so that the model
// knows what it is looking at.
func windowsLine(windows []string) string {
	if len(windows) == 0 {
		return "windows on the screen: none\n"
	}
	quoted := make([]string, 0, len(windows))
	for _, title := range windows {
		quoted = append(quoted, strconv.Quote(oneLine(title)))
	}
	return "windows on the screen: " + strings.Join(quoted, ", ") + "\n"
}

// said turns the desktop's answer into the line the model reads.
func said(asked input, err error) (contract.ToolOutput, error) {
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot %s on the desktop: %w", asked.Action, err)
	}
	if strings.TrimSpace(asked.Expectation) == "" {
		return contract.ToolOutput{Text: asked.Action + " done; take a screenshot to see what happened\n"}, nil
	}
	return contract.ToolOutput{Text: fmt.Sprintf("%s done; take a screenshot to see whether %s\n",
		asked.Action, oneLine(asked.Expectation))}, nil
}

// oneLine puts text on a single line, because the answer keeps one.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
