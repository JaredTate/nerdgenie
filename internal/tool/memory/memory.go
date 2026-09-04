package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The three things one call can ask for.
const (
	// ActionSearch finds the facts that match some words.
	ActionSearch = "search"
	// ActionGet brings one fact back by its name.
	ActionGet = "get"
	// ActionSave writes one fact down.
	ActionSave = "save"
)

// MaxFound is how many facts one search brings back. A model that cannot use ten
// will not do better with a hundred.
const MaxFound = 10

// Settings is what the memory tool needs to do its work.
type Settings struct {
	// Memory is what the agent knows across tasks.
	Memory contract.Memory
}

// input is what the model writes when it calls this tool.
type input struct {
	// Action is search, get, or save.
	Action string `json:"action"`
	// Query is the words to search for.
	Query string `json:"query"`
	// Name is the name of one fact, for get.
	Name string `json:"name"`
	// Text is the fact to write down, for save.
	Text string `json:"text"`
	// Source is where the fact came from, for save.
	Source string `json:"source"`
}

// Tool is the memory tool.
type Tool struct {
	settings Settings
}

// New returns the memory tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolMemory,
		Description: "Searches what the agent knows, brings one fact back by its name, or writes a new fact down with its source. " +
			"Use read for a file and search for a folder.",
		Fields: []contract.ToolField{
			{Name: "action", Type: "string", Description: "One of search, get, or save.", Required: true},
			{Name: "query", Type: "string", Description: "The words to search for, when the action is search."},
			{Name: "name", Type: "string", Description: "The name of one fact, when the action is get."},
			{Name: "text", Type: "string", Description: "The fact to write down, when the action is save."},
			{Name: "source", Type: "string", Description: "Where the fact came from, when the action is save."},
		},
		Classes: []contract.PermissionClass{contract.ClassRead, contract.ClassWrite},
	}
}

// Run does what the call asks for, through the memory behind the contract.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Memory == nil {
		return contract.ToolOutput{}, errors.New("this tool has no memory behind it, so wire the memory in before using it")
	}
	switch asked.Action {
	case ActionSearch:
		return tool.search(ctx, asked.Query)
	case ActionGet:
		return tool.get(ctx, asked.Name)
	default:
		return tool.save(ctx, asked)
	}
}

// search finds the facts that match and writes them out, newest first.
func (tool *Tool) search(ctx context.Context, query string) (contract.ToolOutput, error) {
	found, err := tool.settings.Memory.Search(ctx, query, MaxFound)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot search memory for %q: %w", query, err)
	}
	if len(found) == 0 {
		return contract.ToolOutput{Text: "nothing in memory matches those words\n"}, nil
	}
	written := &strings.Builder{}
	for _, fact := range found {
		written.WriteString(factAsLine(fact))
	}
	return contract.ToolOutput{Text: written.String()}, nil
}

// get brings one fact back by its name.
func (tool *Tool) get(ctx context.Context, name string) (contract.ToolOutput, error) {
	fact, err := tool.settings.Memory.Get(ctx, name)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot get the fact %q: %w", name, err)
	}
	return contract.ToolOutput{Text: factAsLine(fact)}, nil
}

// save writes one fact down with the source it came from.
func (tool *Tool) save(ctx context.Context, asked input) (contract.ToolOutput, error) {
	fact := contract.Fact{Text: asked.Text, Source: asked.Source}
	if err := tool.settings.Memory.Save(ctx, []contract.Fact{fact}); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot write the fact down: %w", err)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("written down, from %s\n", asked.Source)}, nil
}

// factAsLine is one fact as the model reads it: its name, its words, and where
// it came from.
func factAsLine(fact contract.Fact) string {
	return fmt.Sprintf("%s: %s (from %s, %s)\n", fact.ID, oneLine(fact.Text), fact.Source,
		fact.Recorded.Format("2006-01-02"))
}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an action in it: %w", err)
		}
	}
	switch asked.Action {
	case ActionSearch:
		if strings.TrimSpace(asked.Query) == "" {
			return input{}, errors.New("this search has nothing to look for, so write the words to search memory for")
		}
	case ActionGet:
		if strings.TrimSpace(asked.Name) == "" {
			return input{}, errors.New("this call names no fact, so give the name of the fact to bring back")
		}
	case ActionSave:
		if strings.TrimSpace(asked.Text) == "" {
			return input{}, errors.New("this call has no fact in it, so write the fact to remember in one line")
		}
		if strings.TrimSpace(asked.Source) == "" {
			return input{}, errors.New("this fact carries no source, so say where it came from, such as the user or a file")
		}
	default:
		return input{}, fmt.Errorf("the action %q is not one this tool knows, so use search, get, or save", asked.Action)
	}
	return asked, nil
}

// oneLine puts text on a single line, because a fact keeps one line.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
