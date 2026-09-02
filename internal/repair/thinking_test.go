package repair_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/repair"
)

// hiddenCall is a whole tool call, written the way the harness asks for it, of
// the kind a reasoning model writes to itself while it is thinking aloud.
const hiddenCall = contract.ToolCallOpenTag +
	`{"name": "write", "arguments": {"path": "hidden.txt", "content": "hi"}}` +
	contract.ToolCallCloseTag

func TestNothingInsideAThinkBlockIsEverReadAsACall(t *testing.T) {
	cases := map[string]string{
		"one block":                           "<think>" + hiddenCall + "</think>",
		"the tags in capitals":                "<THINK>" + hiddenCall + "</THINK>",
		"the tags in mixed case":              "<Think>" + hiddenCall + "</Think>",
		"a block that is never closed":        "<think>" + hiddenCall,
		"two blocks":                          "<think>" + hiddenCall + "</think>\nand\n<think>" + hiddenCall + "</think>",
		"a fence inside a block":              "<think>\n```json\n" + `{"name": "write", "arguments": {}}` + "\n```\n</think>",
		"a bare object inside a block":        "<think>" + `{"name": "write", "arguments": {"path": "a", "content": "b"}}` + "</think>",
		"a function-call line inside a block": "<think>\n" + `write({"path": "a", "content": "b"})` + "\n</think>",
		"broken JSON inside a block":          "<think>" + contract.ToolCallOpenTag + `{"name": ` + "</think>",
	}

	for about, text := range cases {
		t.Run(about, func(t *testing.T) {
			result := repair.Find(contract.Reply{Text: text}, testSpecs(), 0)
			if len(result.Calls) != 0 {
				t.Fatalf("%s produced %+v, want no calls at all", about, result.Calls)
			}
			if result.Problem != "" {
				t.Errorf("%s produced the problem %q, want none, because thinking is not a call", about, result.Problem)
			}
		})
	}
}

func TestNothingInsideAThinkBlockIsEverTheAnswer(t *testing.T) {
	reply := contract.Reply{Text: "Before.\n<think>the secret is 12345 and " + hiddenCall + "</think>\nAfter."}

	for _, failedParses := range []int{0, 1, 2, 5} {
		result := repair.Find(reply, testSpecs(), failedParses)
		if strings.Contains(result.Text, "secret") || strings.Contains(result.Text, "hidden.txt") {
			t.Errorf("after %d failed parses the answer is %q, want the thinking left out", failedParses, result.Text)
		}
		if !strings.Contains(result.Text, "Before.") || !strings.Contains(result.Text, "After.") {
			t.Errorf("after %d failed parses the answer is %q, want what was written outside the thinking", failedParses, result.Text)
		}
	}
}

func TestTheThinkingIsLeftOutOfTheAnswerBesideAStructuredCall(t *testing.T) {
	reply := contract.Reply{
		Text:      "<think>the secret is 12345</think>\nHere is the file.",
		ToolCalls: []contract.ToolCall{{ID: "call_1", Name: "read", Input: []byte(`{"path": "a.txt"}`)}},
	}

	result := repair.Find(reply, testSpecs(), 0)

	if strings.Contains(result.Text, "secret") {
		t.Errorf("the answer is %q, want the thinking left out", result.Text)
	}
	if result.Text != "Here is the file." {
		t.Errorf("the answer is %q, want what was written outside the thinking", result.Text)
	}
}

func TestACallOutsideAThinkBlockStillRuns(t *testing.T) {
	text := "<think>I will read the notes.</think>\n" +
		contract.ToolCallOpenTag + `{"name": "read", "arguments": {"path": "notes.md"}}` + contract.ToolCallCloseTag +
		"\nDone."

	result := repair.Find(contract.Reply{Text: text}, testSpecs(), 0)

	if result.Problem != "" {
		t.Fatalf("a call written outside the thinking was refused with %q", result.Problem)
	}
	if len(result.Calls) != 1 || result.Calls[0].Name != "read" {
		t.Fatalf("a call written outside the thinking produced %+v, want one call to read", result.Calls)
	}
	if result.Text != "Done." {
		t.Errorf("the answer is %q, want only what was written outside the thinking and the call", result.Text)
	}
}

func TestAClosingThinkTagOnItsOwnIsOrdinaryText(t *testing.T) {
	result := repair.Find(contract.Reply{Text: "The tag </think> is what ends a block."}, testSpecs(), 0)

	if result.Text != "The tag </think> is what ends a block." {
		t.Errorf("the answer is %q, want the sentence unchanged", result.Text)
	}
}
