package repair_test

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/repair"
)

// oneWrittenCall is a whole tool call written as text, on one line.
const oneWrittenCall = contract.ToolCallOpenTag +
	`{"name": "read", "arguments": {"path": "a.txt"}}` +
	contract.ToolCallCloseTag

func TestACallInsideTheSearchedTextIsFound(t *testing.T) {
	filler := strings.Repeat("x", repair.MaxSearchedBytes-len(oneWrittenCall)-1) + "\n"

	result := repair.Find(contract.Reply{Text: filler + oneWrittenCall}, testSpecs(), 0)

	if len(result.Calls) != 1 {
		t.Fatalf("a call ending exactly at the searching cap produced %d calls, want one", len(result.Calls))
	}
}

func TestTextPastTheSearchingCapIsNotSearched(t *testing.T) {
	filler := strings.Repeat("x", repair.MaxSearchedBytes)

	result := repair.Find(contract.Reply{Text: filler + "\n" + oneWrittenCall}, testSpecs(), 0)

	if len(result.Calls) != 0 {
		t.Fatalf("a call written past the searching cap produced %d calls, want none", len(result.Calls))
	}
	if result.Problem != "" {
		t.Errorf("text past the cap produced the problem %q, want none, because it was never read", result.Problem)
	}
	if !strings.Contains(result.Text, contract.ToolCallOpenTag) {
		t.Error("the text past the cap was dropped, want it kept as part of the answer")
	}
}

func TestCuttingAtTheSearchingCapKeepsWholeCharacters(t *testing.T) {
	// Two bytes per character means the cap falls in the middle of one of them.
	long := strings.Repeat("é", repair.MaxSearchedBytes)

	result := repair.Find(contract.Reply{Text: long}, testSpecs(), 0)

	if !utf8.ValidString(result.Text) {
		t.Error("cutting the reply at the searching cap split a character in half")
	}
}

func TestMoreCallsThanTheCapAreTruncatedWithANote(t *testing.T) {
	limit := contract.DefaultConfig().Caps.IdenticalCallWindow
	written := &strings.Builder{}
	for round := range limit + 5 {
		written.WriteString(contract.ToolCallOpenTag)
		written.WriteString(`{"name": "read", "arguments": {"path": "` + strconv.Itoa(round) + `.txt"}}`)
		written.WriteString(contract.ToolCallCloseTag + "\n")
	}

	result := repair.Find(contract.Reply{Text: written.String()}, testSpecs(), 0)

	if len(result.Calls) != limit {
		t.Fatalf("a reply asking for %d calls produced %d, want the cap of %d", limit+5, len(result.Calls), limit)
	}
	if result.Problem != "" {
		t.Errorf("truncating produced the problem %q, want the calls that fit and a note", result.Problem)
	}
	if !strings.Contains(result.Note, strconv.Itoa(limit)) {
		t.Errorf("the note is %q, want it to say how many calls were kept", result.Note)
	}
}

func TestUnbalancedBracesAreRefusedRatherThanSearchedForever(t *testing.T) {
	result := repair.Find(contract.Reply{Text: strings.Repeat("{", 20000)}, testSpecs(), 0)

	if len(result.Calls) != 0 {
		t.Fatalf("a run of opening braces produced %d calls, want none", len(result.Calls))
	}
	if result.Problem != "" {
		t.Errorf("a run of opening braces produced the problem %q, want none, because it never looked like a call", result.Problem)
	}
}

func TestThinkingThatIsStillOpenAtTheSearchingCapSwallowsTheTail(t *testing.T) {
	filler := strings.Repeat("x", repair.MaxSearchedBytes)

	result := repair.Find(contract.Reply{Text: "Before.\n<think>" + filler + "\nstill thinking"}, testSpecs(), 0)

	if result.Text != "Before." {
		t.Errorf("the answer is %d characters long, want only what was written before the thinking began", len(result.Text))
	}
}

func TestThinkingThatClosesInsideTheSearchedTextKeepsTheTail(t *testing.T) {
	result := repair.Find(contract.Reply{Text: "Before.\n<think>hidden</think>\nAfter."}, testSpecs(), 0)

	if result.Text != "Before.\nAfter." {
		t.Errorf("the answer is %q, want what was written on either side of the thinking", result.Text)
	}
}

func TestAnEmptyReplyProducesNothingAtAll(t *testing.T) {
	result := repair.Find(contract.Reply{}, testSpecs(), 0)

	if len(result.Calls) != 0 || result.Text != "" || result.Problem != "" || result.Note != "" {
		t.Errorf("an empty reply produced %+v, want an empty result", result)
	}
}
