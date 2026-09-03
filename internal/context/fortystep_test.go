package context

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// fixtureRun plays the forty-step fixture through a real task record and keeps
// the conversation as the turn loop would, so that every test here builds its
// working context from the same task the whole design is proved against.
type fixtureRun struct {
	fixture  testkit.FortyStepTask
	keeper   *record.Keeper
	messages []contract.Message
	played   int
}

// newFixtureRun loads the fixture and opens its task record.
func newFixtureRun(t *testing.T) *fixtureRun {
	t.Helper()
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	keeper, err := record.New(t.Context(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: fixture.TaskID, Origin: fixture.Origin,
		Ask: fixture.Ask, RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot open the fixture's task record: %v", err)
	}
	return &fixtureRun{fixture: fixture, keeper: keeper}
}

// playTo plays the fixture forward to the given round.
func (run *fixtureRun) playTo(t *testing.T, round int) {
	t.Helper()
	for _, playing := range run.fixture.Rounds {
		if playing.Number <= run.played || playing.Number > round {
			continue
		}
		run.playRound(t, playing)
		run.played = playing.Number
	}
}

// playRound puts one round through the record and onto the conversation: the
// user's correction when this is the round it arrives in, what the model wrote
// into the record, the call it made, and the result that came back.
func (run *fixtureRun) playRound(t *testing.T, round testkit.FortyStepRound) {
	t.Helper()
	if round.Number == run.fixture.CorrectionRound {
		if _, err := run.keeper.AddCorrection(t.Context(), run.fixture.Correction); err != nil {
			t.Fatalf("cannot add the user's correction at round %d: %v", round.Number, err)
		}
		run.messages = append(run.messages, contract.Message{Role: contract.RoleUser, Text: run.fixture.Correction})
	}
	run.applyUpdate(t, round)
	if round.ToolName == "" {
		run.messages = append(run.messages, contract.Message{Role: contract.RoleAssistant, Text: round.Reply})
		return
	}
	calls := callsOfRound(t, round)
	run.messages = append(run.messages,
		contract.Message{Role: contract.RoleAssistant, Text: round.Orient, ToolCalls: calls},
		contract.Message{Role: contract.RoleUser, ToolResults: run.resultsOfRound(t, round, calls)},
	)
}

// applyUpdate writes the model's half of the record for one round.
func (run *fixtureRun) applyUpdate(t *testing.T, round testkit.FortyStepRound) {
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
	if err := run.keeper.Apply(t.Context(), update); err != nil {
		t.Fatalf("the model's writing at round %d was refused: %v", round.Number, err)
	}
}

// resultsOfRound writes every result of one round into the record and returns
// them as the tool results the model reads back.
func (run *fixtureRun) resultsOfRound(t *testing.T, round testkit.FortyStepRound, calls []contract.ToolCall) []contract.ToolResult {
	t.Helper()
	results := []contract.ToolResult{}
	for at, produced := range run.fixture.ResultsOfRound(round.Number) {
		id, err := run.keeper.AddResult(t.Context(), produced.Summary, produced.Text)
		if err != nil {
			t.Fatalf("cannot write the result of round %d into the record: %v", round.Number, err)
		}
		if id != produced.ID {
			t.Fatalf("the record labelled round %d's result %s, and the fixture calls it %s", round.Number, id, produced.ID)
		}
		results = append(results, contract.ToolResult{CallID: calls[at].ID, Text: produced.Text})
	}
	return results
}

// callsOfRound is what the model asked for in one round: the round's own tool,
// and the task tool when the round writes to the record, in the same reply.
func callsOfRound(t *testing.T, round testkit.FortyStepRound) []contract.ToolCall {
	t.Helper()
	calls := []contract.ToolCall{{
		ID: fmt.Sprintf("call_%d", round.Number), Name: round.ToolName, Input: asFixtureJSON(t, round.ToolInput),
	}}
	if round.TaskUpdate != nil {
		calls = append(calls, contract.ToolCall{
			ID: fmt.Sprintf("call_%d_task", round.Number), Name: contract.ToolTask, Input: asFixtureJSON(t, round.TaskUpdate),
		})
	}
	return calls
}

// asFixtureJSON writes a tool call's arguments the way the fixture holds them.
func asFixtureJSON(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	written, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("cannot write the fixture's tool arguments as JSON: %v", err)
	}
	return written
}

// input is one turn's worth of build input from where the run has got to.
func (run *fixtureRun) input(contextLength int) BuildInput {
	return BuildInput{
		ContextLength: contextLength,
		Record:        run.keeper.Record(),
		Messages:      run.messages,
		Tools:         sampleTools(),
		MemoryHint:    []string{"Jared posts at 14:00", "one fact per post"},
	}
}
