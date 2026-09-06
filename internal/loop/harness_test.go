package loop_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theStartOfTime is where every test's fake clock starts, so that a task's
// minutes are counted from somewhere a reader can recognise.
var theStartOfTime = time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)

// harness is one loop with every dependency a fake, which is how design section
// 3 is proved a rule at a time.
type harness struct {
	loop       *loop.Loop
	model      *testkit.FakeModel
	tools      *testkit.FakeToolRegistry
	channel    *testkit.FakeChannel
	store      *testkit.FakeStore
	clock      *testkit.FakeClock
	rulings    *testkit.FakePermission
	memory     *testkit.FakeMemory
	skills     *testkit.FakeSkill
	jobs       *fakeJobThatResumes
	sandbox    *testkit.FakeSandbox
	builder    loop.ContextBuilder
	deltaGuard sync.Mutex
	deltas     []string
	lineGuard  sync.Mutex
	lines      []string
	toolGuard  sync.Mutex
	toolLines  []string
	// vision says the loop is built for a model that reads pictures.
	vision bool
	// workFolder is the folder the loop is told the agent works in, a folder
	// of the test's own, which the orientation a fresh window opens with lists.
	workFolder string
}

// newHarness builds a loop over the fakes, with the tools the test needs and the
// task tool the model always sees.
func newHarness(t *testing.T, steps []testkit.Step, tools ...contract.Tool) *harness {
	t.Helper()
	built := &harness{
		model:   testkit.NewFakeModel(testkit.Script{Name: "test", ContextLength: 24000, Steps: steps}),
		channel: testkit.NewFakeChannel("terminal"),
		store:   testkit.NewFakeStore(),
		clock:   testkit.NewFakeClock(theStartOfTime),
		rulings: testkit.NewFakePermission(contract.RulingAllow),
		memory:  testkit.NewFakeMemory(),
		skills:  testkit.NewFakeSkill(),
		sandbox: testkit.NewFakeSandbox(),
	}
	built.workFolder = t.TempDir()
	built.jobs = &fakeJobThatResumes{FakeJob: testkit.NewFakeJob(built.clock)}
	built.tools = testkit.NewFakeToolRegistry(tools...)
	built.builder = theWorkingContext(t)

	made, err := loop.New(built.options())
	if err != nil {
		t.Fatalf("cannot build the loop from the fakes: %v", err)
	}
	built.loop = made
	return built
}

// newHarnessThatSees is newHarness for a model that reads pictures.
func newHarnessThatSees(t *testing.T, steps []testkit.Step, tools ...contract.Tool) *harness {
	t.Helper()
	built := newHarness(t, steps, tools...)
	built.vision = true
	made, err := loop.New(built.options())
	if err != nil {
		t.Fatalf("cannot build the seeing loop from the fakes: %v", err)
	}
	built.loop = made
	return built
}

// options is the dependency set the loop is built from.
func (built *harness) options() loop.Options {
	return built.optionsOver(built.model)
}

// optionsOver is the same dependency set with a model of the test's choosing,
// which is how a test hands the loop a model that behaves in a way no script
// can describe.
func (built *harness) optionsOver(model contract.Model) loop.Options {
	return loop.Options{
		Vision:     built.vision,
		Model:      model,
		Tools:      built.tools,
		Permission: built.rulings,
		Store:      built.store,
		Clock:      built.clock,
		Context:    built.builder,
		Jobs:       built.jobs,
		Memory:     built.memory,
		Skills:     built.skills,
		Sandbox:    built.sandbox,
		Deltas:     built.noteDelta,
		RecordLine: built.noteRecordLine,
		ToolLine:   built.noteToolLine,

		WorkingDirectory: built.workFolder,
	}
}

// noteToolLine records one line about the tool call in flight.
func (built *harness) noteToolLine(line string) {
	built.toolGuard.Lock()
	defer built.toolGuard.Unlock()
	built.toolLines = append(built.toolLines, line)
}

// sentToolLines is every line the loop sent about a tool call.
func (built *harness) sentToolLines() []string {
	built.toolGuard.Lock()
	defer built.toolGuard.Unlock()
	return append([]string(nil), built.toolLines...)
}

// noteRecordLine records one line about a task or a job changing.
func (built *harness) noteRecordLine(line string) {
	built.lineGuard.Lock()
	defer built.lineGuard.Unlock()
	built.lines = append(built.lines, line)
}

// recordLines is every line the loop sent about a task or a job.
func (built *harness) recordLines() []string {
	built.lineGuard.Lock()
	defer built.lineGuard.Unlock()
	return append([]string(nil), built.lines...)
}

// noteDelta records one streamed piece of a reply.
func (built *harness) noteDelta(delta string) {
	built.deltaGuard.Lock()
	defer built.deltaGuard.Unlock()
	built.deltas = append(built.deltas, delta)
}

// streamed is every piece of every reply the loop forwarded.
func (built *harness) streamed() []string {
	built.deltaGuard.Lock()
	defer built.deltaGuard.Unlock()
	return append([]string(nil), built.deltas...)
}

// ask runs one task from a message the way a channel would hand it over.
func (built *harness) ask(t *testing.T, said string) loop.Outcome {
	t.Helper()
	outcome, err := built.loop.Run(t.Context(), built.task(said))
	if err != nil {
		t.Fatalf("the loop could not run the task %q: %v", said, err)
	}
	return outcome
}

// task is one inbound message as the loop takes it.
func (built *harness) task(said string) loop.Task {
	return loop.Task{
		Message: contract.Inbound{ID: "m1", Sender: "the user", Text: said, Channel: "terminal"},
		Channel: built.channel,
	}
}

// held reads one task's record back out of the log, which is how a test sees
// what the harness wrote.
func (built *harness) held(t *testing.T, taskID string) contract.Record {
	t.Helper()
	keeper, err := record.Load(t.Context(), built.store, contract.RecordTask, taskID)
	if err != nil {
		t.Fatalf("cannot load the record of task %s: %v", taskID, err)
	}
	return keeper.Record()
}

// eventsOfKind is every event of one kind the loop wrote.
func (built *harness) eventsOfKind(t *testing.T, kind contract.EventKind) []contract.Event {
	t.Helper()
	found, err := built.store.ByKind(t.Context(), kind)
	if err != nil {
		t.Fatalf("cannot read the %s events out of the log: %v", kind, err)
	}
	return found
}

// theBoundaryTheTestsUse is the tool-result boundary every test here builds
// with, so that a golden line does not change from one run to the next.
const theBoundaryTheTestsUse = "0123456789abcdef"

// theWorkingContext is the real builder from internal/context over a temporary
// home, which is what the loop is driven with everywhere but the tests of the
// builder seam itself.
func theWorkingContext(t *testing.T) loop.ContextBuilder {
	t.Helper()
	home := testkit.NewTempHome(t)
	built, err := workingcontext.New(workingcontext.Options{
		Home:            home,
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

// failingTool is a tool that always returns the error given, which is how the
// three options on an error message are proved.
type failingTool struct {
	name   string
	reason error
	calls  int
}

// Spec is what the model is told about the failing tool.
func (tool *failingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        tool.name,
		Description: "A tool the test made fail, so that the harness's error path can be seen.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run always fails, and counts how many times it was asked to.
func (tool *failingTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	tool.calls++
	return contract.ToolOutput{}, tool.reason
}

// slowTool waits on the clock it was given until its context runs out, which is
// how the tool time limit is proved without waiting on a real clock.
type slowTool struct {
	name  string
	clock contract.Clock
	waits time.Duration
}

// Spec is what the model is told about the slow tool.
func (tool *slowTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        tool.name,
		Description: "A tool the test made slow, so that the tool time limit can be seen.",
		Classes:     []contract.PermissionClass{contract.ClassExecute},
	}
}

// Run waits longer than the loop will allow.
func (tool *slowTool) Run(ctx context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	if err := tool.clock.Sleep(ctx, tool.waits); err != nil {
		return contract.ToolOutput{}, err
	}
	return contract.ToolOutput{Text: "the slow tool finished after all"}, nil
}

// callFor is one tool call as a model writes it.
func callFor(id string, name string, arguments string) contract.ToolCall {
	return contract.ToolCall{ID: id, Name: name, Input: json.RawMessage(arguments)}
}

// taskCall is a call to the task tool, which is how the model writes its half
// of the record in the same reply as its other calls.
func taskCall(id string, arguments string) contract.ToolCall {
	return callFor(id, contract.ToolTask, arguments)
}

// callStep is one scripted reply that asks for tools.
func callStep(orient string, calls ...contract.ToolCall) testkit.Step {
	return testkit.Step{Text: orient, ToolCalls: calls, Finish: contract.FinishToolCalls}
}

// answerStep is one scripted reply that ends the turn.
func answerStep(text string) testkit.Step {
	return testkit.Step{Text: text, Finish: contract.FinishEnd}
}
