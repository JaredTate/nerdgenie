package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

// The design file the instruction text is copied from, and the heading of the
// section that holds it.
const (
	designPath    = "docs/COEUS_PLAN.md"
	designSection = "## 5. What the model is told"
)

// TestTheInstructionTextIsTheDesignsOwnWords proves the constant is the design's
// section 5 word for word. The design is the intent and this package only copies
// it, so when the design's text changes the constant has to change with it, and
// this test is what says so.
func TestTheInstructionTextIsTheDesignsOwnWords(t *testing.T) {
	quoted := instructionTextInTheDesign(t)
	if InstructionText != quoted {
		t.Errorf("the instruction text is not the design's own.\n--- the design says ---\n%s\n--- the constant says ---\n%s",
			quoted, InstructionText)
	}
}

// TestTheInstructionTextIsSmallEnoughToRideInEveryPrompt counts the words,
// because this text is read on every call to every model and its length is a
// cost paid on every turn.
func TestTheInstructionTextIsSmallEnoughToRideInEveryPrompt(t *testing.T) {
	counted := len(strings.Fields(InstructionText))
	t.Logf("the instruction text is %d words and %d bytes", counted, len(InstructionText))
	if counted > MaxInstructionWords {
		t.Errorf("the instruction text is %d words and the cap is %d, so shorten it in the design and copy it here again",
			counted, MaxInstructionWords)
	}
}

// instructionTextInTheDesign reads the block quote under section 5 of the design
// and takes the quote marks off, which leaves exactly the text the model is sent.
func instructionTextInTheDesign(t *testing.T) string {
	t.Helper()
	root, err := testkit.RepositoryRoot()
	if err != nil {
		t.Fatalf("cannot find the repository root: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(designPath)))
	if err != nil {
		t.Fatalf("cannot read the design at %s: %v", designPath, err)
	}

	quoted := []string{}
	inSection := false
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "## ") {
			inSection = line == designSection
		}
		if !inSection {
			continue
		}
		switch {
		case line == ">":
			quoted = append(quoted, "")
		case strings.HasPrefix(line, "> "):
			quoted = append(quoted, strings.TrimPrefix(line, "> "))
		}
	}
	if len(quoted) == 0 {
		t.Fatalf("the design section %q holds no block quote, so the instruction text has moved", designSection)
	}
	return strings.Join(quoted, "\n")
}
