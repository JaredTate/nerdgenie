package browserclick

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserread"
)

// MaxExpectationRunes is how long the line saying what should happen may be.
const MaxExpectationRunes = 300

// Settings is what the browser click tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for.
	Intent string `json:"intent"`
	// Element is the reference of the element to click, such as e12.
	Element string `json:"element"`
	// Expectation says what should happen because of the click.
	Expectation string `json:"expectation"`
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
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an intent, an element, and an expectation in it: %w", err)
		}
	}
	if err := browserread.CheckIntent(asked.Intent); err != nil {
		return contract.ToolOutput{}, err
	}
	if err := CheckElement(asked.Element); err != nil {
		return contract.ToolOutput{}, err
	}
	if err := CheckExpectation(asked.Expectation); err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	change, err := tool.settings.Browser.Click(ctx, asked.Element, asked.Expectation)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot click %s: %w", asked.Element, err)
	}
	return contract.ToolOutput{Text: browserread.ChangeText(change)}, nil
}

// CheckElement holds the rule that every action names the element it acts on,
// by the reference the last reading of the page gave it.
func CheckElement(element string) error {
	if strings.TrimSpace(element) == "" {
		return errors.New("this call names no element, so read the page and give the reference of the one to act on, such as e12")
	}
	return nil
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
