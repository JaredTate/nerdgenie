package context

import (
	"strings"
	"testing"
)

// TestTheInstructionTextSaysADoneListOverFiveLinesIsAJob pins the one sentence
// that tells the model the five-line rule before it tries. A model that has
// already written an eight-line done list is told by the record's refusal; a
// model that reads this sentence first writes the job instead.
func TestTheInstructionTextSaysADoneListOverFiveLinesIsAJob(t *testing.T) {
	const theRule = "A done list over five lines is refused: that ask is a job."
	if !strings.Contains(InstructionText, theRule) {
		t.Errorf("the instruction text does not say %q, so the model is never told the five-line rule until it has broken it", theRule)
	}
}
