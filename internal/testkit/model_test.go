package testkit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// oneStepScript is the smallest script there is: one reply and nothing else.
func oneStepScript() testkit.Script {
	return testkit.Script{
		Name:          "one-step",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Text:   "The DigiByte anniversary is in January.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 120, CachedInputTokens: 100, OutputTokens: 9},
		}},
	}
}

func TestTheFakeModelPlaysItsScriptInOrder(t *testing.T) {
	ctx := context.Background()
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "two-step",
		ContextLength: 24000,
		Steps: []testkit.Step{
			{Text: "first", ToolCalls: []contract.ToolCall{{ID: "c1", Name: contract.ToolRead, Input: json.RawMessage(`{}`)}}, Finish: contract.FinishToolCalls},
			{Text: "second", Finish: contract.FinishEnd},
		},
	})

	first, err := model.Send(ctx, contract.Request{}, nil)
	if err != nil {
		t.Fatalf("the first call failed: %v", err)
	}
	if first.Text != "first" || len(first.ToolCalls) != 1 {
		t.Errorf("the first reply is %+v, want the first step of the script", first)
	}

	second, err := model.Send(ctx, contract.Request{}, nil)
	if err != nil {
		t.Fatalf("the second call failed: %v", err)
	}
	if second.Text != "second" || second.Finish != contract.FinishEnd {
		t.Errorf("the second reply is %+v, want the second step of the script", second)
	}
}

func TestTheFakeModelStreamsItsTextInDeltas(t *testing.T) {
	model := testkit.NewFakeModel(oneStepScript())

	deltas := []string{}
	reply, err := model.Send(context.Background(), contract.Request{}, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if len(deltas) < 2 {
		t.Errorf("the reply arrived in %d deltas, want it broken into more than one", len(deltas))
	}
	if strings.Join(deltas, "") != reply.Text {
		t.Errorf("the deltas joined to %q, want the reply text %q", strings.Join(deltas, ""), reply.Text)
	}
}

func TestTheFakeModelRefusesARequestThatLostSomethingTheStepExpected(t *testing.T) {
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "expects-a-correction",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Expect: []string{"no, lead with the date not the features"},
			Text:   "Rewriting with the date first.",
			Finish: contract.FinishEnd,
		}},
	})

	_, err := model.Send(context.Background(), contract.Request{
		SystemBlocks: []contract.SystemBlock{{Name: "record", Text: "## Rules\nCorrections:\n"}},
	}, nil)
	if err == nil {
		t.Fatal("a request that lost the correction was accepted, want an error naming what was missing")
	}
	if !strings.Contains(err.Error(), "no, lead with the date not the features") {
		t.Errorf("the error is %q, want it to name the text that was missing", err)
	}
}

func TestTheFakeModelAcceptsARequestThatCarriesWhatTheStepExpected(t *testing.T) {
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "expects-a-correction",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Expect: []string{"no, lead with the date not the features", "r3"},
			Text:   "Rewriting with the date first.",
			Finish: contract.FinishEnd,
		}},
	})

	_, err := model.Send(context.Background(), contract.Request{
		SystemBlocks: []contract.SystemBlock{{Name: "record", Text: "C1 \"no, lead with the date not the features\""}},
		Messages:     []contract.Message{{Role: contract.RoleUser, ToolResults: []contract.ToolResult{{CallID: "r3", Text: "r3 read memory/product.md"}}}},
	}, nil)
	if err != nil {
		t.Fatalf("a request carrying everything the step expected was refused: %v", err)
	}
}

func TestTheFakeModelSaysSoWhenItsScriptRunsOut(t *testing.T) {
	model := testkit.NewFakeModel(oneStepScript())

	if _, err := model.Send(context.Background(), contract.Request{}, nil); err != nil {
		t.Fatalf("the first call failed: %v", err)
	}
	_, err := model.Send(context.Background(), contract.Request{}, nil)
	if err == nil {
		t.Fatal("calling past the end of the script was reported as a success, want an error saying the script ran out")
	}
	if !strings.Contains(err.Error(), "one-step") {
		t.Errorf("the error is %q, want it to name the script", err)
	}
}

func TestTheFakeModelRecordsEveryRequestItWasGiven(t *testing.T) {
	model := testkit.NewFakeModel(oneStepScript())

	request := contract.Request{SystemBlocks: []contract.SystemBlock{{Name: "rules", Text: "the harness rules", Boundary: contract.CacheBoundaryA}}}
	if _, err := model.Send(context.Background(), request, nil); err != nil {
		t.Fatalf("the call failed: %v", err)
	}

	seen := model.Requests()
	if len(seen) != 1 {
		t.Fatalf("the fake recorded %d requests, want 1", len(seen))
	}
	if len(seen[0].SystemBlocks) != 1 || seen[0].SystemBlocks[0].Boundary != contract.CacheBoundaryA {
		t.Errorf("the recorded request is %+v, want the one that was sent", seen[0])
	}
}

func TestTheFakeModelReportsItsNameAndItsWindow(t *testing.T) {
	model := testkit.NewFakeModel(oneStepScript())

	if model.Name() != "one-step" {
		t.Errorf("the model is named %q, want the script's name", model.Name())
	}
	if model.ContextLength() != 24000 {
		t.Errorf("the model holds %d tokens, want the script's context length", model.ContextLength())
	}
	if model.StepsLeft() != 1 {
		t.Errorf("the script has %d steps left, want 1", model.StepsLeft())
	}
}

func TestTheFakeModelKeepsTheModelContract(t *testing.T) {
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "contract-check",
		ContextLength: 24000,
		Steps:         []testkit.Step{{Text: "an answer for the contract check", Finish: contract.FinishEnd}},
	})
	if err := testkit.CheckModel(context.Background(), model); err != nil {
		t.Fatalf("the fake model does not keep the model contract: %v", err)
	}
}
