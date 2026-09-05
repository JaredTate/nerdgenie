package context

import (
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestAProvenDoneLineChangesNothingAboveTheCacheLine is the cost the live game
// build paid three times over on its earlier run: the done list rode in the
// system prompt with the goal and the rules, so writing it, and then pointing
// each line at its proof, rewrote the front of the prompt and cost the whole
// conversation under it, a hundred and thirty-five seconds on the local model
// at sixty thousand tokens. The ask, the why and the rules are written once
// and hold still; the done list is written and marked as the work goes, so it
// rides in the tail with the plan, where a change costs only the tail.
func TestAProvenDoneLineChangesNothingAboveTheCacheLine(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 20)
	builder := newGoldenBuilder(t)
	before := run.input(200000)
	if len(before.Record.Goal.DoneWhen) == 0 {
		t.Fatal("the fixture's record has no done list at round twenty, and the test needs one to mark")
	}
	after := before
	after.Record.Goal.DoneWhen = slices.Clone(before.Record.Goal.DoneWhen)
	last := before.Record.Work.Results[len(before.Record.Work.Results)-1]
	for at := range after.Record.Goal.DoneWhen {
		after.Record.Goal.DoneWhen[at].Done, after.Record.Goal.DoneWhen[at].ResultID = true, last.ID
	}

	earlier, err := builder.Build(t.Context(), before)
	if err != nil {
		t.Fatalf("cannot build the prompt before the proof: %v", err)
	}
	later, err := builder.Build(t.Context(), after)
	if err != nil {
		t.Fatalf("cannot build the prompt after the proof: %v", err)
	}

	if aboveTheCacheLine(earlier) != aboveTheCacheLine(later) {
		t.Error("marking every done line done changed the bytes above the cache line, and the done list has to ride in the tail")
	}
	if !strings.Contains(aboveTheCacheLine(later), "Ask: ") || strings.Contains(aboveTheCacheLine(later), "Done when:") {
		t.Error("the system prompt must still carry the ask and must no longer carry the done list")
	}
	body := theMessageHeaded(later, recordSecondHalfHeading)
	if !strings.Contains(body, "Done when:") || !strings.Contains(body, "-> "+last.ID) {
		t.Errorf("the record's body in the tail reads:\n%s\nwant the done list with its proof at the top of it", body)
	}
}

// theMessageHeaded is the text of the one message in a prompt that begins with
// the heading given, or empty when there is none.
func theMessageHeaded(request contract.Request, heading string) string {
	for _, message := range request.Messages {
		if strings.HasPrefix(message.Text, heading) {
			return message.Text
		}
	}
	return ""
}
