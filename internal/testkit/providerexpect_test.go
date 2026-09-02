package testkit_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theCorrection is the user's words the forty-step fixture sends at round
// twelve. A harness that drops it is the exact failure the expectation catches.
const theCorrection = "no, lead with the date not the features"

// scriptExpectingTheCorrection is a one-step script whose step demands that the
// correction is still somewhere in the request.
func scriptExpectingTheCorrection() testkit.Script {
	return testkit.Script{
		Name:          "provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Expect: []string{theCorrection},
			Text:   "Rewriting the draft so it leads with the date.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 900, OutputTokens: 40},
		}},
	}
}

func TestTheFakeProviderRefusesARequestThatLostWhatTheStepExpects(t *testing.T) {
	for _, row := range []struct {
		name    string
		address func(server *testkit.FakeProviderServer) string
		body    string
	}{
		{
			name:    "the Anthropic shape with the correction gone",
			address: (*testkit.FakeProviderServer).AnthropicAddress,
			body:    `{"model":"opus","system":[{"type":"text","text":"the harness rules"}],"messages":[{"role":"user","content":"post the tweet"}]}`,
		},
		{
			name:    "the OpenAI shape with the correction gone",
			address: (*testkit.FakeProviderServer).OpenAIAddress,
			body:    `{"model":"local-coder","messages":[{"role":"system","content":"the harness rules"},{"role":"user","content":"post the tweet"}]}`,
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			server := testkit.NewFakeProviderServer(scriptExpectingTheCorrection())
			defer server.Close()

			code, _, answer := postJSON(t, row.address(server), row.body)

			if code != http.StatusBadRequest {
				t.Fatalf("a request that dropped the correction answered %d, want 400. It said:\n%s", code, answer)
			}
			if !strings.Contains(answer, theCorrection) {
				t.Errorf("the refusal does not name what the request lost:\n%s", answer)
			}
			if !strings.Contains(answer, "the harness lost it") {
				t.Errorf("the refusal does not say the same thing the fake model says:\n%s", answer)
			}
		})
	}
}

func TestTheFakeProviderPlaysTheStepWhenTheRequestStillCarriesWhatItExpects(t *testing.T) {
	for _, row := range []struct {
		name    string
		address func(server *testkit.FakeProviderServer) string
		body    string
	}{
		{
			name:    "the correction in an Anthropic system block",
			address: (*testkit.FakeProviderServer).AnthropicAddress,
			body:    `{"model":"opus","system":[{"type":"text","text":"corrections: ` + theCorrection + `"}],"messages":[]}`,
		},
		{
			name:    "the correction in an Anthropic user message",
			address: (*testkit.FakeProviderServer).AnthropicAddress,
			body:    `{"model":"opus","messages":[{"role":"user","content":[{"type":"text","text":"` + theCorrection + `"}]}]}`,
		},
		{
			name:    "the correction inside an Anthropic tool result",
			address: (*testkit.FakeProviderServer).AnthropicAddress,
			body:    `{"model":"opus","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"` + theCorrection + `"}]}]}]}`,
		},
		{
			name:    "the correction in an OpenAI system message",
			address: (*testkit.FakeProviderServer).OpenAIAddress,
			body:    `{"model":"local-coder","messages":[{"role":"system","content":"corrections: ` + theCorrection + `"}]}`,
		},
		{
			name:    "the correction in an OpenAI tool message",
			address: (*testkit.FakeProviderServer).OpenAIAddress,
			body:    `{"model":"local-coder","messages":[{"role":"tool","tool_call_id":"call_1","content":"` + theCorrection + `"}]}`,
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			server := testkit.NewFakeProviderServer(scriptExpectingTheCorrection())
			defer server.Close()

			code, _, answer := postJSON(t, row.address(server), row.body)

			if code != http.StatusOK {
				t.Fatalf("a request that carried the correction answered %d, want 200. It said:\n%s", code, answer)
			}
			if !strings.Contains(answer, "Rewriting") {
				t.Errorf("the server did not play the step it was given:\n%s", answer)
			}
		})
	}
}

func TestTheFakeProviderKeepsTheStepItRefusedSoTheNextGoodRequestPlaysIt(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptExpectingTheCorrection())
	defer server.Close()

	if code, _, _ := postJSON(t, server.OpenAIAddress(), `{"messages":[{"role":"user","content":"post it"}]}`); code != http.StatusBadRequest {
		t.Fatalf("the request that lost the correction answered %d, want 400", code)
	}

	code, _, answer := postJSON(t, server.OpenAIAddress(),
		`{"messages":[{"role":"user","content":"`+theCorrection+`"}]}`)

	if code != http.StatusOK {
		t.Fatalf("the good request that followed a refusal answered %d, want 200. It said:\n%s", code, answer)
	}
	if !strings.Contains(answer, "Rewriting") {
		t.Errorf("the refused step was thrown away rather than kept for the next call:\n%s", answer)
	}
}
