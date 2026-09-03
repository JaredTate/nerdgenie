package browsertype

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserclick"
	"github.com/JaredTate/coeus/internal/tool/browserread"
)

// MaxTextRunes is how much may be typed into one box in one call. A model
// pasting a book into a text field has lost its way.
const MaxTextRunes = 20000

// Settings is what the browser type tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for.
	Intent string `json:"intent"`
	// Element is the reference of the box to type into.
	Element string `json:"element"`
	// Text is what to type, which may be nothing at all to clear the box.
	Text string `json:"text"`
	// Expectation says what should happen because of the typing.
	Expectation string `json:"expectation"`
}

// Tool is the browser type tool.
type Tool struct {
	settings Settings
}

// New returns the browser type tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserType,
		Description: "Types into one box on the page and checks that what you expected happened. " +
			"Use browser_login for a username or a password, which this tool never types.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "element", Type: "string", Description: "The reference of the box to type into, such as e12.", Required: true},
			{Name: "text", Type: "string", Description: "What to type. Leave it empty to clear the box.", Required: true},
			{Name: "expectation", Type: "string", Description: "What should happen because of the typing.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassExecute},
	}
}

// Run types into the box and hands back what changed.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an intent, an element, text, and an expectation in it: %w", err)
		}
	}
	if err := checkCall(asked); err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	change, err := tool.settings.Browser.Type(ctx, asked.Element, asked.Text, asked.Expectation)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot type into %s: %w", asked.Element, err)
	}
	return contract.ToolOutput{Text: browserread.ChangeText(change)}, nil
}

// checkCall holds the rules one call must satisfy before anything is typed.
func checkCall(asked input) error {
	if err := browserread.CheckIntent(asked.Intent); err != nil {
		return err
	}
	if err := browserclick.CheckElement(asked.Element); err != nil {
		return err
	}
	if err := browserclick.CheckExpectation(asked.Expectation); err != nil {
		return err
	}
	if len([]rune(asked.Text)) > MaxTextRunes {
		return fmt.Errorf("this call would type %d characters and the cap is %d, so type it in pieces",
			len([]rune(asked.Text)), MaxTextRunes)
	}
	return nil
}
