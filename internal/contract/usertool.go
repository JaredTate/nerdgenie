package contract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// UserToolDescribeFlag is the one argument the harness passes a user's own tool
// to ask it what it is.
//
// The whole of the user-tool protocol is this: put an executable in the tools
// folder under the agent's home; the harness runs it once at startup with this
// flag and reads a JSON tool specification from its standard output; and when
// the model calls the tool, the harness runs it again with one JSON object on
// its standard input and reads the result as plain text from its standard
// output. Nothing else. A user tool goes through the same permission function as
// a built-in one.
const UserToolDescribeFlag = "--describe"

// DecodeUserToolSpec reads the JSON a user's own tool prints when it is asked to
// describe itself, and refuses anything the registry could not use: no name, a
// name with a space in it, no description, a description over the forty-word
// cap, an unknown permission class, or a field with no name.
func DecodeUserToolSpec(described []byte) (ToolSpec, error) {
	var spec ToolSpec
	decoder := json.NewDecoder(strings.NewReader(string(described)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return ToolSpec{}, fmt.Errorf("cannot read the tool description as JSON, so check what %s printed: %w", UserToolDescribeFlag, err)
	}
	if err := checkUserToolSpec(spec); err != nil {
		return ToolSpec{}, err
	}
	return spec, nil
}

// checkUserToolSpec holds the rules a described tool must satisfy, so that
// DecodeUserToolSpec stays short enough to read in one go.
func checkUserToolSpec(spec ToolSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("the tool description has no name, so add a name field to what %s prints", UserToolDescribeFlag)
	}
	if strings.ContainsAny(spec.Name, " \t\r\n") {
		return fmt.Errorf("the tool name %q holds a space, so use one word the model can call", spec.Name)
	}
	if spec.Description == "" {
		return fmt.Errorf("the tool %q has no description, so add one saying when to use it and when not to", spec.Name)
	}
	if words := DescriptionWordCount(spec.Description); words > MaxToolDescriptionWords {
		return fmt.Errorf("the description of %q is %d words, so cut it to %d or fewer", spec.Name, words, MaxToolDescriptionWords)
	}
	for _, class := range spec.Classes {
		if !KnownPermissionClass(class) {
			return fmt.Errorf("the tool %q claims the permission class %q, so use one of R, W, X, N, or I", spec.Name, class)
		}
	}
	for _, field := range spec.Fields {
		if field.Name == "" {
			return fmt.Errorf("an input field of the tool %q has no name, so give every field a name", spec.Name)
		}
	}
	return nil
}
