package browseract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserclick"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
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
	// MethodScroll scrolls the page.
	MethodScroll = "scroll"
	// MaxScrollAmount is the most steps one scroll may ask for, because a
	// page is read a screen at a time and a longer scroll is a loop.
	MaxScrollAmount = 20
)

// Settings is what the browser act tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// writtenStep is one step of a batch as the model writes it.
type writtenStep struct {
	// Method is click, type, press, or scroll.
	Method string
	// Target is the element to act on, for a click or typing, or the point to
	// click, as the model wrote it.
	Target browserclick.Target
	// Text is what to type, when the method is type.
	Text string
	// Key is the key to press, when the method is press.
	Key string
	// Direction is up or down, when the method is scroll.
	Direction string
	// Amount is how many steps to scroll, when the method is scroll; one
	// when it is left out.
	Amount int
	// Expectation says what should happen because of this step.
	Expectation string
	// WroteExpectation says the step said what should happen at all.
	WroteExpectation bool
	// Fields is the step as the model wrote it, for a refusal that names the
	// field it did not write.
	Fields *loose.Fields
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what the whole batch is for.
	Intent string
	// Steps are the steps to run, in order.
	Steps []writtenStep
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
			{Name: "steps", Type: "array", Description: "The steps in order, each with a method (click, type, press, or scroll), what it acts on (an element, or x and y for a click at a point), and an expectation.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassExecute},
	}
}

// Run runs the batch and hands back what each step did.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
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

// The names a model writes for the fields of one batch and of the steps in it.
var (
	stepNames      = []string{"steps", "actions", "batch"}
	keyNames       = []string{"key", "keys", "key_name", "text", "press"}
	directionNames = []string{"direction", "way", "towards"}
	amountNames    = []string{"amount", "steps_to_scroll", "how_far", "count"}
	textNames      = []string{"text", "value", "content", "input", "to_type"}
	methodNames    = []string{"method", "action", "kind", "do"}
)

// readInput reads the batch the model wrote, each step as loosely as one call to
// a tool of its own, so that a step written in capitals or with the element
// under another name is still the step it plainly means.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent and steps")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	items, wroteSteps := fields.List(stepNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if !wroteSteps {
		return input{}, fields.Missing("steps", "the steps to run, in the order they happen,")
	}
	steps, err := readWrittenSteps(items)
	if err != nil {
		return input{}, err
	}
	return input{Intent: intent, Steps: steps}, nil
}

// readWrittenSteps reads every step of the batch as the model wrote it.
func readWrittenSteps(items []json.RawMessage) ([]writtenStep, error) {
	steps := make([]writtenStep, 0, len(items))
	for at, item := range items {
		fields, err := loose.Read(item, "a method, what it acts on, and an expectation")
		if err != nil {
			return nil, fmt.Errorf("cannot read step %d: %w", at+1, err)
		}
		step := writtenStep{Fields: fields}
		method, _ := fields.Text(methodNames...)
		step.Method = loose.Action(method)
		step.Target = browserclick.ReadTarget(fields)
		step.Text, _ = fields.Text(textNames...)
		step.Key, _ = fields.Text(keyNames...)
		step.Direction, _ = fields.Text(directionNames...)
		step.Amount, _ = fields.Number(amountNames...)
		step.Expectation, step.WroteExpectation = fields.Text(browserclick.ExpectationNames...)
		if err := fields.Wrong(); err != nil {
			return nil, fmt.Errorf("cannot read step %d: %w", at+1, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// readSteps turns the batch the model wrote into the steps the worker runs, and
// refuses anything the worker could not run.
func readSteps(asked input) ([]contract.ActStep, error) {
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
		steps = append(steps, actStep(step))
	}
	return steps, nil
}

// actStep turns one written step into the step the worker runs. A click at a
// point carries x and y and no reference; every other step carries its
// reference, which is empty for a press or a scroll.
func actStep(step writtenStep) contract.ActStep {
	built := contract.ActStep{
		Method: step.Method, Ref: step.Target.Element, Text: step.Text, Key: step.Key,
		Direction: contract.ScrollDirection(loose.Action(step.Direction)), Amount: max(step.Amount, 1),
		Expectation: step.Expectation,
	}
	if step.Method == MethodClick && step.Target.AtPoint() {
		across, down := step.Target.Across, step.Target.Down
		built.Across, built.Down = &across, &down
	}
	return built
}

// checkStep holds the rules one step of a batch must satisfy.
func checkStep(at int, step writtenStep) error {
	switch step.Method {
	case MethodClick:
		if err := browserclick.NeedTarget(step.Target); err != nil {
			return fmt.Errorf("the batch cannot run step %d: %w", at+1, err)
		}
	case MethodType:
		if err := browserclick.NeedElement(step.Fields, step.Target.Element, step.Target.WroteElement); err != nil {
			return fmt.Errorf("the batch cannot run step %d: %w", at+1, err)
		}
	case MethodPress:
		if strings.TrimSpace(step.Key) == "" {
			return fmt.Errorf("step %d names no key to press, so write one such as Enter", at+1)
		}
	case MethodScroll:
		if err := checkScroll(step); err != nil {
			return fmt.Errorf("step %d cannot scroll the page: %w", at+1, err)
		}
	default:
		return fmt.Errorf("step %d asks for %q, which is not a method this tool knows, so use click, type, press, or scroll",
			at+1, step.Method)
	}
	if err := browserclick.NeedExpectation(step.Fields, step.Expectation, step.WroteExpectation); err != nil {
		return fmt.Errorf("the batch cannot run step %d: %w", at+1, err)
	}
	return nil
}

// checkScroll holds the rules a scroll step must satisfy: a direction the
// browser knows and an amount inside the cap.
func checkScroll(step writtenStep) error {
	direction := contract.ScrollDirection(loose.Action(step.Direction))
	if direction != contract.ScrollUp && direction != contract.ScrollDown {
		return fmt.Errorf("the scroll direction %q is not one the browser knows, so use up or down", step.Direction)
	}
	if step.Amount > MaxScrollAmount {
		return fmt.Errorf("the scroll asks for %d steps and the cap is %d, so scroll in pieces", step.Amount, MaxScrollAmount)
	}
	return nil
}
