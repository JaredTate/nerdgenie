package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// Run 23's visual QA task spent 157 calls and 45 minutes polishing one SVG
// ring: every round read something new or changed the page, so the stall
// meter saw progress in all of them, and nothing ever said "enough". At a
// round cap the harness says so, once, and asks the model to finish.

// readsOfNewFiles is a task of the given number of reads, each of a file not
// read before, so that every round counts as progress to the meter, then the
// answer, then the review a long task earns.
func readsOfNewFiles(t *testing.T, calls int) *harness {
	t.Helper()
	steps := []testkit.Step{}
	answers := []string{}
	for at := 1; at <= calls; at++ {
		steps = append(steps, callStep("Reading on.", callFor(fmt.Sprintf("c%d", at), contract.ToolRead, fmt.Sprintf(`{"path":"notes%d.md"}`, at))))
		answers = append(answers, fmt.Sprintf("the notes %d", at))
	}
	steps = append(steps,
		answerStep("Done. What changed: nothing. What I checked: the notes. What is left: nothing."),
		aReviewReply("Read fewer notes."))
	return newHarness(t, steps, scriptedTool(contract.ToolRead, answers...))
}

func TestAtTheRoundCapTheModelIsAskedOnceToFinish(t *testing.T) {
	built := readsOfNewFiles(t, loop.CallsBeforeTheFinishNudge+3)

	outcome := built.ask(t, "read all the notes")

	first, count := requestsCarrying(built, loop.TheFinishNudgeLine)
	if count == 0 {
		t.Fatalf("the finish nudge was never said in %d calls; the task ended %q after %d requests with the report:\n%s", loop.CallsBeforeTheFinishNudge+3, outcome.Status, len(built.model.Requests()), outcome.Report)
	}
	// Request number N is the one made after the N-th call, so the line
	// first rides in the request after the hundredth call, or one round
	// later when the window was full at that moment and a fresh one opened,
	// which a hundred rounds of two messages each is exactly.
	if first < loop.CallsBeforeTheFinishNudge || first > loop.CallsBeforeTheFinishNudge+1 {
		t.Errorf("the finish nudge first rides in request %d, want request %d or %d, right after the %dth call", first, loop.CallsBeforeTheFinishNudge, loop.CallsBeforeTheFinishNudge+1, loop.CallsBeforeTheFinishNudge)
	}
	requests := built.model.Requests()
	if strings.Count(wholeRequestText(requests[len(requests)-2]), loop.TheFinishNudgeLine) > 1 {
		t.Errorf("the finish nudge was said more than once; the last request reads:\n%s", wholeRequestText(requests[len(requests)-2]))
	}
	if !strings.Contains(loop.TheFinishNudgeLine, "finish") || !strings.Contains(loop.TheFinishNudgeLine, "failure") {
		t.Errorf("the finish nudge does not ask to finish or to write what blocks as a failure: %q", loop.TheFinishNudgeLine)
	}
}

func TestATaskUnderTheRoundCapIsNeverAskedToFinish(t *testing.T) {
	built := readsOfNewFiles(t, loop.CallsBeforeTheFinishNudge-1)

	built.ask(t, "read all the notes")

	if _, count := requestsCarrying(built, loop.TheFinishNudgeLine); count != 0 {
		t.Errorf("the finish nudge was said %d times in a task of %d calls, want never", count, loop.CallsBeforeTheFinishNudge-1)
	}
}
