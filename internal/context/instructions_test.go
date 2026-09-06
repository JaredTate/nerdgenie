package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The design file the instruction text is copied from, and the heading of the
// section that holds it.
const (
	designPath    = "docs/NERDGENIE_PLAN.md"
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
// cost paid on every turn. The heading over the skill list is the harness's own
// words too, sent whenever a home has a skill, so it is counted against the same
// budget: a line added there is a line taken out of the instructions.
func TestTheInstructionTextIsSmallEnoughToRideInEveryPrompt(t *testing.T) {
	counted := len(strings.Fields(InstructionText)) + len(strings.Fields(skillsHeading))
	t.Logf("the instruction text and the skill heading are %d words and %d bytes together",
		counted, len(InstructionText)+len(skillsHeading))
	if counted > MaxInstructionWords {
		t.Errorf("the instruction text and the skill heading are %d words and the cap is %d, so shorten the text in the design and copy it here again",
			counted, MaxInstructionWords)
	}
}

// TestTheInstructionTextSaysSeveralCallsMayRideInOneReply is the loop half of
// brief 6.6. The repair package has always read a list of calls out of one
// reply, and the loop has always run them in order, and nothing in the text
// ever told the model it could write one, so a model that asks for one tool a
// round pays a whole call for every step that could have ridden with the one
// before it. The text has to say so in one short sentence, and say when: only
// when the calls do not depend on each other.
func TestTheInstructionTextSaysSeveralCallsMayRideInOneReply(t *testing.T) {
	for _, said := range []string{"several tools in one reply", "they run in order", "every write or edit reruns them"} {
		if !strings.Contains(InstructionText, said) {
			t.Errorf("the instruction text does not say %q, so the model is never told it may ask for several tools at once",
				said)
		}
	}
}

// TestTheInstructionTextSaysAJobIsPlannedThenWorked proves the instruction text
// tells the model that work of many features, or work that must wait for a date,
// is a job written with its task list before its first task is worked, and never
// done as a plain task. Without this a model given a whole game builds it inside
// one task, whose done list cannot prove it and whose record it overruns.
func TestTheInstructionTextSaysAJobIsPlannedThenWorked(t *testing.T) {
	for _, said := range []string{"Work of many features", "must wait for a date", "task list first", "never do a job's work in a plain task"} {
		if !strings.Contains(InstructionText, said) {
			t.Errorf("the instruction text does not say %q, so the model is never told to plan a job before working it", said)
		}
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

// TestTheInstructionTextSaysToMarkEachPlanStepDone pins the sentence that
// makes the side panel's check marks move: a live build finished its engine,
// its hazards and its UI with the plan standing at "0 of 9 done", because
// nothing told the model to mark a step and nothing else ever did.
func TestTheInstructionTextSaysToMarkEachPlanStepDone(t *testing.T) {
	if !strings.Contains(InstructionText, "Mark each plan step done, with its result, when it is.") {
		t.Error("the instruction text does not tell the model to mark each plan step done as it goes")
	}
}
