package functional

// This file is the functional test for the working context: the forty-step
// fixture is played through a real task record and a real context builder
// against the scripted model, and the model itself is the judge. Every step of
// the script after round twelve refuses the call unless the user's correction is
// still in the request, and every step after round thirty refuses it unless the
// user's reply to the stop is there too. There is no agent loop yet, so the
// smallest possible stand-in wires them together. From wave 3 the same
// assertions run against the real loop through the local socket.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	nerdgeniecontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFortyStepFixtureRunsThroughTheWorkingContext plays the whole fixture on
// a 24k model and checks the three things the fixture exists to check: the ask
// and the correction are byte for byte what the user wrote, every done line
// points at a result, and every result still reads back by its id.
func TestTheFortyStepFixtureRunsThroughTheWorkingContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	turn := newContextTurn(t)
	for _, round := range turn.fixture.Rounds {
		turn.play(ctx, t, round)
	}

	if left := turn.model.StepsLeft(); left != 0 {
		t.Errorf("the model has %d steps of the script left, so the run stopped early", left)
	}
	if err := turn.fixture.CheckAskAndCorrections(turn.keeper.Record()); err != nil {
		t.Errorf("the user's own words did not survive the task: %v", err)
	}
	readBack := func(id string) (string, error) { return turn.keeper.Read(ctx, id) }
	if err := turn.fixture.CheckResultsReadable(readBack); err != nil {
		t.Errorf("a result cannot be read back: %v", err)
	}
	if err := turn.keeper.Apply(ctx, doneLinesOf(turn.fixture)); err != nil {
		t.Fatalf("cannot point the done list at the results that prove it: %v", err)
	}
	if err := turn.fixture.CheckDoneList(turn.keeper.Record()); err != nil {
		t.Errorf("the done-check does not pass at the end of the task: %v", err)
	}
}

// contextTurn is the stand-in loop: a record, a context builder, a scripted
// model, and the conversation so far.
type contextTurn struct {
	fixture  testkit.FortyStepTask
	keeper   *record.Keeper
	builder  *nerdgeniecontext.Builder
	model    *testkit.FakeModel
	messages []contract.Message
}

// newContextTurn wires the fakes and the two real packages together.
func newContextTurn(t *testing.T) *contextTurn {
	t.Helper()
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	home := testkit.NewTempHome(t)
	if err := os.WriteFile(home.SoulFile(), []byte("You are Nerd Genie."), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write SOUL.md: %v", err)
	}
	keeper, err := record.New(t.Context(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: fixture.TaskID, Origin: fixture.Origin,
		Ask: fixture.Ask, RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot open the fixture's task record: %v", err)
	}
	builder, err := nerdgeniecontext.New(nerdgeniecontext.Options{
		Home:            home,
		MemoryCaps:      contract.DefaultConfig().MemoryCaps,
		MaxOutputTokens: 1024,
	})
	if err != nil {
		t.Fatalf("cannot make the working-context builder: %v", err)
	}
	return &contextTurn{fixture: fixture, keeper: keeper, builder: builder, model: testkit.NewFakeModel(fixture.Script())}
}

// play is one turn: the user's message if there is one, the context built from
// the record, the model's reply, and the results written back.
func (turn *contextTurn) play(ctx context.Context, t *testing.T, round testkit.FortyStepRound) {
	t.Helper()
	if round.Number == turn.fixture.CorrectionRound {
		if _, err := turn.keeper.AddCorrection(ctx, turn.fixture.Correction); err != nil {
			t.Fatalf("cannot add the user's correction at round %d: %v", round.Number, err)
		}
		turn.say(turn.fixture.Correction)
	}
	request := turn.build(ctx, t, round.Number)
	reply, err := turn.model.Send(ctx, request, nil)
	if err != nil {
		t.Fatalf("the model refused the request at round %d: %v", round.Number, err)
	}
	turn.write(ctx, t, round)
	turn.messages = append(turn.messages, contract.Message{
		Role: contract.RoleAssistant, Text: reply.Text, ToolCalls: reply.ToolCalls,
	})
	turn.answer(ctx, t, round, reply.ToolCalls)
	if round.Number == turn.fixture.StopRound {
		turn.say(turn.fixture.UserReplyAfterStop)
	}
}

// build makes the working context for one turn on a 24k model, which is the
// window the fixture names for itself.
func (turn *contextTurn) build(ctx context.Context, t *testing.T, round int) contract.Request {
	t.Helper()
	request, err := turn.builder.Build(ctx, nerdgeniecontext.BuildInput{
		ContextLength: turn.fixture.ContextLength,
		Record:        turn.keeper.Record(),
		Messages:      turn.messages,
		Tools:         builtInToolSpecs(),
		MemoryHint:    []string{"Jared posts at 14:00"},
	})
	if err != nil {
		t.Fatalf("cannot build the working context at round %d: %v", round, err)
	}
	return request
}

// write puts the model's half of the record in, which the model asked for with
// the task tool in the same reply as its other calls.
func (turn *contextTurn) write(ctx context.Context, t *testing.T, round testkit.FortyStepRound) {
	t.Helper()
	written, err := round.Update()
	if err != nil {
		t.Fatalf("cannot read what round %d writes into the record: %v", round.Number, err)
	}
	if written.Empty() {
		return
	}
	update := record.Update{Why: written.Why, StopWhen: written.StopWhen, Plan: written.Plan}
	for _, line := range written.DoneWhen {
		update.DoneWhen = append(update.DoneWhen, contract.DoneLine{Text: line})
	}
	if written.Decision != nil {
		update.Decision = &record.NewDecision{Text: written.Decision.Text, Reason: written.Decision.Reason}
	}
	if written.Failure != nil {
		update.Failure = &record.NewFailure{Text: written.Failure.Text, Cause: written.Failure.Cause}
	}
	if err := turn.keeper.Apply(ctx, update); err != nil {
		t.Fatalf("the model's writing at round %d was refused: %v", round.Number, err)
	}
}

// answer writes every result of one round into the record and onto the
// conversation, in the order the model asked for them.
func (turn *contextTurn) answer(ctx context.Context, t *testing.T, round testkit.FortyStepRound, calls []contract.ToolCall) {
	t.Helper()
	produced := turn.fixture.ResultsOfRound(round.Number)
	if len(produced) != len(calls) {
		t.Fatalf("round %d asked for %d tools and the fixture has %d results for it", round.Number, len(calls), len(produced))
	}
	results := []contract.ToolResult{}
	for at, result := range produced {
		id, err := turn.keeper.AddResult(ctx, result.Summary, result.Text)
		if err != nil {
			t.Fatalf("cannot write the result of round %d into the record: %v", round.Number, err)
		}
		if id != result.ID {
			t.Fatalf("the record labelled round %d's result %s, and the fixture calls it %s", round.Number, id, result.ID)
		}
		results = append(results, contract.ToolResult{CallID: calls[at].ID, Text: result.Text})
	}
	if len(results) > 0 {
		turn.messages = append(turn.messages, contract.Message{Role: contract.RoleUser, ToolResults: results})
	}
}

// say puts a message from the user onto the conversation.
func (turn *contextTurn) say(text string) {
	turn.messages = append(turn.messages, contract.Message{Role: contract.RoleUser, Text: text})
}

// doneLinesOf is the done list at the end, every line pointing at the result
// that proves it.
func doneLinesOf(fixture testkit.FortyStepTask) record.Update {
	return record.Update{DoneWhen: fixture.DoneLinesAtTheEnd()}
}

// builtInToolSpecs is one specification per built-in tool, which is what the
// registry of brief 2.5 hands the builder.
func builtInToolSpecs() []contract.ToolSpec {
	specs := []contract.ToolSpec{}
	for _, name := range contract.BuiltInToolNames() {
		specs = append(specs, contract.ToolSpec{
			Name:        name,
			Description: fmt.Sprintf("The %s tool. Use it when the task needs it, and not otherwise.", name),
			Fields: []contract.ToolField{
				{Name: "input", Type: "string", Description: "what to work on", Required: true},
			},
			Classes: []contract.PermissionClass{contract.ClassRead},
		})
	}
	return specs
}
