package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The first human trial ran the same shell command thirteen times in a row,
// "google-chrome --new-window ... & sleep 3; echo launched", and the detector
// never fired, because every answer carried a new process id, and to the first
// rule a call whose answer changed is a different call. The tests here hold the
// second rule: the same call with the same arguments, whatever it answered, is
// refused past SameCallHardCap and ends the turn past the cap plus two.

// theChromeLaunch is the call the trial made over and over.
const theChromeLaunch = `{"command":"google-chrome --new-window https://example.com & sleep 3; echo launched"}`

// launchSteps are the same shell call asked for the given number of times, one
// round each, followed by the answer the model gives when it is let.
func launchSteps(count int) []testkit.Step {
	steps := []testkit.Step{}
	for number := 1; number <= count; number++ {
		steps = append(steps, callStep("I will open the browser.",
			callFor(fmt.Sprintf("c%d", number), contract.ToolShell, theChromeLaunch)))
	}
	return append(steps, answerStep("The browser is open. Shall I go on?"))
}

// launchesWithNewProcessIDs answers every call the way the shell did in the
// trial: the same words with a process id nobody has seen before.
func launchesWithNewProcessIDs(count int) *pollingTool {
	answers := []string{}
	for number := 1; number <= count; number++ {
		answers = append(answers, fmt.Sprintf("[1] %d\nlaunched", 4100+number))
	}
	return &pollingTool{answers: answers}
}

// TestSixSameCallsWithChangingAnswersAllRun is the near side of the boundary:
// six is the cap, so the sixth call still runs and nothing is said about it.
func TestSixSameCallsWithChangingAnswersAllRun(t *testing.T) {
	launching := launchesWithNewProcessIDs(loop.SameCallHardCap)
	built := newHarness(t, launchSteps(loop.SameCallHardCap), launching)

	outcome := built.ask(t, "open the browser")

	if launching.calls != loop.SameCallHardCap {
		t.Errorf("the shell tool ran %d times, want %d, because six is the cap and the sixth still runs",
			launching.calls, loop.SameCallHardCap)
	}
	if strings.Contains(requestsJoined(built.model.Requests()), "was not run again") {
		t.Error("a call was refused before the cap was reached")
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q, and six of the same call is inside the cap", outcome.StopLine)
	}
}

// TestTheSeventhSameCallIsRefusedWhateverItAnswered is the far side of the
// boundary: the seventh call is refused although every answer so far was new,
// and the refusal says how many times the call was made and what to do instead.
func TestTheSeventhSameCallIsRefusedWhateverItAnswered(t *testing.T) {
	launching := launchesWithNewProcessIDs(loop.SameCallHardCap + 1)
	built := newHarness(t, launchSteps(loop.SameCallHardCap+1), launching)

	outcome := built.ask(t, "open the browser")

	if launching.calls != loop.SameCallHardCap {
		t.Errorf("the shell tool ran %d times, want %d, because the seventh of the same call is refused whatever the answers were",
			launching.calls, loop.SameCallHardCap)
	}
	shown := requestsJoined(built.model.Requests())
	for _, words := range []string{
		"already called shell with these exact arguments 6 times",
		"did not change what you did next",
		"wait longer",
		"read the result you already have",
		"answer the user",
	} {
		if !strings.Contains(shown, words) {
			t.Errorf("the refusal never told the model %q", words)
		}
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q, and one refusal is a refusal, not the end of the turn", outcome.StopLine)
	}
}

// TestPastTheHardCapPlusTwoTheTurnEnds proves the second rule ends the turn the
// way the first one does: the seventh and eighth calls are refused, and the
// ninth ends the turn with the same stopped report the first rule sends.
func TestPastTheHardCapPlusTwoTheTurnEnds(t *testing.T) {
	launching := launchesWithNewProcessIDs(loop.SameCallHardCap + 3)
	built := newHarness(t, launchSteps(loop.SameCallHardCap+3), launching)

	outcome := built.ask(t, "open the browser")

	if launching.calls != loop.SameCallHardCap {
		t.Errorf("the shell tool ran %d times, want %d, because nothing past the cap runs", launching.calls, loop.SameCallHardCap)
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the model asked for the same thing nine times", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "the same thing over and over") {
		t.Errorf("the user was sent %v, and a task the detector ended says so in its report", built.channel.Sent())
	}
}

// TestTheFirstRuleStillEndsARunWhoseAnswersStopChanging proves the first rule
// is still there under the second: a poll that keeps saying "done" is refused
// on its third identical answer and ends the turn on its fourth, long before the
// hard cap is reached.
func TestTheFirstRuleStillEndsARunWhoseAnswersStopChanging(t *testing.T) {
	polling := &pollingTool{answers: []string{
		"p1 is still running, 12 seconds in",
		"p1 is still running, 24 seconds in",
		"p1 exited 0: the build finished",
		"p1 exited 0: the build finished",
	}}
	same := func(id string) testkit.Step {
		return callStep("I will wait for it.", callFor(id, contract.ToolShell, `{"action":"poll","id":"p1"}`))
	}
	built := newHarness(t, []testkit.Step{
		same("c1"), same("c2"), same("c3"), same("c4"), same("c5"), same("c6"),
		answerStep("This answer is never given, because the turn ends before it."),
	}, polling)

	outcome := built.ask(t, "wait for the build")

	if polling.calls != 4 {
		t.Errorf("the shell tool ran %d times, want 4: two polls whose answers changed, and two more with the same answer",
			polling.calls)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "Do something different") {
		t.Error("the model was never told to do something different, and the first rule still says so")
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the first rule ends a run of the same answer on its fourth call",
			outcome.Status)
	}
}
