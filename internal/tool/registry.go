// The idea of one registry that names every tool the model may call, with the
// description kept beside the tool and checked at registration, is OpenCode's,
// at ~/Code/opencode/packages/opencode/src/tool/registry.ts. The Go here is
// written fresh, and the forty-word cap and the result cap are Coeus rules.

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxTools is the most tools one registry will hold. Every description rides in
// the prompt on every call, so the number of them is a cost the user pays each
// turn, and a cap is what stops a folder of forgotten scripts from paying it.
const MaxTools = 64

// ErrDescriptionTooLong is the rule that a tool's description stays under the
// forty-word cap, whether it is built in or the user's own.
var ErrDescriptionTooLong = errors.New("a tool description is capped at forty words, so cut it to say when to use the tool and when not to")

// ErrTooManyTools is the rule that a registry holds at most MaxTools tools.
var ErrTooManyTools = errors.New("this registry already holds as many tools as the model can be asked to read, so take one out before adding another")

// Registry is the whole set of tools available on one turn: the eighteen built
// in and every executable the user dropped into the tools folder. It caps every
// result and never reads one, because a tool result is data and never
// instructions.
type Registry struct {
	guard    sync.Mutex
	order    []string
	tools    map[string]contract.Tool
	settings Settings
}

// NewRegistry returns an empty registry that caps every result at the
// configuration's output cap and spills the rest to a file under the home
// folder's run folder.
func NewRegistry(settings Settings) (*Registry, error) {
	if settings.Home.Root == "" {
		return nil, errors.New("the registry needs the agent's home folder, so pass the home the program was started with")
	}
	return &Registry{tools: map[string]contract.Tool{}, settings: settings}, nil
}

// Add registers one tool under its own name, refusing anything the model could
// not use: a tool with no name, a tool with no description, a description over
// the word cap, a name already taken, or one tool too many.
func (registry *Registry) Add(tool contract.Tool) error {
	if tool == nil {
		return errors.New("this registry was handed nothing to add, so pass a tool that answers Spec and Run")
	}
	spec := tool.Spec()
	if err := checkSpec(spec); err != nil {
		return err
	}

	registry.guard.Lock()
	defer registry.guard.Unlock()
	if _, taken := registry.tools[spec.Name]; taken {
		return fmt.Errorf("the name %q already belongs to a tool in this registry, so give this one a name of its own", spec.Name)
	}
	if len(registry.order) >= MaxTools {
		return fmt.Errorf("the tool %q would be number %d: %w", spec.Name, len(registry.order)+1, ErrTooManyTools)
	}
	registry.order = append(registry.order, spec.Name)
	registry.tools[spec.Name] = tool
	return nil
}

// Specs is what goes into the model's system prompt, in the order the tools were
// added.
func (registry *Registry) Specs() []contract.ToolSpec {
	registry.guard.Lock()
	defer registry.guard.Unlock()
	specs := make([]contract.ToolSpec, 0, len(registry.order))
	for _, name := range registry.order {
		specs = append(specs, registry.tools[name].Spec())
	}
	return specs
}

// Lookup finds one tool by name and hands back a tool whose result is already
// inside the output cap, so that nothing in the loop has to remember to cap one.
func (registry *Registry) Lookup(name string) (contract.Tool, bool) {
	registry.guard.Lock()
	defer registry.guard.Unlock()
	found, held := registry.tools[name]
	if !held {
		return nil, false
	}
	return &cappedTool{registry: registry, inner: found}, true
}

// cappedTool is one registered tool with the registry's output cap applied to
// whatever it returns. The registry never reads the text it passes on, because a
// tool result is data and never instructions.
type cappedTool struct {
	registry *Registry
	inner    contract.Tool
}

// Spec is what the model is told about the tool, unchanged.
func (tool *cappedTool) Spec() contract.ToolSpec { return tool.inner.Spec() }

// Run runs the tool and returns its result inside the output cap, with the whole
// of a long result written to a file the result names.
func (tool *cappedTool) Run(ctx context.Context, input json.RawMessage) (contract.ToolOutput, error) {
	output, err := tool.inner.Run(ctx, input)
	if err != nil {
		return output, err
	}
	return tool.registry.applyOutputCap(output)
}

// checkSpec holds the rules every tool's description must satisfy before the
// model is ever told about it.
func checkSpec(spec contract.ToolSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("this tool has no name, so give it the one word the model calls it by")
	}
	if strings.TrimSpace(spec.Description) == "" {
		return fmt.Errorf("the tool %q has no description, so write one saying when to use it and when not to", spec.Name)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		return fmt.Errorf("the description of %q is %d words: %w", spec.Name, words, ErrDescriptionTooLong)
	}
	for _, class := range spec.Classes {
		if !contract.KnownPermissionClass(class) {
			return fmt.Errorf("the tool %q claims the permission class %q, so use one of R, W, X, N, or I", spec.Name, class)
		}
	}
	return nil
}
