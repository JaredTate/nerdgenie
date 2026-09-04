package edit_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/edit"
)

func TestAnExactSpanIsReplaced(t *testing.T) {
	after, how, err := edit.Replace("alpha\nbeta\ngamma\n", "beta", "delta")
	if err != nil {
		t.Fatalf("an exact span was not replaced: %v", err)
	}
	if after != "alpha\ndelta\ngamma\n" {
		t.Errorf("the text now reads %q, want the span replaced", after)
	}
	if how != edit.MatchExact {
		t.Errorf("the span was found by the %s matcher, want the exact one", how)
	}
}

func TestTrailingSpacesInTheFileDoNotStopAnEdit(t *testing.T) {
	before := "func main() {\n\tbeta()   \n\tgamma()  \n}\n"
	after, how, err := edit.Replace(before, "\tbeta()\n\tgamma()", "\tdelta()")
	if err != nil {
		t.Fatalf("a span whose lines carry trailing spaces was not replaced: %v", err)
	}
	if !strings.Contains(after, "\tdelta()\n}") {
		t.Errorf("the text now reads %q, want the two lines replaced by one", after)
	}
	if how != edit.MatchTrimmedLines {
		t.Errorf("the span was found by the %s matcher, want the trimmed-lines one", how)
	}
}

func TestSpacingInsideALineDoesNotStopAnEdit(t *testing.T) {
	after, how, err := edit.Replace("let  x   =  1\nlet y = 2\n", "let x = 1", "let x = 9")
	if err != nil {
		t.Fatalf("a line written with different spacing was not replaced: %v", err)
	}
	if after != "let x = 9\nlet y = 2\n" {
		t.Errorf("the text now reads %q, want the first line replaced", after)
	}
	if how != edit.MatchWhitespaceNormalized {
		t.Errorf("the span was found by the %s matcher, want the whitespace-normalized one", how)
	}
}

func TestABlockAtADifferentIndentationIsStillFound(t *testing.T) {
	before := "func run() {\n        if ok {\n            return 1\n        }\n}\n"
	after, how, err := edit.Replace(before, "if ok {\n    return 1\n}", "return 2")
	if err != nil {
		t.Fatalf("a block written at a different indentation was not replaced: %v", err)
	}
	if !strings.Contains(after, "return 2") || strings.Contains(after, "if ok") {
		t.Errorf("the text now reads %q, want the block replaced", after)
	}
	if how != edit.MatchIndentationFlexible {
		t.Errorf("the span was found by the %s matcher, want the indentation-flexible one", how)
	}
}

func TestASpanWithWhitespaceAtItsEndsIsFoundAsAUniqueSubstring(t *testing.T) {
	after, how, err := edit.Replace("return alpha + beta;\n", "  alpha + beta  ", "gamma")
	if err != nil {
		t.Fatalf("a span with whitespace at its ends was not replaced: %v", err)
	}
	if after != "return gamma;\n" {
		t.Errorf("the text now reads %q, want the middle of the line replaced", after)
	}
	if how != edit.MatchUniqueSubstring {
		t.Errorf("the span was found by the %s matcher, want the unique-substring one", how)
	}
}

func TestASpanThatAppearsTwiceIsRefused(t *testing.T) {
	_, _, err := edit.Replace("beta\nbeta\n", "beta", "delta")
	if err == nil {
		t.Fatalf("a span that appears twice was replaced, and the tool cannot know which one was meant")
	}
	if !strings.Contains(err.Error(), "more") {
		t.Errorf("the refusal reads %q and does not say the span was found more than once", err)
	}
}

func TestASpanThatIsNotThereIsRefused(t *testing.T) {
	_, _, err := edit.Replace("alpha\n", "omega", "delta")
	if err == nil {
		t.Fatalf("a span that is not in the text was replaced")
	}
	if !strings.Contains(err.Error(), "omega") {
		t.Errorf("the refusal reads %q and does not name the text that was looked for", err)
	}
}

func TestReplacingTextWithItselfIsRefused(t *testing.T) {
	if _, _, err := edit.Replace("alpha\n", "alpha", "alpha"); err == nil {
		t.Errorf("an edit that changes nothing was treated as a change")
	}
}

func TestAnEmptyOldTextIsRefused(t *testing.T) {
	if _, _, err := edit.Replace("alpha\n", "", "beta"); err == nil {
		t.Errorf("an edit with nothing to look for was treated as a change")
	}
}

func TestAMatchMuchBiggerThanWhatWasAskedForIsRefused(t *testing.T) {
	before := "let" + strings.Repeat(" ", 400) + "x = 1\n"
	_, _, err := edit.Replace(before, "let x = 1", "let x = 2")
	if err == nil {
		t.Errorf("a span far bigger than the text asked for was replaced, and that is not the edit the model meant")
	}
}
