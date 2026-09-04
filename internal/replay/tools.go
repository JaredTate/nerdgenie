package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// recordedTools is the tool registry a replay runs against. No tool here
// touches the machine: each one hands back the text the same tool returned when
// the task was recorded, which is what makes a replay safe to run at any time.
//
// It is also where a replay notices that the code has changed its mind. The
// recorded calls are expected in the order the log holds them, so the first
// call the loop makes that is not the one the recording has next is the first
// step where the two runs parted company.
type recordedTools struct {
	// guard keeps one call at a time.
	guard sync.Mutex
	// specs is what the model is told about the tools, built from the names the
	// recording used.
	specs []contract.ToolSpec
	// wanted is every recorded call a tool really saw, in order.
	wanted []Call
	// at is how many of them the replay has used.
	at int
	// difference is the first step where the replay and the recording parted
	// company, and is empty while they agree.
	difference string
}

// newRecordedTools builds the registry of one recording.
//
// Two kinds of recorded call are left out of the answers. The record-writing
// tool is left out because the turn loop applies that one itself when the
// registry does not hold it, which is what keeps a replayed record write going
// through the record's own rules rather than through a recorded answer. A call
// that never reached a tool is left out because there is nothing it can answer
// with: the recording never learned what that tool would have said. Both kinds
// are still offered to the model, because the model has to ask for them again
// for the run to be the same run.
func newRecordedTools(recording Recording) *recordedTools {
	tools := &recordedTools{}
	for _, round := range recording.Rounds {
		for _, call := range round.Calls {
			if call.Name == contract.ToolTask {
				continue
			}
			tools.addSpec(call.Name, recording.TaskID)
			if call.Ran {
				tools.wanted = append(tools.wanted, call)
			}
		}
	}
	return tools
}

// addSpec tells the model about one tool the recording used, once.
func (tools *recordedTools) addSpec(name string, taskID string) {
	if slices.ContainsFunc(tools.specs, func(spec contract.ToolSpec) bool { return spec.Name == name }) {
		return
	}
	tools.specs = append(tools.specs, contract.ToolSpec{
		Name: name,
		Description: fmt.Sprintf("Hands back what %s returned when task %s was recorded, because a replay asks the world nothing.",
			name, taskID),
	})
}

// Specs is what the model is told about the tools of this recording.
func (tools *recordedTools) Specs() []contract.ToolSpec {
	tools.guard.Lock()
	defer tools.guard.Unlock()
	copied := make([]contract.ToolSpec, len(tools.specs))
	copy(copied, tools.specs)
	return copied
}

// Lookup finds one recorded tool by name. A name the recording never used is
// not here, because a replay has no answer to give for it.
func (tools *recordedTools) Lookup(name string) (contract.Tool, bool) {
	tools.guard.Lock()
	defer tools.guard.Unlock()
	for _, spec := range tools.specs {
		if spec.Name == name {
			return recordedTool{tools: tools, spec: spec}, true
		}
	}
	return nil, false
}

// firstDifference reports the first step where the replay and the recording
// parted company, and is empty when they agree. A recorded call the replay
// never made is a difference too, and it is the one a changed rulebook shows up
// as, so the answers left over at the end are counted as well.
func (tools *recordedTools) firstDifference() string {
	tools.guard.Lock()
	defer tools.guard.Unlock()
	if tools.difference == "" && tools.at < len(tools.wanted) {
		return fmt.Sprintf("step %d: the recording ran %q and the replay never did",
			tools.at+1, tools.wanted[tools.at].Name)
	}
	return tools.difference
}

// answer hands back what the next recorded call returned, and writes down the
// first step where the loop asked for something else.
func (tools *recordedTools) answer(name string) (string, error) {
	tools.guard.Lock()
	defer tools.guard.Unlock()

	step := tools.at + 1
	if tools.at >= len(tools.wanted) {
		tools.note(fmt.Sprintf("step %d: the replay asked for %q and the recording has nothing left to answer with", step, name))
		return "", fmt.Errorf("the recording of this task has no answer for a call to %s, because it never made one", name)
	}
	call := tools.wanted[tools.at]
	tools.at++
	if call.Name != name {
		tools.note(fmt.Sprintf("step %d: the replay asked for %q and the recording asked for %q", step, name, call.Name))
	}
	return call.Result, nil
}

// note keeps the first difference and lets the later ones go, because the first
// one is where the two runs parted company and the rest follow from it. The
// caller holds the lock.
func (tools *recordedTools) note(said string) {
	if tools.difference == "" {
		tools.difference = said
	}
}

// recordedTool is one tool of a recording: it takes whatever arguments it is
// given and hands back what the log says came back.
type recordedTool struct {
	// tools is the registry that holds the recorded answers.
	tools *recordedTools
	// spec is what the model was told about this tool.
	spec contract.ToolSpec
}

// Spec is what the model is told about this tool.
func (one recordedTool) Spec() contract.ToolSpec {
	return one.spec
}

// Run hands back the recorded answer. The arguments are ignored, because the
// model wrote them from the recording in the first place.
func (one recordedTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	text, err := one.tools.answer(one.spec.Name)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	return contract.ToolOutput{Text: text}, nil
}
