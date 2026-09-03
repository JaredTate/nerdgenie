package context

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestTheEstimateCountsCharactersAtTheOneRatio proves the estimate is the ratio
// this package documents and nothing else, and that it rounds up, so that a
// short piece of text never counts as no tokens at all.
func TestTheEstimateCountsCharactersAtTheOneRatio(t *testing.T) {
	for _, check := range []struct {
		text   string
		wanted int
	}{
		{"", 0},
		{"a", 1},
		{strings.Repeat("a", CharactersPerToken), 1},
		{strings.Repeat("a", CharactersPerToken+1), 2},
		{strings.Repeat("a", CharactersPerToken*250), 250},
	} {
		if counted := EstimateTokens(check.text); counted != check.wanted {
			t.Errorf("%d characters count as %d tokens, want %d", len(check.text), counted, check.wanted)
		}
	}
}

// TestTheRequestEstimateCountsEveryPartTheModelReads proves nothing the model
// reads is left out of the count, because a part left out is a part the window
// rule does not pay for.
func TestTheRequestEstimateCountsEveryPartTheModelReads(t *testing.T) {
	request := contract.Request{
		SystemBlocks: []contract.SystemBlock{{Name: "instructions", Text: strings.Repeat("s", 400)}},
		Messages: []contract.Message{{
			Role:        contract.RoleAssistant,
			Text:        strings.Repeat("m", 400),
			ToolCalls:   []contract.ToolCall{{ID: "call_1", Name: "read", Input: json.RawMessage(`{"path":"a"}`)}},
			ToolResults: []contract.ToolResult{{CallID: "call_1", Text: strings.Repeat("r", 400)}},
		}},
		Tools: []contract.ToolSpec{{
			Name:        "read",
			Description: strings.Repeat("d", 400),
			Fields:      []contract.ToolField{{Name: "path", Type: "string", Description: strings.Repeat("f", 400)}},
		}},
	}

	whole := EstimateRequestTokens(request)
	for _, part := range []struct {
		name    string
		smaller contract.Request
	}{
		{"the system blocks", contract.Request{Messages: request.Messages, Tools: request.Tools}},
		{"the messages", contract.Request{SystemBlocks: request.SystemBlocks, Tools: request.Tools}},
		{"the tool specifications", contract.Request{SystemBlocks: request.SystemBlocks, Messages: request.Messages}},
	} {
		if EstimateRequestTokens(part.smaller) >= whole {
			t.Errorf("taking %s out of the request does not change the estimate, so they are not counted", part.name)
		}
	}
}

// TestTheEstimateNeverCountsBelowZero proves an empty request costs nothing,
// which is the case the window rule meets on the very first turn.
func TestTheEstimateNeverCountsBelowZero(t *testing.T) {
	if counted := EstimateRequestTokens(contract.Request{}); counted != 0 {
		t.Errorf("an empty request counts as %d tokens, want none", counted)
	}
}

// FuzzTheEstimateReadsAnyBytes proves the estimate never panics and never
// returns a negative count, whatever text a tool returned.
func FuzzTheEstimateReadsAnyBytes(f *testing.F) {
	f.Add("")
	f.Add("a plain line of English")
	f.Add("\x00\xff\xfe")
	f.Add("日本語のテキスト")
	f.Fuzz(func(t *testing.T, text string) {
		if counted := EstimateTokens(text); counted < 0 {
			t.Errorf("%q counts as %d tokens, and no text costs less than nothing", text, counted)
		}
	})
}
