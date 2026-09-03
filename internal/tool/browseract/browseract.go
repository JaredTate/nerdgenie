package browseract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserclick"
	"github.com/JaredTate/coeus/internal/tool/browserread"
)

// MaxSteps is how many steps one batch may hold. A batch longer than this is a
// plan, and a plan belongs in the record.
const MaxSteps = 10

// The methods a step may ask for, which are the ones that act on a page.
const (
	// MethodClick clicks an element.
	MethodClick = "click"
	// MethodType types into an element.
	MethodType = "type"
	// MethodPress presses one key.
	MethodPress = "press"
)

// Settings is what the browser act tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// writtenStep is one step of a batch as the model writes it.
type writtenStep struct {
	// Method is click, type, or press.
	Method string `json:"method"`
	// Element is the reference to act on, for a click or typing.
	Element string `json:"element"`
	// Text is what to type, when the method is type.
	Text string `json:"text"`
	// Key is the key to press, when the method is press.
	Key string `json:"key"`
	// Expectation says what should happen because of this step.
	Expectation string `json:"expectation"`
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what the whole batch is for.
	Intent string `json:"intent"`
	// Steps are the steps to run, in order.
	Steps []writtenStep `json:"steps"`
}

// Tool is the browser act tool.
type Tool struct {
	settings Settings
}

// New returns the browser act tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserAct,
		Description: "Runs a short batch of browser steps, checking each one, and stops at the first that does not do what you expected. " +
			"Use it for a form rather than one call per box.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What the whole batch is for, in one line.", Required: true},
			{Name: "steps", Type: "array", Description: "The steps in order, each with a method, an element, and an expectation.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassExecute},
	}
}

// Run runs the batch and hands back what each step did.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an intent and steps in it: %w", err)
		}
	}
	steps, err := readSteps(asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	changes, err := tool.settings.Browser.Act(ctx, steps)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the batch stopped after %d steps: %w", len(changes), err)
	}
	return contract.ToolOutput{Text: batchText(changes)}, nil
}

// batchText is what the model reads back: one part per step that ran, numbered.
func batchText(changes []contract.Diff) string {
	written := &strings.Builder{}
	for at, change := range changes {
		fmt.Fprintf(written, "step %d of %d\n%s", at+1, len(changes), browserread.ChangeText(change))
	}
	if len(changes) == 0 {
		written.WriteString("no step ran\n")
	}
	return written.String()
}

// readSteps reads the batch the model wrote and refuses anything the worker
// could not run.
func readSteps(asked input) ([]contract.ActStep, error) {
	if err := browserread.CheckIntent(asked.Intent); err != nil {
		return nil, err
	}
	if len(asked.Steps) == 0 {
		return nil, errors.New("this batch has no steps in it, so write the steps to run in the order they happen")
	}
	if len(asked.Steps) > MaxSteps {
		return nil, fmt.Errorf("this batch has %d steps and the cap is %d, so run it in pieces", len(asked.Steps), MaxSteps)
	}

	steps := make([]contract.ActStep, 0, len(asked.Steps))
	for at, step := range asked.Steps {
		if err := checkStep(at, step); err != nil {
			return nil, err
		}
		steps = append(steps, contract.ActStep{
			Method: step.Method, Ref: step.Element, Text: step.Text, Key: step.Key, Expectation: step.Expectation,
		})
	}
	return steps, nil
}

// checkStep holds the rules one step of a batch must satisfy.
func checkStep(at int, step writtenStep) error {
	switch step.Method {
	case MethodClick, MethodType:
		if err := browserclick.CheckElement(step.Element); err != nil {
			return fmt.Errorf("the batch cannot run step %d: %w", at+1, err)
		}
	case MethodPress:
		if strings.TrimSpace(step.Key) == "" {
			return fmt.Errorf("step %d names no key to press, so write one such as Enter", at+1)
		}
	default:
		return fmt.Errorf("step %d asks for %q, which is not a method this tool knows, so use click, type, or press",
			at+1, step.Method)
	}
	if err := browserclick.CheckExpectation(step.Expectation); err != nil {
		return fmt.Errorf("the batch cannot run step %d: %w", at+1, err)
	}
	return nil
}
