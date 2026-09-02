// Turning a bad tool call into something the model can read and fix, rather than
// into a crash, is OpenCode's design, read from its invalid-call path at
// ~/Code/opencode/packages/opencode/src/tool/invalid.ts and written fresh here.

package repair

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// Every problem in this file is written to be read by a model rather than by a
// person: it says what went wrong, it says what to write instead, and it ends
// with the names of the tools that really exist, because a model that has
// invented a tool needs the real list more than it needs an apology.

// problemUnknownName is what a name that matches no real tool produces.
func problemUnknownName(written string, specs []contract.ToolSpec) string {
	return fmt.Sprintf("The tool name %q is not one of the real tools. Write the name exactly as it is spelled here. The real tools are: %s.",
		written, listNames(specs))
}

// problemAmbiguousName is what a name that is as close to one real tool as it is
// to another produces, because guessing between them would run the wrong tool.
func problemAmbiguousName(written string, nearest []string, specs []contract.ToolSpec) string {
	return fmt.Sprintf("The tool name %q is equally close to more than one real tool: %s. Write the whole name of the one you meant. The real tools are: %s.",
		written, strings.Join(nearest, ", "), listNames(specs))
}

// problemUnreadable is what an envelope that was plainly a tool call and would
// not parse produces.
func problemUnreadable(specs []contract.ToolSpec) string {
	return fmt.Sprintf("A tool call was written as text but its JSON could not be read. Write it as %s{\"name\": \"the tool's name\", \"arguments\": {}}%s with valid JSON inside. The real tools are: %s.",
		contract.ToolCallOpenTag, contract.ToolCallCloseTag, listNames(specs))
}

// problemArguments is what arguments that are not a JSON object produce, and it
// names the fields the tool takes so that the next try has something to aim at.
func problemArguments(spec contract.ToolSpec, specs []contract.ToolSpec) string {
	return fmt.Sprintf("The arguments for %q must be a JSON object. %s Write the arguments as a JSON object. The real tools are: %s.",
		spec.Name, describeFields(spec), listNames(specs))
}

// describeFields writes the tool's input fields as one sentence.
func describeFields(spec contract.ToolSpec) string {
	if len(spec.Fields) == 0 {
		return fmt.Sprintf("The %s tool takes no fields.", spec.Name)
	}
	described := make([]string, 0, len(spec.Fields))
	for _, field := range spec.Fields {
		shape := field.Type
		if field.Required {
			shape += ", required"
		}
		described = append(described, fmt.Sprintf("%s (%s) - %s", field.Name, shape, field.Description))
	}
	return fmt.Sprintf("The %s tool takes these fields: %s.", spec.Name, strings.Join(described, "; "))
}

// listNames writes the names of the real tools, and says so plainly when there
// are none, because "the real tools are:" followed by nothing helps no one.
func listNames(specs []contract.ToolSpec) string {
	if len(specs) == 0 {
		return "none, so answer the user in plain text instead"
	}
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return strings.Join(names, ", ")
}
