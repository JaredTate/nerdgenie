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
)

// MaxContentBytes is the most one call may write. A model that asks to write
// more than this in one go has had a run away with it, and a cap is cheaper to
// explain than a full disk.
const MaxContentBytes = 4 << 20

// PathCheck says whether the tool may write a path and returns it with its links
// followed. The registry hands one in, so that the rule about where the agent
// may write lives in one place rather than in four tools.
type PathCheck func(path string) (string, error)

// Settings is what the write tool needs to do its work.
type Settings struct {
	// Allowed says whether a path may be written.
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
	// Path is the whole path of the file to write.
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
			{Name: "path", Type: "string", Description: "The whole path of the file to write.", Required: true},
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

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with a path and content in it: %w", err)
		}
	}
	if strings.TrimSpace(asked.Path) == "" {
		return input{}, errors.New("this call names no file to write, so give the whole path of the file")
	}
	if len(asked.Content) > MaxContentBytes {
		return input{}, fmt.Errorf("this call would write %d bytes, and one write is capped at %d, so write it in pieces",
			len(asked.Content), MaxContentBytes)
	}
	return asked, nil
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
