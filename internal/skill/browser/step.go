package browser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// Descriptor is the three ways a recorded step finds its element again: the
// short reference the page gave it, its role and name together, and the text it
// showed. A page that has been redesigned under a skill usually still answers to
// one of the three, which is why all three are written down rather than only the
// reference the model happened to use.
type Descriptor struct {
	// Ref is the short label the snapshot gave the element, such as "e12".
	Ref string
	// Role says what kind of element it is, such as "button" or "textbox".
	Role string
	// Name is what the page calls it, such as "Compose post".
	Name string
	// Shown is the text the element showed when the step was recorded.
	Shown string
}

// Empty says the descriptor names nothing at all, so no rung of the cascade
// could find an element with it.
func (descriptor Descriptor) Empty() bool {
	return descriptor.Ref == "" && descriptor.Role == "" && descriptor.Name == "" && descriptor.Shown == ""
}

// String says in plain words what the descriptor points at, which is what a
// patch line and a failed step both have to print.
func (descriptor Descriptor) String() string {
	named := descriptor.roleAndName()
	switch {
	case descriptor.Ref != "" && named != "":
		return descriptor.Ref + " (" + named + ")"
	case descriptor.Ref != "":
		return descriptor.Ref
	case named != "":
		return "the " + named
	case descriptor.Shown != "":
		return `the element showing "` + descriptor.Shown + `"`
	default:
		return "no element at all"
	}
}

// roleAndName is the role and the name written together, and an empty string
// when the descriptor carries neither.
func (descriptor Descriptor) roleAndName() string {
	switch {
	case descriptor.Role != "" && descriptor.Name != "":
		return descriptor.Role + ` "` + descriptor.Name + `"`
	case descriptor.Name != "":
		return `element named "` + descriptor.Name + `"`
	default:
		return descriptor.Role
	}
}

// Step is one recorded browser step: what it is for, which of the three browser
// actions it is, the element it acts on, and what the page should do because of
// it.
type Step struct {
	// Number is the step's place in the list, starting at one.
	Number int
	// Intent is what the step is for, in the words of whoever recorded it.
	Intent string
	// Tool is the browser tool the step calls, which is one of browser_open,
	// browser_click, and browser_type.
	Tool string
	// Address is the page to go to, and is empty on anything but an opening
	// step.
	Address string
	// Typed is what to type, and is empty on anything but a typing step.
	Typed string
	// Element is how to find the element again, and is empty on an opening step.
	Element Descriptor
	// Expectation is what the page should do because of this step, in plain
	// words.
	Expectation string
}

// stepInput is the JSON a recorded step hands its browser tool. It is the shape
// the tools in internal/tool already read, with the descriptor's other two rungs
// added, so one written step is both a recording this package can replay through
// the cascade and a call the ordinary skill runner can make through the registry.
type stepInput struct {
	// Intent says what the step is for.
	Intent string `json:"intent,omitempty"`
	// URL is the address an opening step goes to.
	URL string `json:"url,omitempty"`
	// Element is the reference of the element to act on.
	Element string `json:"element,omitempty"`
	// Role is the kind of element it was when the step was recorded.
	Role string `json:"role,omitempty"`
	// Name is what the page called it when the step was recorded.
	Name string `json:"name,omitempty"`
	// Shown is the text it showed when the step was recorded.
	Shown string `json:"shown,omitempty"`
	// Text is what a typing step types.
	Text string `json:"text,omitempty"`
	// Expectation says what the page should do because of the step.
	Expectation string `json:"expectation,omitempty"`
}

// DrivenHere says whether this package knows how to replay a step that calls
// that tool.
func DrivenHere(tool string) bool {
	switch tool {
	case contract.ToolBrowserOpen, contract.ToolBrowserClick, contract.ToolBrowserType:
		return true
	default:
		return false
	}
}

// ParseStep reads one step of a steps.md as a browser step. Everything it can
// refuse it refuses with the step's number and what to fix, because a recording
// that cannot be replayed has to say so when it is read rather than half way
// through a walk.
func ParseStep(step skill.Step) (Step, error) {
	if !DrivenHere(step.Tool) {
		return Step{}, fmt.Errorf("step %d calls the tool %q, and a browser recording holds only %s, %s, and %s",
			step.Number, step.Tool, contract.ToolBrowserOpen, contract.ToolBrowserClick, contract.ToolBrowserType)
	}
	input, err := readStepInput(step)
	if err != nil {
		return Step{}, err
	}

	parsed := Step{
		Number: step.Number,
		Intent: step.Intent,
		Tool:   step.Tool,
		Typed:  input.Text,
		Element: Descriptor{
			Ref:   strings.TrimSpace(input.Element),
			Role:  strings.TrimSpace(input.Role),
			Name:  strings.TrimSpace(input.Name),
			Shown: strings.TrimSpace(input.Shown),
		},
		Address:     strings.TrimSpace(input.URL),
		Expectation: expectationOf(step, input),
	}
	return parsed, checkStep(parsed)
}

// readStepInput reads the JSON a step hands its tool, treating a step with no
// input at all as an empty object rather than as a mistake, so that the one
// refusal a reader sees is the one about what is missing.
func readStepInput(step skill.Step) (stepInput, error) {
	written := strings.TrimSpace(step.Input)
	if written == "" {
		return stepInput{}, nil
	}
	input := stepInput{}
	if err := json.Unmarshal([]byte(written), &input); err != nil {
		return stepInput{}, fmt.Errorf("cannot read the input of step %d as JSON, so write it as {\"element\": \"e12\"}: %w", step.Number, err)
	}
	return input, nil
}

// expectationOf is what the step says should happen. The expect line wins,
// because that is the line a person editing a skill folder would change, and
// the copy inside the input is what the browser tool reads when the ordinary
// runner makes the call.
func expectationOf(step skill.Step, input stepInput) string {
	if written := strings.TrimSpace(step.Expect); written != "" {
		return written
	}
	return strings.TrimSpace(input.Expectation)
}

// checkStep holds what a recorded step must carry to be replayable at all.
func checkStep(step Step) error {
	if step.Expectation == "" {
		return fmt.Errorf("step %d says nothing about what should happen, so add an expect line saying what the page does", step.Number)
	}
	if step.Tool == contract.ToolBrowserOpen {
		if step.Address == "" {
			return fmt.Errorf("step %d opens a page and gives no address, so put the address in its input as a url", step.Number)
		}
		return checkStepFits(step)
	}
	if step.Element.Empty() {
		return fmt.Errorf("step %d names no element to act on, so give its reference, its role and name, or the text it showed", step.Number)
	}
	return checkStepFits(step)
}

// checkStepFits refuses a step that would not fit in a skill folder once it is
// written out. A written step carries its intent and its expectation twice over,
// once for a person to read and once inside the input the browser tool is given,
// so a step that was read from somewhere else can be inside the cap and still be
// too big to save. Refusing it here is what keeps the promise that anything this
// package reads it can also write back.
func checkStepFits(step Step) error {
	written := RenderStep(step)
	size := len(written.Intent) + len(written.Input) + len(written.Expect)
	if size > skill.MaxStepBytes {
		return fmt.Errorf("step %d would be %d bytes written into a skill folder and the most allowed is %d, so shorten what it says or what it types",
			step.Number, size, skill.MaxStepBytes)
	}
	return nil
}

// ParseStepList reads a whole steps.md as browser steps, refusing a list longer
// than one recording may hold.
func ParseStepList(steps []skill.Step) ([]Step, error) {
	if len(steps) > MaxRecordedSteps {
		return nil, fmt.Errorf("this recording has %d steps and the most a skill may hold is %d, so split it into two skills",
			len(steps), MaxRecordedSteps)
	}
	parsed := make([]Step, 0, len(steps))
	for _, step := range steps {
		one, err := ParseStep(step)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, one)
	}
	return parsed, nil
}

// StepsOf reads a skill folder as a browser recording.
func StepsOf(folder skill.Folder) ([]Step, error) {
	if folder.HasScript {
		return nil, fmt.Errorf("the skill %q carries a script rather than steps, so it is not a browser recording and has to be run as a script",
			folder.Definition.Name)
	}
	if len(folder.Steps) == 0 {
		return nil, fmt.Errorf("the skill %q has no steps in it, so there is nothing in the browser to replay", folder.Definition.Name)
	}
	return ParseStepList(folder.Steps)
}

// RenderStep writes one browser step back out as a step of a steps.md.
func RenderStep(step Step) skill.Step {
	written, err := json.Marshal(stepInput{
		Intent:      step.Intent,
		URL:         step.Address,
		Element:     step.Element.Ref,
		Role:        step.Element.Role,
		Name:        step.Element.Name,
		Shown:       step.Element.Shown,
		Text:        step.Typed,
		Expectation: step.Expectation,
	})
	if err != nil {
		written = []byte("{}")
	}
	return skill.Step{
		Number: step.Number,
		Intent: step.Intent,
		Tool:   step.Tool,
		Input:  string(written),
		Expect: step.Expectation,
	}
}

// RenderSteps writes a whole recording out as the steps.md of a skill folder,
// numbering the steps by their place in the list so that a recording with a gap
// in its numbers is still a folder internal/skill will read back.
func RenderSteps(steps []Step) []byte {
	written := make([]skill.Step, 0, len(steps))
	for position, step := range steps {
		step.Number = position + 1
		written = append(written, RenderStep(step))
	}
	return skill.RenderSteps(written)
}
