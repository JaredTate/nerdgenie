package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheCostLineMatchesWhatTheModelReported proves the number the user sees in
// the record's header is the provider's own count, not the builder's estimate.
// The estimate sizes the window; only the provider knows what a call cost.
func TestTheCostLineMatchesWhatTheModelReported(t *testing.T) {
	spent := contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}
	model := testkit.NewFakeModel(testkit.Script{
		Name: "costly", ContextLength: 24000,
		Steps: []testkit.Step{{Text: "Reading the notes.", Finish: contract.FinishEnd, Usage: spent}},
	})
	keeper := newTestKeeper(t)

	reply, err := model.Send(t.Context(), contract.Request{}, nil)
	if err != nil {
		t.Fatalf("the fake model refused the call: %v", err)
	}
	if err := WriteCostLine(t.Context(), keeper, reply.Usage); err != nil {
		t.Fatalf("cannot write the cost line: %v", err)
	}

	wanted := contract.CostLine{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}
	if held := keeper.Record().Header.Cost; held != wanted {
		t.Errorf("the record holds the cost line %+v, and the model reported %+v", held, wanted)
	}
	line := "this turn: 6.1k tokens in, 5.2k of them cached, 0.4k out"
	if !strings.Contains(keeper.Text(), line) {
		t.Errorf("the record's header does not read %q:\n%s", line, keeper.Text())
	}
}

// TestTheCostLineIsWrittenEveryTurn proves a second call overwrites the first,
// because the line says what this turn cost rather than what the task has cost
// so far.
func TestTheCostLineIsWrittenEveryTurn(t *testing.T) {
	keeper := newTestKeeper(t)
	for _, spent := range []contract.Usage{
		{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400},
		{InputTokens: 7200, CachedInputTokens: 6100, OutputTokens: 300},
	} {
		if err := WriteCostLine(t.Context(), keeper, spent); err != nil {
			t.Fatalf("cannot write the cost line: %v", err)
		}
	}
	if held := keeper.Record().Header.Cost.InputTokens; held != 7200 {
		t.Errorf("the cost line says %d tokens in, and the last turn read 7200", held)
	}
}

// TestTheCostLineNeedsARecordToWriteInto proves a caller with no record is told
// so rather than losing the count in silence.
func TestTheCostLineNeedsARecordToWriteInto(t *testing.T) {
	if err := WriteCostLine(t.Context(), nil, contract.Usage{}); err == nil {
		t.Error("writing the cost line with no record was allowed")
	}
}

// newTestKeeper makes a task record over a fake event log.
func newTestKeeper(t *testing.T) *record.Keeper {
	t.Helper()
	keeper, err := record.New(t.Context(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: "17", Origin: "Signal",
		Ask: "Post a tweet about the DigiByte anniversary.", RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot make a task record: %v", err)
	}
	return keeper
}
