package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The three things one call can ask for.
const (
	// ActionView shows a skill's body.
	ActionView = "view"
	// ActionRun replays a skill.
	ActionRun = "run"
	// ActionSave writes a skill folder.
	ActionSave = "save"
)

// The bounds on saving one skill, because a folder written by a model must not
// be able to fill a disk.
const (
	// MaxFiles is how many files one skill folder may hold.
	MaxFiles = 20
	// MaxFileBytes is how big one of those files may be.
	MaxFileBytes = 256 << 10
)

// Settings is what the skill tool needs to do its work.
type Settings struct {
	// Skills is the store of saved procedures.
	Skills contract.Skill
}

// input is what the model writes when it calls this tool.
type input struct {
	// Action is view, run, or save.
	Action string `json:"action"`
	// Name is the skill's folder name.
	Name string `json:"name"`
	// Query is what to run the skill on, when the action is run.
	Query string `json:"query"`
	// Files are the skill folder's files by name, when the action is save.
	Files map[string]string `json:"files"`
}

// Tool is the skill tool.
type Tool struct {
	settings Settings
}

// New returns the skill tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolSkill,
		Description: "Shows a saved procedure, runs one on the words you give it, or writes a new one down as a folder of files. " +
			"Use shell for a one-off command.",
		Fields: []contract.ToolField{
			{Name: "action", Type: "string", Description: "One of view, run, or save.", Required: true},
			{Name: "name", Type: "string", Description: "The skill's folder name.", Required: true},
			{Name: "query", Type: "string", Description: "What to run the skill on, when the action is run."},
			{Name: "files", Type: "object", Description: "The folder's files by name, when the action is save."},
		},
		Classes: []contract.PermissionClass{contract.ClassRead, contract.ClassExecute, contract.ClassWrite},
	}
}

// Run does what the call asks for, through the skill store behind the contract.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Skills == nil {
		return contract.ToolOutput{}, errors.New("this tool has no skill store behind it, so wire the skills in before using it")
	}
	switch asked.Action {
	case ActionView:
		body, err := tool.settings.Skills.Load(ctx, asked.Name)
		if err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot show the skill %q: %w", asked.Name, err)
		}
		return contract.ToolOutput{Text: body}, nil
	case ActionRun:
		said, err := tool.settings.Skills.Run(ctx, asked.Name, asked.Query)
		if err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot run the skill %q: %w", asked.Name, err)
		}
		return contract.ToolOutput{Text: said}, nil
	default:
		return tool.save(ctx, asked)
	}
}

// save writes a skill folder and says what it wrote.
func (tool *Tool) save(ctx context.Context, asked input) (contract.ToolOutput, error) {
	files := map[string][]byte{}
	names := make([]string, 0, len(asked.Files))
	for name, held := range asked.Files {
		files[name] = []byte(held)
		names = append(names, name)
	}
	if err := tool.settings.Skills.Save(ctx, asked.Name, files); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot write the skill %q: %w", asked.Name, err)
	}
	sort.Strings(names)
	return contract.ToolOutput{
		Text: fmt.Sprintf("wrote the skill %s, holding %s\n", asked.Name, strings.Join(names, ", ")),
	}, nil
}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an action and a name in it: %w", err)
		}
	}
	if !knownAction(asked.Action) {
		return input{}, fmt.Errorf("the action %q is not one this tool knows, so use view, run, or save", asked.Action)
	}
	if strings.TrimSpace(asked.Name) == "" {
		return input{}, errors.New("this call names no skill, so give the folder name of the skill to work with")
	}
	if asked.Action == ActionSave {
		return asked, checkFiles(asked.Files)
	}
	return asked, nil
}

// knownAction says whether the action is one of the three.
func knownAction(action string) bool {
	return action == ActionView || action == ActionRun || action == ActionSave
}

// checkFiles holds the rules a skill folder must satisfy before it is written.
func checkFiles(files map[string]string) error {
	if len(files) == 0 {
		return errors.New("this skill has no files in it, so a folder needs at least a SKILL.md saying what the skill does")
	}
	if len(files) > MaxFiles {
		return fmt.Errorf("this skill has %d files and a folder holds %d, so write fewer of them", len(files), MaxFiles)
	}
	for name, held := range files {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\") {
			return fmt.Errorf("the file name %q is not one a skill folder holds, so use a plain name such as SKILL.md", name)
		}
		if len(held) > MaxFileBytes {
			return fmt.Errorf("the file %s is %d bytes and one file may be %d, so write less in it", name, len(held), MaxFileBytes)
		}
	}
	return nil
}
