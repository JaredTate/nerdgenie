package context

import (
	"strings"
	"testing"
)

// TestTheInstructionTextSaysATooLongDoneListOrPlanIsAJob pins the one sentence
// that tells the model the two caps before it tries them: a done list over five
// lines, or a plan over eight steps, is refused because that ask is a job. A
// model that has already written an eight-line done list, or hidden a whole
// game build in a twelve-step plan, is told by the record's refusal; a model
// that reads this sentence first writes the job instead.
func TestTheInstructionTextSaysATooLongDoneListOrPlanIsAJob(t *testing.T) {
	const theRule = "A done list over five lines, or a plan over eight steps, is refused: that ask is a job."
	if !strings.Contains(InstructionText, theRule) {
		t.Errorf("the instruction text does not say %q, so the model is never told the five-line and eight-step rules until it has broken one", theRule)
	}
}
