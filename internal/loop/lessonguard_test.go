package loop_test

import (
	"testing"
)

// TestAReviewAnswerThatIsToolMarkupIsNotKept guards memory against what a
// small model writes when the four questions confuse it. The live home's
// MEMORY.md held two facts that read "<parameter=file_path>" and
// "</function>": the model had answered the review with the opening of a tool
// call, and the harness kept the fourth line as a lesson. Markup is no lesson,
// so it is not saved, and the task is none the worse for it.
func TestAReviewAnswerThatIsToolMarkupIsNotKept(t *testing.T) {
	for _, markup := range []string{"<parameter=file_path>", "</function>", "<tool_call>{\"name\":\"read\"}", "<function=read>"} {
		steps := append(closingScript("the notes are read"), aReviewReply(markup))
		built, _ := midTurnHarness(t, steps, "no, check the brand file first")

		built.ask(t, "read the notes")

		if facts := factsIn(t, built); len(facts) != 0 {
			t.Errorf("memory holds %v after a review answered with %q, want nothing saved", facts, markup)
		}
	}
}
