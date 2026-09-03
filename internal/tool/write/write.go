package write

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/loose"
)

// MaxContentBytes is the most one call may write. A model that asks to write
// more than this in one go has had a run away with it, and a cap is cheaper to
// explain than a full disk.
const MaxContentBytes = 4 << 20

// Settings is what the write tool needs to do its work.
type Settings struct {
	// Allowed says whether a path may be written.
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
	// Path is the path of the file to write.
	Path string `json:"path"`
	// Content is everything the file is to hold afterwards.
	Content string `json:"content"`
}

// Tool is the write tool.
type Tool struct {
	settings Settings
	change   Change
}

// New returns the write tool.
func New(settings Settings) *Tool {
	return &Tool{
		settings: settings,
		change:   Change{Log: settings.Log, TaskID: settings.TaskID, Clock: settings.Clock},
	}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolWrite,
		Description: "Creates a file or replaces everything in one, inside the folders the agent may work in. " +
			"Use edit instead to change part of a file that already exists.",
		Fields: []contract.ToolField{
			{Name: "path", Type: "string", Description: "The path of the file to write, taken from the folder the agent works in unless it starts at the root or at ~.", Required: true},
			{Name: "content", Type: "string", Description: "Everything the file is to hold afterwards.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassWrite},
	}
}

// Run records what the file held and then writes it.
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

	existed, held := fileAsItStands(path)
	mode, err := tool.change.Before(ctx, path)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot make the folder holding %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(asked.Content), mode); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot write %s, so check that the agent may write there: %w", path, err)
	}
	return contract.ToolOutput{Text: saidAndDone(path, len(asked.Content), existed, held)}, nil
}

// The names a model writes for the two fields this tool needs. The first of
// each is the one the specification asks for, and the rest are the names the
// other agents use or a model half-remembers; a call that writes none of them is
// refused rather than acted on, because a write with no content is a file
// emptied.
var (
	pathNames    = []string{"path", "file_path", "filepath", "file", "filename"}
	contentNames = []string{"content", "contents", "text", "body", "file_text", "new_content"}
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "a path and content")
	if err != nil {
		return input{}, err
	}
	path, wrotePath := fields.Text(pathNames...)
	content, wroteContent := fields.Text(contentNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if !wrotePath {
		return input{}, fields.Missing("path", "the path of the file to write")
	}
	if strings.TrimSpace(path) == "" {
		return input{}, errors.New("this call names no file to write, so give the path of the file")
	}
	if !wroteContent {
		return input{}, fields.Missing("content", "everything the file is to hold afterwards")
	}
	if len(content) > MaxContentBytes {
		return input{}, fmt.Errorf("this call would write %d bytes, and one write is capped at %d, so write it in pieces",
			len(content), MaxContentBytes)
	}
	return input{Path: path, Content: content}, nil
}

// fileAsItStands says whether the file is there and how big it is, for the one
// line the tool returns.
func fileAsItStands(path string) (bool, int64) {
	about, err := os.Stat(path)
	if err != nil {
		return false, 0
	}
	return true, about.Size()
}

// saidAndDone is the one line the model reads back, which says what happened and
// nothing else.
func saidAndDone(path string, wrote int, existed bool, held int64) string {
	if existed {
		return fmt.Sprintf("wrote %s, %d bytes, replacing the %d bytes it held\n", path, wrote, held)
	}
	return fmt.Sprintf("created %s, %d bytes\n", path, wrote)
}
