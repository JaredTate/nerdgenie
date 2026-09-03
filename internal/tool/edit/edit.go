package edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/write"
)

// MaxFileBytes is the biggest file this tool will edit. A file larger than this
// is not one a model can hold enough of in mind to quote a span from, and the
// matchers would walk the whole of it.
const MaxFileBytes = 8 << 20

// PathCheck says whether the tool may change a path and returns it with its
// links followed. The registry hands one in, so that the rule about where the
// agent may write lives in one place rather than in four tools.
type PathCheck func(path string) (string, error)

// Settings is what the edit tool needs to do its work.
type Settings struct {
	// Allowed says whether a path may be changed.
	Allowed PathCheck
	// Log is the event log the prior contents are recorded in.
	Log contract.Store
	// TaskID is the task or job the change belongs to.
	TaskID string
	// Clock is where the time on the event comes from.
	Clock contract.Clock
}

// input is what the model writes when it calls this tool.
type input struct {
	// Path is the whole path of the file to change.
	Path string `json:"path"`
	// Old is the text to replace, quoted from the file.
	Old string `json:"old"`
	// New is what to put in its place.
	New string `json:"new"`
}

// Tool is the edit tool.
type Tool struct {
	settings Settings
	change   write.Change
}

// New returns the edit tool.
func New(settings Settings) *Tool {
	return &Tool{
		settings: settings,
		change:   write.Change{Log: settings.Log, TaskID: settings.TaskID, Clock: settings.Clock},
	}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolEdit,
		Description: "Replaces one span of text in a file with another, inside the folders the agent may work in. " +
			"Quote enough lines to name one place. Use write to replace a whole file.",
		Fields: []contract.ToolField{
			{Name: "path", Type: "string", Description: "The whole path of the file to change.", Required: true},
			{Name: "old", Type: "string", Description: "The text to replace, quoted from the file.", Required: true},
			{Name: "new", Type: "string", Description: "What to put in its place.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassWrite},
	}
}

// Run finds the one span the model quoted, records what the file held, and
// writes the file back with the span replaced.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Allowed == nil {
		return contract.ToolOutput{}, errors.New("this tool has no list of folders it may write in, so wire the sandbox roots in before using it")
	}
	path, err := tool.settings.Allowed(asked.Path)
	if err != nil {
		return contract.ToolOutput{}, err
	}

	before, err := readFile(path)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	after, how, err := Replace(before, asked.Old, asked.New)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot edit %s: %w", path, err)
	}

	mode, err := tool.change.Before(ctx, path)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if err := os.WriteFile(path, []byte(after), mode); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot write %s back after the edit: %w", path, err)
	}
	return contract.ToolOutput{
		Text: fmt.Sprintf("edited %s by the %s matcher; the file now holds %d bytes\n", path, how, len(after)),
	}, nil
}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with a path, old, and new in it: %w", err)
		}
	}
	if strings.TrimSpace(asked.Path) == "" {
		return input{}, errors.New("this call names no file to change, so give the whole path of the file")
	}
	return asked, nil
}

// readFile reads the file to be changed, refusing one this tool has no business
// walking through.
func readFile(path string) (string, error) {
	about, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s to edit it, so check the path and try again: %w", path, err)
	}
	if about.IsDir() {
		return "", fmt.Errorf("%s is a folder, so name a file to edit rather than the folder holding it", path)
	}
	if about.Size() > MaxFileBytes {
		return "", fmt.Errorf("%s holds %d bytes and this tool edits files up to %d, so change it with a command instead",
			path, about.Size(), MaxFileBytes)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s to edit it: %w", path, err)
	}
	return string(held), nil
}
