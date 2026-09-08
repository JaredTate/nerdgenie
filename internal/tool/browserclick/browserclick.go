package browserclick

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
)

// MaxExpectationRunes is how long the line saying what should happen may be.
const MaxExpectationRunes = 300

// ElementNames are the names a model writes for the element a step acts on,
// ExpectationNames the names it writes for what should happen because of it,
// and XNames and YNames the names it writes for the two halves of a point.
// Every browser tool and the desktop tool read them under all of these.
var (
	ElementNames     = []string{"element", "ref", "element_ref", "target", "on"}
	ExpectationNames = []string{"expectation", "expected", "expect", "expected_result", "should_happen"}
	XNames           = []string{"x"}
	YNames           = []string{"y"}
)

// Settings is what the browser click tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// Target is what a click lands on, as the model wrote it: an element by the
// reference the last reading of the page gave it, or a point in whole CSS
// pixels from the top left of the viewport, which is how a canvas or anything
// else the outline does not list is clicked. NeedTarget holds the rule that a
// call names one or the other and never both.
type Target struct {
	// Element is the reference of the element, such as e12, and empty when the
	// call named none.
	Element string
	// Across is how far the point is from the left of the viewport, which the
	// model writes as x.
	Across int
	// Down is how far the point is from the top of the viewport, which the
	// model writes as y.
	Down int
	// WroteElement says the call carried an element field at all.
	WroteElement bool
	// WroteAcross says the call carried x, because a point at zero is still a
	// point.
	WroteAcross bool
	// WroteDown says the call carried y.
	WroteDown bool
}

// AtPoint says the target is a point rather than an element, once NeedTarget
// has passed it.
func (target Target) AtPoint() bool {
	return strings.TrimSpace(target.Element) == ""
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for.
	Intent string
	// Target is the element or the point to click.
	Target Target
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
		Description: "Clicks one element on the page, or a point on it for a canvas or anything the outline does not list, " +
			"and checks that what you expected happened. Read the page first for the element's reference.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "element", Type: "string", Description: "The reference of the element to click, such as e12. Leave it out to click a point."},
			{Name: "x", Type: "integer", Description: "With y, the point to click in place of an element: whole pixels from the left of the page's viewport, for a canvas or anything the outline does not list."},
			{Name: "y", Type: "integer", Description: "With x, whole pixels from the top of the page's viewport."},
			{Name: "expectation", Type: "string", Description: "What should happen because of the click.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassExecute},
	}
}

// Run clicks the element or the point and hands back what changed.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	change, err := tool.click(ctx, asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	return contract.ToolOutput{Text: browserread.ChangeText(change)}, nil
}

// click makes the click in whichever of its two forms the call asked for.
func (tool *Tool) click(ctx context.Context, asked input) (contract.Diff, error) {
	if asked.Target.AtPoint() {
		change, err := tool.settings.Browser.ClickAt(ctx, asked.Target.Across, asked.Target.Down, asked.Expectation)
		if err != nil {
			return contract.Diff{}, fmt.Errorf("cannot click the point %d,%d: %w", asked.Target.Across, asked.Target.Down, err)
		}
		return change, nil
	}
	change, err := tool.settings.Browser.Click(ctx, asked.Target.Element, asked.Expectation)
	if err != nil {
		return contract.Diff{}, fmt.Errorf("cannot click the element %s: %w", asked.Target.Element, err)
	}
	return change, nil
}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent, an element or a point, and an expectation")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	target := ReadTarget(fields)
	expectation, wroteExpectation := fields.Text(ExpectationNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if err := NeedTarget(target); err != nil {
		return input{}, err
	}
	if err := NeedExpectation(fields, expectation, wroteExpectation); err != nil {
		return input{}, err
	}
	return input{Intent: intent, Target: target, Expectation: expectation}, nil
}

// ReadTarget reads what a call says it clicks, under every name a model writes
// for it: the element's reference, or the x and y of a point. It only reads.
// NeedTarget holds the rules, and runs after fields.Wrong() so that a number
// written as something else is refused as that rather than as a point not
// named.
func ReadTarget(fields *loose.Fields) Target {
	target := Target{}
	target.Element, target.WroteElement = fields.Text(ElementNames...)
	target.Across, target.WroteAcross = fields.Number(XNames...)
	target.Down, target.WroteDown = fields.Number(YNames...)
	return target
}

// NeedTarget holds the rule that a click names the element to click, by the
// reference the last reading of the page gave it, or a point on the page, and
// never both. The refusal for neither, and for both, is one sentence saying
// what to send.
func NeedTarget(target Target) error {
	namesElement := strings.TrimSpace(target.Element) != ""
	namesPoint := target.WroteAcross || target.WroteDown
	switch {
	case namesElement && namesPoint:
		return errors.New("this call names both an element and a point, so send the element's reference alone for something the outline lists, or x and y alone for a canvas or anything it does not")
	case !namesElement && !namesPoint:
		return errors.New("this call names neither an element nor a point, so send the reference of an element from the page, such as e12, or x and y in whole pixels from the top left for a canvas or anything the outline does not list")
	case namesPoint && !(target.WroteAcross && target.WroteDown):
		return errors.New("a point needs both x and y, in whole pixels from the top left of the page, so send the one that is missing")
	default:
		return nil
	}
}

// NeedElement holds the rule that an action on an element names it, by the
// reference the last reading of the page gave it, and refuses a call that
// names none with the field's own name. Typing keeps this rule; a click may
// name a point instead, which NeedTarget holds.
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
