package edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
	"github.com/JaredTate/nerdgenie/internal/tool/write"
)

// MaxFileBytes is the biggest file this tool will edit. A file larger than this
// is not one a model can hold enough of in mind to quote a span from, and the
// matchers would walk the whole of it.
const MaxFileBytes = 8 << 20

// Settings is what the edit tool needs to do its work.
type Settings struct {
	// Allowed says whether a path may be changed.
	Allowed func(path string) (string, error)
	// Log is the event log the prior contents are recorded in.
	Log contract.Store
	// TaskID is the task or job the change belongs to.
	TaskID string
	// Clock is where the time on the event comes from.
	Clock contract.Clock
}

// input is what the model writes when it calls this tool.
type input struct {
	// Path is the path of the file to change.
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
			{Name: "path", Type: "string", Description: "The path of the file to change, taken from the folder the agent works in unless it starts at the root or at ~.", Required: true},
			{Name: "old", Type: "string", Description: "The text to replace, quoted from the file.", Required: true},
			{Name: "new", Type: "string", Description: "What to put in its place.", Required: true},
			{Name: "expect", Type: "string", Description: "What the change should show, checked by the harness: parses, tests 62 to 63, all passing, or 2 failing."},
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
		return contract.ToolOutput{}, fmt.Errorf("cannot edit the file %s: %w", path, err)
	}
	if write.StartsAHeadlessBrowser(after) && !write.StartsAHeadlessBrowser(before) {
		return contract.ToolOutput{}, errors.New(write.TheOnScreenBrowserLine)
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

// The names a model writes for the three fields this tool needs. The first of
// each is the one the specification asks for; old_string and new_string are what
// Claude Code and OpenCode call them, so a model that half-remembers one of those
// writes them here, and a call that names neither is refused rather than acted
// on, because an edit with no new text is a span deleted.
var (
	pathNames = []string{"path", "file_path", "filepath", "file", "filename"}
	oldNames  = []string{"old", "old_string", "old_text", "old_str", "search", "find", "target"}
	newNames  = []string{"new", "new_string", "new_text", "new_str", "replace", "replacement", "with"}
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "a path, old, and new")
	if err != nil {
		return input{}, err
	}
	path, wrotePath := fields.Text(pathNames...)
	old, wroteOld := fields.Text(oldNames...)
	replacement, wroteNew := fields.Text(newNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if !wrotePath {
		return input{}, fields.Missing("path", "the path of the file to change")
	}
	if strings.TrimSpace(path) == "" {
		return input{}, errors.New("this call names no file to change, so give the path of the file")
	}
	if !wroteOld {
		return input{}, fields.Missing("old", "the text to replace, quoted from the file")
	}
	if !wroteNew {
		return input{}, fields.Missing("new", "what to put in its place")
	}
	return input{Path: path, Old: old, New: replacement}, nil
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
