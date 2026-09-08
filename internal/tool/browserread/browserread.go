package browserread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
)

// MaxIntentRunes is how long the line saying what a step is for may be. It is
// read by a person in a preview and written into the log, so it fits on a line.
const MaxIntentRunes = 300

// IntentNames are the names a model writes for the line saying what a step is
// for. Every browser tool and the desktop tool read it under all of them.
var IntentNames = []string{"intent", "why", "goal"}

// visibleNames are the names a model writes for reading only what is above the
// fold.
var visibleNames = []string{"visible_only", "visible", "only_visible", "above_the_fold"}

// askNames are the names a model writes for the one expression it asks the
// page to answer.
var askNames = []string{"ask", "expression", "evaluate", "question"}

// Settings is what the browser read tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for, in the model's own words.
	Intent string
	// VisibleOnly reads only what is above the fold.
	VisibleOnly bool
	// Ask is one expression for the page to answer, on a page served from
	// this machine.
	Ask string
}

// Tool is the browser read tool.
type Tool struct {
	settings Settings
}

// New returns the browser read tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserRead,
		Description: "Reads the browser's page as an outline of its elements, each with a reference to act on it by. " +
			"To see the page itself, take a browser_screenshot. Use web fetch for a page needing no login.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "visible_only", Type: "boolean", Description: "True to read only what is above the fold."},
			{Name: "ask", Type: "string", Description: "One expression the page evaluates and answers, such as window.game.state; only on a page served from this machine (localhost, 127.0.0.1) or a file. Not for clicking: a canvas or anything the outline does not list is clicked with browser_click at x and y."},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork},
	}
}

// Run reads the page the browser is on.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	page, err := tool.settings.Browser.Read(ctx, contract.ReadOptions{VisibleOnly: asked.VisibleOnly, Ask: asked.Ask})
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot read the page the browser is on: %w", err)
	}
	return contract.ToolOutput{Text: PageText(page)}, nil
}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(IntentNames...)
	visibleOnly, _ := fields.Flag(visibleNames...)
	ask, _ := fields.Text(askNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	ask = strings.TrimSpace(ask)
	if err := needAsk(intent, ask); err != nil {
		return input{}, err
	}
	return input{Intent: intent, VisibleOnly: visibleOnly, Ask: ask}, nil
}

// NeedIntent holds the rule every browser and desktop call keeps: it says what
// the step is for, because that line is what the user reads in a preview and
// what a recorded skill is patched against when a page changes. A call that
// writes no intent at all is refused with the field's own name.
func NeedIntent(fields *loose.Fields, intent string, wrote bool) error {
	if !wrote {
		return fields.Missing("intent", "what this step is for, in one line,")
	}
	return CheckIntent(intent)
}

// CheckIntent holds the length rule for that line.
func CheckIntent(intent string) error {
	if strings.TrimSpace(intent) == "" {
		return errors.New("this call does not say what the step is for, so write the intent in one line")
	}
	if len([]rune(intent)) > MaxIntentRunes {
		return fmt.Errorf("the intent is %d characters and the cap is %d, so say it in one line",
			len([]rune(intent)), MaxIntentRunes)
	}
	return nil
}
