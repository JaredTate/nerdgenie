package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/orientation"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheCapOpensAFreshWindowWithTheNewestResultsAndTheAsk is the second half
// of the cut-do-not-wipe idea. When the conversation reaches MaxMessagesKept
// the loop used to drop the oldest half of it: the model then read a hundred
// messages that began in the middle of a round, with no ask and no bearings,
// and run ten's polish task paid two re-reads of a hundred and fifteen
// thousand tokens for it. Now the window is opened afresh the way a pick-up
// opens it: the orientation block with the newest six results in full, one
// line saying what happened, and the ask last, because the last thing the
// model reads is what it answers. Nothing before the cap is lost: the record
// holds every step and every result by its id, and the log holds the rest.
func TestTheCapOpensAFreshWindowWithTheNewestResultsAndTheAsk(t *testing.T) {
	rounds := loop.MaxMessagesKept / 2
	steps := []testkit.Step{}
	answers := []string{}
	for round := 1; round <= rounds; round++ {
		steps = append(steps, callStep(fmt.Sprintf("I will read file %d.", round),
			callFor(fmt.Sprintf("c%d", round), "read", fmt.Sprintf(`{"path":"file%d.md"}`, round))))
		answers = append(answers, fmt.Sprintf("the words of file %d", round))
	}
	steps = append(steps, answerStep("Every file is read."))
	built := newHarness(t, steps, scriptedTool("read", answers...))

	built.ask(t, "read every file")

	requests := built.model.Requests()
	if len(requests) < rounds+1 {
		t.Fatalf("the model was called %d times, want at least %d", len(requests), rounds+1)
	}
	atTheCap := wholeRequestText(requests[rounds-1])
	if !strings.Contains(atTheCap, "I will read file 1.") {
		t.Errorf("the request at the cap has lost the first round, and nothing leaves until the cap is passed:\n%s", atTheCap)
	}
	after := wholeRequestText(requests[rounds])
	if strings.Contains(after, "I will read file 1.") {
		t.Errorf("the request after the cap still carries the first round, want a fresh window:\n%s", after)
	}
	newest := fmt.Sprintf("the words of file %d", rounds)
	if !strings.Contains(after, orientation.TheResultsHeading) || !strings.Contains(after, newest) {
		t.Errorf("the fresh window does not carry the newest result in full:\n%s", after)
	}
	if !strings.Contains(after, loop.TheFreshWindowLine) {
		t.Errorf("the fresh window does not say what happened:\n%s", after)
	}
	ask := strings.LastIndex(after, "read every file")
	if ask < strings.Index(after, orientation.TheHeading) || ask < strings.Index(after, loop.TheFreshWindowLine) {
		t.Errorf("the ask does not ride last in the fresh window, and the last thing the model reads is what it answers:\n%s", after)
	}
	if len(after) > len(atTheCap)/2 {
		t.Errorf("the request after the cap is %d letters against %d at the cap, want it under half", len(after), len(atTheCap))
	}
	if len(requests[rounds].Messages) > len(requests[rounds-1].Messages)/2 {
		t.Errorf("the request after the cap carries %d messages against %d at the cap, want the window opened afresh",
			len(requests[rounds].Messages), len(requests[rounds-1].Messages))
	}
	if !aFreshWindowIsLogged(t, built) {
		t.Error("the log holds no event saying the window was opened afresh, and a run's numbers count what the log holds")
	}
}

// aFreshWindowIsLogged says whether the log holds the harness's own event
// about a fresh window.
func aFreshWindowIsLogged(t *testing.T, built *harness) bool {
	t.Helper()
	events, err := built.store.ByKind(t.Context(), contract.EventRecordChange)
	if err != nil {
		t.Fatalf("cannot read the record changes out of the log: %v", err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Body), loop.FreshWindowMarker) {
			return true
		}
	}
	return false
}

// TestNothingIsLoggedAsAFreshWindowUnderTheCap keeps the event honest: a task
// that never reaches the cap writes no fresh-window event.
func TestNothingIsLoggedAsAFreshWindowUnderTheCap(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The notes are read."),
	}, scriptedTool("read", "the notes"))

	built.ask(t, "read the notes")

	if aFreshWindowIsLogged(t, built) {
		t.Error("the log says the window was opened afresh, and the conversation was two rounds long")
	}
}
