package skill

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The keys a line under a numbered step may carry. Anything else is read as
// more of the step's own words, so a step can explain itself over several
// lines without the reader having to know a syntax.
const (
	toolKey   = "tool"
	inputKey  = "input"
	expectKey = "expect"
)

// Step is one step of a skill: what it is for, which tool it calls, what to
// give that tool, and what the result should hold.
type Step struct {
	// Number is the step's place in the list, starting at one.
	Number int
	// Intent is what the step is for, in the words the writer used.
	Intent string
	// Tool is the tool the step calls, and is empty on a step written in words
	// alone, which replay cannot run without the model.
	Tool string
	// Input is the JSON the tool is given, with the token "{{arguments}}"
	// standing wherever the run's own arguments belong.
	Input string
	// Expect is the text the result must hold for the step to have worked, and
	// is empty when any result will do.
	Expect string
}

// ParseSteps reads a steps.md and returns the steps in it. The numbers have to
// run one, two, three, because a list whose numbers jump has had something cut
// out of it and replaying half a procedure is worse than replaying none.
func ParseSteps(content []byte) ([]Step, error) {
	if len(content) > MaxFileBytes {
		return nil, fmt.Errorf("%s is %d bytes and the most allowed is %d, so shorten it", StepsFile, len(content), MaxFileBytes)
	}

	steps := []Step{}
	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if number, intent, starts := stepHeading(line); starts {
			if len(steps) >= MaxSteps {
				return nil, fmt.Errorf("%s has more than %d steps, so split the skill into two skills", StepsFile, MaxSteps)
			}
			steps = append(steps, Step{Number: number, Intent: intent})
			continue
		}
		if line == "" || len(steps) == 0 {
			continue
		}
		readStepLine(&steps[len(steps)-1], line)
	}
	return steps, checkSteps(steps)
}

// stepHeading reads a line that begins a step, such as "3. Open the page.", and
// says whether the line was one.
func stepHeading(line string) (int, string, bool) {
	digits, rest, separated := strings.Cut(line, ".")
	if !separated || digits == "" {
		return 0, "", false
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 {
		return 0, "", false
	}
	return number, strings.TrimSpace(rest), true
}

// readStepLine reads one line under a numbered step into that step.
func readStepLine(step *Step, line string) {
	key, value, separated := strings.Cut(line, ":")
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	switch {
	case separated && key == toolKey:
		step.Tool = value
	case separated && key == inputKey:
		step.Input = value
	case separated && key == expectKey:
		step.Expect = value
	case step.Intent == "":
		step.Intent = line
	default:
		step.Intent += " " + line
	}
}

// checkSteps holds every bound on a list of steps.
func checkSteps(steps []Step) error {
	for position, step := range steps {
		if step.Number != position+1 {
			return fmt.Errorf("%s numbers a step %d where step %d should be, so number the steps one, two, three", StepsFile, step.Number, position+1)
		}
		if size := len(step.Intent) + len(step.Input) + len(step.Expect); size > MaxStepBytes {
			return fmt.Errorf("step %d of %s is %d bytes and the most allowed is %d, so move the long part into a script", step.Number, StepsFile, size, MaxStepBytes)
		}
		if step.Intent == "" {
			return fmt.Errorf("step %d of %s says nothing about what it is for, so write what the step does after its number", step.Number, StepsFile)
		}
		if step.Tool == "" && step.Input != "" {
			return fmt.Errorf("step %d of %s gives an input but names no tool, so add a tool line saying which tool takes it", step.Number, StepsFile)
		}
		if err := checkStepInput(step); err != nil {
			return err
		}
	}
	return nil
}

// checkStepInput refuses a step whose input is not a JSON object, because the
// input is handed to a tool as the arguments the model would have written.
func checkStepInput(step Step) error {
	if step.Input == "" {
		return nil
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(substituteArguments(step.Input, "")), &fields); err != nil {
		return fmt.Errorf("the input of step %d of %s is not a JSON object, so write it as {\"path\": \"...\"}: %w", step.Number, StepsFile, err)
	}
	return nil
}

// RenderSteps writes steps back out as a steps.md, which is what the learning
// paths use to build a folder they are about to save.
func RenderSteps(steps []Step) []byte {
	lines := []string{}
	for _, step := range steps {
		lines = append(lines, fmt.Sprintf("%d. %s", step.Number, step.Intent))
		if step.Tool != "" {
			lines = append(lines, "   "+toolKey+": "+step.Tool)
		}
		if step.Input != "" {
			lines = append(lines, "   "+inputKey+": "+step.Input)
		}
		if step.Expect != "" {
			lines = append(lines, "   "+expectKey+": "+step.Expect)
		}
		lines = append(lines, "")
	}
	return []byte(strings.Join(lines, "\n"))
}

// DryRunPlan is what test.md says: the arguments to replay the skill with and
// what the whole dry run should have produced by the time it stops.
type DryRunPlan struct {
	// Arguments are what the dry run replays the skill with.
	Arguments string
	// Expect is the text the whole dry run's report must hold, and is empty
	// when every step passing is enough.
	Expect string
}

// ParseTestFile reads a test.md and returns the dry run it describes. Anything
// in the file that is not one of the two keys is prose explaining the dry run
// to a reader, and is passed over.
func ParseTestFile(content []byte) (DryRunPlan, error) {
	if len(content) > MaxFileBytes {
		return DryRunPlan{}, fmt.Errorf("%s is %d bytes and the most allowed is %d, so shorten it", TestFile, len(content), MaxFileBytes)
	}

	plan := DryRunPlan{}
	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		key, value, separated := strings.Cut(strings.TrimSpace(raw), ":")
		if !separated {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "arguments":
			plan.Arguments = strings.TrimSpace(value)
		case expectKey:
			plan.Expect = strings.TrimSpace(value)
		}
	}
	return plan, nil
}

// RenderTestFile writes a dry run back out as a test.md.
func RenderTestFile(name string, plan DryRunPlan) []byte {
	lines := []string{"# the dry run for " + name, "",
		"This runs the steps up to the first one that cannot be undone and stops.", "",
		"arguments: " + plan.Arguments}
	if plan.Expect != "" {
		lines = append(lines, expectKey+": "+plan.Expect)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
