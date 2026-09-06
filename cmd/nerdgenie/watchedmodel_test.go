package main

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aWatchedModelOver wraps one fake model and counts how many times the screen
// was told the numbers moved.
func aWatchedModelOver(script testkit.Script) (*watchedModel, *int) {
	told := 0
	clock := testkit.NewFakeClock(time.Date(2026, 9, 4, 16, 19, 0, 0, time.UTC))
	watched := newWatchedModel(testkit.NewFakeModel(script), clock, func() { told++ })
	return watched, &told
}

func TestTheWatchedModelCountsWhatACallCostAndTellsTheScreen(t *testing.T) {
	watched, told := aWatchedModelOver(testkit.Script{
		Name:          "local-coder",
		ContextLength: 262144,
		Steps: []testkit.Step{{
			Text:  "here is the answer",
			Usage: contract.Usage{InputTokens: 1200, CachedInputTokens: 800, OutputTokens: 300, CostUSD: 0.02},
		}},
	})

	if watched.Name() != "local-coder" {
		t.Errorf("the watched model calls itself %q, want local-coder", watched.Name())
	}
	if watched.ContextLength() != 262144 {
		t.Errorf("the watched model holds %d tokens, want the window the chain reports", watched.ContextLength())
	}
	if watched.calling() {
		t.Error("the watched model says a call is in flight before any call was made")
	}

	pieces := ""
	reply, err := watched.Send(context.Background(), contract.Request{}, func(delta string) { pieces += delta })
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if reply.Text != "here is the answer" {
		t.Errorf("the reply is %q, want the model's own words", reply.Text)
	}
	if pieces != "here is the answer" {
		t.Errorf("the deltas assembled to %q, want the whole reply, so the counting wrapper dropped the caller's words", pieces)
	}
	if watched.calling() {
		t.Error("the watched model still says a call is in flight after it answered")
	}
	if cost := watched.costSoFar(); cost.InputTokens != 1200 || cost.CachedInputTokens != 800 || cost.OutputTokens != 300 {
		t.Errorf("the session cost is %+v, want the call's own usage", cost)
	}
	// The screen is told at least twice: once when the call began and once when
	// it ended.
	if *told < 2 {
		t.Errorf("the screen was told the numbers moved %d times, want at least the begin and the end", *told)
	}

	fields := map[string]string{}
	watched.fillStatus(fields)
	if fields[contract.StatusFieldContextTokens] != "1200" {
		t.Errorf("the status says the last call held %q tokens, want 1200", fields[contract.StatusFieldContextTokens])
	}
	if fields[contract.StatusFieldCost] == "" {
		t.Error("the status carries no money spent after a call that cost some")
	}
}

func TestTheWatchedModelKeepsTheSessionCountsWhenTheChainIsSwapped(t *testing.T) {
	watched, _ := aWatchedModelOver(testkit.Script{
		Name:          "first",
		ContextLength: 100,
		Steps:         []testkit.Step{{Text: "one", Usage: contract.Usage{InputTokens: 10, OutputTokens: 5}}},
	})
	if _, err := watched.Send(context.Background(), contract.Request{}, nil); err != nil {
		t.Fatalf("the first call failed: %v", err)
	}

	watched.use(testkit.NewFakeModel(testkit.Script{Name: "second", ContextLength: 200}))

	if watched.Name() != "second" {
		t.Errorf("after the swap the model calls itself %q, want second", watched.Name())
	}
	if watched.ContextLength() != 200 {
		t.Errorf("after the swap the window is %d, want the new chain's 200", watched.ContextLength())
	}
	if cost := watched.costSoFar(); cost.InputTokens != 10 {
		t.Errorf("the session cost is %+v after the swap, and the counts are the session's, not the model's", cost)
	}
}

func TestACallThatFailedAddsNothingToTheSessionCost(t *testing.T) {
	// A script with no steps fails the first call, which is how the fake model
	// stands in for a chain that could not answer.
	watched, _ := aWatchedModelOver(testkit.Script{Name: "empty"})

	if _, err := watched.Send(context.Background(), contract.Request{}, nil); err == nil {
		t.Fatal("a call to a model with nothing to say answered without an error")
	}
	if cost := watched.costSoFar(); cost != (contract.CostLine{}) {
		t.Errorf("a failed call added %+v to the session cost, and only a call that answered costs anything", cost)
	}
	if watched.calling() {
		t.Error("the watched model still says a call is in flight after the call failed")
	}
}

// TestTheWatchedModelCountsWhatTheModelWritesUnseen is the strip's count of a
// call in progress: the deltas are counted, and so is what the provider says
// it wrote that no delta shows, its thinking and its tool calls, so that a
// model writing a file for two minutes is not shown as writing nothing.
func TestTheWatchedModelCountsWhatTheModelWritesUnseen(t *testing.T) {
	watched, _ := aWatchedModelOver(testkit.Script{Name: "local-coder", ContextLength: 262144, Steps: []testkit.Step{{Text: "done"}}})

	watched.callBegins()
	watched.counting(nil)("four words of text here")
	watched.countUnseen(400)

	fields := map[string]string{}
	watched.fillStatus(fields)
	if fields[contract.StatusFieldStreamed] != "105" {
		t.Errorf("the status says %q tokens streamed, want 105: 23 characters of text and 400 written unseen, four characters to a token", fields[contract.StatusFieldStreamed])
	}
}

// TestTheWatchedModelReportsTheLastCallsSpeedsAndTheModelFile is what the
// strip's top line and MODEL panel read: which model file is loaded, and how
// fast the last call read its prompt and wrote its answer.
func TestTheWatchedModelReportsTheLastCallsSpeedsAndTheModelFile(t *testing.T) {
	watched, _ := aWatchedModelOver(testkit.Script{
		Name:          "local",
		ContextLength: 131072,
		Steps: []testkit.Step{{
			Text:  "here is the answer",
			Usage: contract.Usage{InputTokens: 4020, CachedInputTokens: 3500, OutputTokens: 40, PromptTokensPerSecond: 320.5, OutputTokensPerSecond: 61.2},
		}},
	})
	watched.describeFile("hauhau-Q4_K_P.gguf")

	if _, err := watched.Send(context.Background(), contract.Request{}, nil); err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	fields := map[string]string{}
	watched.fillStatus(fields)
	if fields[contract.StatusFieldModelFile] != "hauhau-Q4_K_P.gguf" {
		t.Errorf("the status names the model file %q, want hauhau-Q4_K_P.gguf", fields[contract.StatusFieldModelFile])
	}
	if fields[contract.StatusFieldPromptSpeed] != "320" || fields[contract.StatusFieldOutputSpeed] != "61" {
		t.Errorf("the status says prefill %q and output %q tokens a second, want 320 and 61", fields[contract.StatusFieldPromptSpeed], fields[contract.StatusFieldOutputSpeed])
	}
}
