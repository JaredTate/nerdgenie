package browsertype

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserclick"
	"github.com/JaredTate/coeus/internal/tool/browserread"
	"github.com/JaredTate/coeus/internal/tool/loose"
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
	Intent string
	// Element is the reference of the box to type into.
	Element string
	// Text is what to type, which may be nothing at all to clear the box.
	Text string
	// Expectation says what should happen because of the typing.
	Expectation string
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
	asked, err := readInput(written)
	if err != nil {
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

// textNames are the names a model writes for what to type. A call that writes
// none of them is refused rather than acted on, because typing nothing into a
// box empties it, and a model that forgot the text did not mean to.
var textNames = []string{"text", "value", "content", "input", "to_type"}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent, an element, text, and an expectation")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	element, wroteElement := fields.Text(browserclick.ElementNames...)
	text, wroteText := fields.Text(textNames...)
	expectation, wroteExpectation := fields.Text(browserclick.ExpectationNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if err := browserclick.NeedElement(fields, element, wroteElement); err != nil {
		return input{}, err
	}
	if !wroteText {
		return input{}, fields.Missing("text", "what to type, which may be an empty string to clear the box,")
	}
	if len([]rune(text)) > MaxTextRunes {
		return input{}, fmt.Errorf("this call would type %d characters and the cap is %d, so type it in pieces",
			len([]rune(text)), MaxTextRunes)
	}
	if err := browserclick.NeedExpectation(fields, expectation, wroteExpectation); err != nil {
		return input{}, err
	}
	return input{Intent: intent, Element: element, Text: text, Expectation: expectation}, nil
}
