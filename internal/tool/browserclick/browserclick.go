package browserclick

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserread"
	"github.com/JaredTate/coeus/internal/tool/loose"
)

// MaxExpectationRunes is how long the line saying what should happen may be.
const MaxExpectationRunes = 300

// ElementNames are the names a model writes for the element a step acts on, and
// ExpectationNames the names it writes for what should happen because of it.
// Every browser tool and the desktop tool read the two under all of them.
var (
	ElementNames     = []string{"element", "ref", "element_ref", "target", "on"}
	ExpectationNames = []string{"expectation", "expected", "expect", "expected_result", "should_happen"}
)

// Settings is what the browser click tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for.
	Intent string
	// Element is the reference of the element to click, such as e12.
	Element string
	// Expectation says what should happen because of the click.
	Expectation string
}

// Tool is the browser click tool.
type Tool struct {
	settings Settings
}

// New returns the browser click tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserClick,
		Description: "Clicks one element on the page the browser is on and checks that what you expected happened. " +
			"Read the page first to get the element's reference.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "element", Type: "string", Description: "The reference of the element to click, such as e12.", Required: true},
			{Name: "expectation", Type: "string", Description: "What should happen because of the click.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassExecute},
	}
}

// Run clicks the element and hands back what changed.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	change, err := tool.settings.Browser.Click(ctx, asked.Element, asked.Expectation)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot click the element %s: %w", asked.Element, err)
	}
	return contract.ToolOutput{Text: browserread.ChangeText(change)}, nil
}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent, an element, and an expectation")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	element, wroteElement := fields.Text(ElementNames...)
	expectation, wroteExpectation := fields.Text(ExpectationNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if err := NeedElement(fields, element, wroteElement); err != nil {
		return input{}, err
	}
	if err := NeedExpectation(fields, expectation, wroteExpectation); err != nil {
		return input{}, err
	}
	return input{Intent: intent, Element: element, Expectation: expectation}, nil
}

// NeedElement holds the rule that every action names the element it acts on, by
// the reference the last reading of the page gave it, and refuses a call that
// names none with the field's own name.
func NeedElement(fields *loose.Fields, element string, wrote bool) error {
	if !wrote {
		return fields.Missing("element", "the reference of the element to act on, such as e12,")
	}
	return CheckElement(element)
}

// CheckElement holds the rule that an element written as nothing at all is no
// element.
func CheckElement(element string) error {
	if strings.TrimSpace(element) == "" {
		return errors.New("this call names no element, so read the page and give the reference of the one to act on, such as e12")
	}
	return nil
}

// NeedExpectation holds the rule that every action says what it thinks will
// happen, and refuses a call that says nothing with the field's own name.
func NeedExpectation(fields *loose.Fields, expectation string, wrote bool) error {
	if !wrote {
		return fields.Missing("expectation", "what you expect to happen because of this step,")
	}
	return CheckExpectation(expectation)
}

// CheckExpectation holds the rule that every action says what it thinks will
// happen, because an action nobody checked is an action nobody can trust.
func CheckExpectation(expectation string) error {
	if strings.TrimSpace(expectation) == "" {
		return errors.New("this call says nothing about what should happen, so write in one line what you expect the page to do")
	}
	if len([]rune(expectation)) > MaxExpectationRunes {
		return fmt.Errorf("the expectation is %d characters and the cap is %d, so say it in one line",
			len([]rune(expectation)), MaxExpectationRunes)
	}
	return nil
}
