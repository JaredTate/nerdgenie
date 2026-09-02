package contract

import (
	"context"
	"encoding/json"
	"strings"
)

// PermissionClass is one letter saying what a tool does, from design section 7.
type PermissionClass string

const (
	// ClassRead means the tool reads.
	ClassRead PermissionClass = "R"
	// ClassWrite means the tool writes.
	ClassWrite PermissionClass = "W"
	// ClassExecute means the tool runs a program.
	ClassExecute PermissionClass = "X"
	// ClassNetwork means the tool uses the network.
	ClassNetwork PermissionClass = "N"
	// ClassIrreversible means the tool does something that cannot be undone.
	ClassIrreversible PermissionClass = "I"
)

// KnownPermissionClass says whether the letter is one of the five.
func KnownPermissionClass(class PermissionClass) bool {
	switch class {
	case ClassRead, ClassWrite, ClassExecute, ClassNetwork, ClassIrreversible:
		return true
	default:
		return false
	}
}

// MaxToolDescriptionWords is the cap the registry enforces on a tool's
// description, because a description the model must read on every call has to
// earn its tokens.
const MaxToolDescriptionWords = 40

// ToolField is one input a tool takes.
type ToolField struct {
	// Name is the field's name in the JSON the model writes.
	Name string `json:"name"`
	// Type is the field's type in plain words, such as "string" or "integer".
	Type string `json:"type"`
	// Description says what to put in it.
	Description string `json:"description"`
	// Required says the tool cannot run without it.
	Required bool `json:"required"`
}

// ToolSpec is the five parts of a tool that the model sees: the name, the
// description, the input fields, and the permission classes. The fifth part, the
// function that runs it, is the Tool interface itself.
type ToolSpec struct {
	// Name is what the model calls, such as "read".
	Name string `json:"name"`
	// Description is plain text under forty words saying when to use the tool
	// and when not to.
	Description string `json:"description"`
	// Fields are the inputs, in the order the description mentions them.
	Fields []ToolField `json:"fields,omitempty"`
	// Classes are the letters saying what the tool does.
	Classes []PermissionClass `json:"classes,omitempty"`
}

// ToolOutput is what one tool run produced.
type ToolOutput struct {
	// Text is the result, already inside the output cap.
	Text string
	// SpillPath names the file holding the rest when the result was over the
	// cap, and is empty when nothing was cut.
	SpillPath string
}

// Tool is one thing the harness knows how to do on the model's behalf.
type Tool interface {
	// Spec is what the model is told about this tool.
	Spec() ToolSpec
	// Run does the work and returns text, or an error the model can act on.
	Run(ctx context.Context, input json.RawMessage) (ToolOutput, error)
}

// ToolRegistry is the whole set of tools available on one turn: the built-in
// ones and any executable the user dropped into the tools folder.
type ToolRegistry interface {
	// Specs is what goes into the model's system prompt.
	Specs() []ToolSpec
	// Lookup finds one tool by name.
	Lookup(name string) (Tool, bool)
}

// DescriptionWordCount counts the words in a tool description the way the
// registry counts them when it applies the forty-word cap.
func DescriptionWordCount(description string) int {
	return len(strings.Fields(description))
}
