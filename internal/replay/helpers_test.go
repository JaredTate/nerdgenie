package replay_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/replay"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theStartOfTime is where every test's fake clock starts, so that a task's
// minutes are counted from a moment a reader can recognise.
var theStartOfTime = time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)

// theBoundaryTheTestsUse is the fixed marker the working context wraps tool
// results in, so that a prompt built twice reads the same both times.
const theBoundaryTheTestsUse = "0123456789abcdef"

// scriptedTask is one task written out for a test: what the model says on each
// call, the tools it may call, and what the permission function rules.
type scriptedTask struct {
	// steps are the model's replies, in order.
	steps []testkit.Step
	// tools are the tools the registry holds.
	tools []contract.Tool
	// rules are the rulings the permission function gives, by tool name.
	rules map[string]contract.PermissionDecision
	// ask is the user's message.
	ask string
	// deliver is a message the user sends while the first tool call runs, and
	// is empty when nobody interrupts the task.
	deliver string
	// caps are the limits the run works inside, and are the defaults when zero.
	caps contract.Caps
}

// recorded is a scripted task that has been run through the real turn loop,
// together with the log it left behind, which is what a recording is read from.
type recorded struct {
	// store is the event log the run wrote.
	store *testkit.FakeStore
	// outcome is where the task ended.
	outcome loop.Outcome
	// channel is the channel the run answered on.
	channel *testkit.FakeChannel
}

// runScript plays one scripted task through the real loop over the fakes.
func runScript(t *testing.T, scripted scriptedTask) recorded {
	t.Helper()
	store := testkit.NewFakeStore()
	channel := testkit.NewFakeChannel("terminal")
	rulings := testkit.NewFakePermission(contract.RulingAllow)
	for name, decision := range scripted.rules {
		rulings.Rule(name, decision)
	}
	interrupt := &interruption{said: scripted.deliver}
	made, err := loop.New(loop.Options{
		Model:      testkit.NewFakeModel(testkit.Script{Name: "test", ContextLength: 24000, Steps: scripted.steps}),
		Tools:      testkit.NewFakeToolRegistry(interrupt.wrap(scripted.tools)...),
		Permission: rulings,
		Store:      store,
		Clock:      testkit.NewFakeClock(theStartOfTime),
		Context:    theWorkingContext(t),
		Caps:       scripted.caps,
	})
	if err != nil {
		t.Fatalf("cannot build the loop that records the fixture: %v", err)
	}
	interrupt.into = made
	outcome, err := made.Run(context.Background(), loop.Task{
		Message: contract.Inbound{ID: "in-1", Text: scripted.ask, Channel: "terminal"},
		Channel: channel,
	})
	if err != nil {
		t.Fatalf("the recorded run did not finish: %v", err)
	}
	return recorded{store: store, outcome: outcome, channel: channel}
}

// runScriptDelivering plays a scripted task while the user sends one message in
// the middle of it, which is how a recorded correction is made.
func runScriptDelivering(t *testing.T, scripted scriptedTask, said string) recorded {
	t.Helper()
	scripted.deliver = said
	return runScript(t, scripted)
}

// interruption is the user typing while the first tool call runs. It holds the
// loop because the loop is built from the tools and so cannot be passed to them.
type interruption struct {
	// said is what the user sends, and is empty when nobody interrupts.
	said string
	// into is the loop the message is delivered to.
	into *loop.Loop
	// sent says the message has already gone, because it is sent once.
	sent bool
}

// wrap puts the interruption in front of the first tool, and hands the tools
// back unchanged when nobody interrupts.
func (interrupt *interruption) wrap(tools []contract.Tool) []contract.Tool {
	if interrupt.said == "" || len(tools) == 0 {
		return tools
	}
	wrapped := make([]contract.Tool, len(tools))
	copy(wrapped, tools)
	wrapped[0] = interruptingTool{interrupt: interrupt, tool: tools[0]}
	return wrapped
}

// interruptingTool is one tool that delivers the user's message the first time
// it is called and then behaves like the tool it wraps.
type interruptingTool struct {
	// interrupt is the message and the loop it goes to.
	interrupt *interruption
	// tool is the tool underneath.
	tool contract.Tool
}

// Spec is the wrapped tool's own specification.
func (wrapping interruptingTool) Spec() contract.ToolSpec {
	return wrapping.tool.Spec()
}

// Run delivers the user's message once and then runs the wrapped tool.
func (wrapping interruptingTool) Run(ctx context.Context, input json.RawMessage) (contract.ToolOutput, error) {
	if !wrapping.interrupt.sent {
		wrapping.interrupt.sent = true
		if err := wrapping.interrupt.into.Deliver(contract.Inbound{
			ID: "in-2", Text: wrapping.interrupt.said, Channel: "terminal",
		}); err != nil {
			return contract.ToolOutput{}, err
		}
	}
	return wrapping.tool.Run(ctx, input)
}

// theWorkingContext builds the real working context over a temporary home, so
// that every test drives the builder the running program drives.
func theWorkingContext(t *testing.T) loop.ContextBuilder {
	t.Helper()
	built, err := workingcontext.New(workingcontext.Options{
		Home:            testkit.NewTempHome(t),
		MemoryCaps:      contract.DefaultConfig().MemoryCaps,
		MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall,
		Boundary:        theBoundaryTheTestsUse,
	})
	if err != nil {
		t.Fatalf("cannot build the working context over a temporary home: %v", err)
	}
	return loop.TheWorkingContext(built)
}

// scriptedTool is one tool that answers with the text given, in order.
func scriptedTool(name string, outputs ...string) contract.Tool {
	return testkit.NewScriptedTool(contract.ToolSpec{
		Name:        name,
		Description: "A tool the test scripted, which answers with what the test gave it.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}, outputs...)
}

// callTo is one tool call written out for a script.
func callTo(id string, name string, arguments string) contract.ToolCall {
	return contract.ToolCall{ID: id, Name: name, Input: json.RawMessage(arguments)}
}

// theReplayOptions is the dependency set every replay in these tests runs with:
// a throwaway log to write into, the real working context, and a rulebook that
// allows everything, which stands for a machine where nothing is in the way.
func theReplayOptions(t *testing.T, from contract.Store) replay.Options {
	t.Helper()
	return replay.Options{
		From:       from,
		Into:       testkit.NewFakeStore(),
		Context:    theWorkingContext(t),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(theStartOfTime),
	}
}
