package context

import (
	"strings"
	"testing"
)

// TestTheInstructionTextSaysATooLongDoneListPlanOrAskIsAJob pins the one
// sentence that tells the model the three caps before it tries them: a done
// list over five lines, a plan over ten steps, or an ask over 250 words, is
// refused because that ask is a job. A model that has already written an
// eight-line done list, or hidden a whole game build in a twelve-step plan, or
// squeezed a several-hundred-word ask into one task with short lists, is told
// by the record's refusal; a model that reads this sentence first writes the
// job instead.
func TestTheInstructionTextSaysATooLongDoneListPlanOrAskIsAJob(t *testing.T) {
	const theRule = "A done list over five lines, a plan over ten steps, or an ask over 250 words, is refused: that ask is a job."
	if !strings.Contains(InstructionText, theRule) {
		t.Errorf("the instruction text does not say %q, so the model is never told the five-line, ten-step and 250-word rules until it has broken one", theRule)
	}
}
