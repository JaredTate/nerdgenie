package repair_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/repair"
)

func TestOddEnvelopesThatStillHoldOneCall(t *testing.T) {
	cases := map[string]string{
		"a fence inside the block": contract.ToolCallOpenTag + "\n```json\n" +
			`{"name": "read", "arguments": {"path": "a.txt"}}` + "\n```\n" + contract.ToolCallCloseTag,
		"the block written in capitals": strings.ToUpper(contract.ToolCallOpenTag) +
			`{"name": "read", "arguments": {"path": "a.txt"}}` + strings.ToUpper(contract.ToolCallCloseTag),
		"an opening tag written again inside the arguments": contract.ToolCallOpenTag +
			`{"name": "read", "arguments": {"path": "` + contract.ToolCallOpenTag + `"}}` + contract.ToolCallCloseTag,
		"the arguments spread over several lines": contract.ToolCallOpenTag + "\n{\n  \"name\": \"read\",\n" +
			"  \"arguments\": {\n    \"path\": \"a.txt\"\n  }\n}\n" + contract.ToolCallCloseTag,
	}

	for about, text := range cases {
		t.Run(about, func(t *testing.T) {
			result := repair.Find(contract.Reply{Text: text}, testSpecs(), 0)
			if result.Problem != "" {
				t.Fatalf("%s was refused with %q", about, result.Problem)
			}
			if len(result.Calls) != 1 || result.Calls[0].Name != "read" {
				t.Fatalf("%s produced %+v, want one call to read", about, result.Calls)
			}
		})
	}
}

func TestAnEnvelopeThatIsPlainlyACallAndCannotBeReadIsAProblem(t *testing.T) {
	cases := map[string]string{
		"a block holding no JSON at all": contract.ToolCallOpenTag + "please read the notes",
		"a block holding an empty list":  contract.ToolCallOpenTag + "[]" + contract.ToolCallCloseTag,
		"a block holding JSON with no name": contract.ToolCallOpenTag +
			`{"arguments": {"path": "a.txt"}}` + contract.ToolCallCloseTag,
		"a fence holding a call that was never finished": "```json\n" +
			`{"name": "read", "arguments": {` + "\n```",
	}

	for about, text := range cases {
		t.Run(about, func(t *testing.T) {
			result := repair.Find(contract.Reply{Text: text}, testSpecs(), 0)
			if len(result.Calls) != 0 {
				t.Fatalf("%s produced %d calls, want none", about, len(result.Calls))
			}
			if result.Problem == "" {
				t.Errorf("%s produced no problem, want one the model can act on", about)
			}
		})
	}
}

func TestTextThatOnlyLooksALittleLikeACallIsLeftAsText(t *testing.T) {
	cases := map[string]string{
		"a fence in another language":         "```python\nprint(1)\n```",
		"a fence of ordinary JSON":            "```\n{\"model\": \"local\"}\n```",
		"a name with no arguments":            `Here it is.` + "\n" + `{"name": "read"}`,
		"arguments that never close":          `read({"path": "a.txt"`,
		"a sentence with a colon and a brace": "Note: {not json at all}",
	}

	for about, text := range cases {
		t.Run(about, func(t *testing.T) {
			result := repair.Find(contract.Reply{Text: text}, testSpecs(), 0)
			if len(result.Calls) != 0 {
				t.Fatalf("%s produced %d calls, want none", about, len(result.Calls))
			}
			if result.Problem != "" {
				t.Errorf("%s produced the problem %q, want the text left alone", about, result.Problem)
			}
			if result.Text != strings.TrimSpace(text) {
				t.Errorf("%s came back as %q, want the text unchanged", about, result.Text)
			}
		})
	}
}

func TestTheMostTrustedShapePresentIsTheOnlyOneRead(t *testing.T) {
	text := contract.ToolCallOpenTag + `{"name": "read", "arguments": {"path": "a.txt"}}` + contract.ToolCallCloseTag +
		"\n" + `{"name": "write", "arguments": {"path": "b.txt", "content": "hi"}}`

	result := repair.Find(contract.Reply{Text: text}, testSpecs(), 0)

	if len(result.Calls) != 1 || result.Calls[0].Name != "read" {
		t.Fatalf("a reply holding a block and a bare object produced %+v, want only the block", result.Calls)
	}
	if !strings.Contains(result.Text, "write") {
		t.Errorf("the bare object was taken out of the text %q, want it left there as text", result.Text)
	}
}

func TestAReplyWithStructuredCallsIgnoresAnythingWrittenInItsText(t *testing.T) {
	reply := contract.Reply{
		Text:      contract.ToolCallOpenTag + `{"name": "write", "arguments": {}}` + contract.ToolCallCloseTag,
		ToolCalls: []contract.ToolCall{{ID: "call_1", Name: "read", Input: []byte(`{"path": "a.txt"}`)}},
	}

	result := repair.Find(reply, testSpecs(), 0)

	if len(result.Calls) != 1 || result.Calls[0].Name != "read" {
		t.Fatalf("a reply with a structured call produced %+v, want only the structured one", result.Calls)
	}
}
