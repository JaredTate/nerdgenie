package loop

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// capsWithAWindowOf is the caps a test needs to see the detector's window fill
// up and roll over without making twenty calls to do it.
func capsWithAWindowOf(window int) contract.Caps {
	caps := contract.DefaultConfig().Caps
	caps.IdenticalCallWindow = window
	return caps
}

// TestEveryBoundOfThisPackageIsTheNumberItSays is the gate review's eleventh
// finding: seven of the twelve bounds here could be changed to any other number
// and the package's tests stayed green, so nothing held them. Every bound is
// named here with the literal it is meant to be, so that changing one is a
// decision somebody makes on purpose and not a number that slipped.
func TestEveryBoundOfThisPackageIsTheNumberItSays(t *testing.T) {
	for _, bound := range []struct {
		name  string
		held  int
		wants int
	}{
		{"extraRounds", extraRounds, 2},
		{"MaxMessagesKept", MaxMessagesKept, 200},
		{"MaxDoneCheckNudges", MaxDoneCheckNudges, 3},
		{"MaxFailingTestsNamed", MaxFailingTestsNamed, 5},
		{"MaxChangedFilesNamed", MaxChangedFilesNamed, 3},
		{"MaxProbesBetweenEdits", MaxProbesBetweenEdits, 5},
		{"MaxJobsMadeNoted", MaxJobsMadeNoted, 8},
		{"MaxJobTasksRemembered", MaxJobTasksRemembered, 100},
		{"IdenticalCallsAllowed", IdenticalCallsAllowed, 2},
		{"SameCallHardCap", SameCallHardCap, 6},
		{"MaxRethinkResultBytes", MaxRethinkResultBytes, 16384},
		{"ClosedForRounds", ClosedForRounds, 10},
		{"MaxResultsPinned", MaxResultsPinned, 4},
		{"MaxLinesProvedByTheReply", MaxLinesProvedByTheReply, 8},
		{"MaxResultsNamedInARefusal", MaxResultsNamedInARefusal, 12},
		{"MaxFilesInTheSituation", MaxFilesInTheSituation, 8},
		{"MaxSituationLineLetters", MaxSituationLineLetters, 160},
		{"MaxCommandsCheckedPerLine", MaxCommandsCheckedPerLine, 3},
		{"MaxPathsCheckedPerLine", MaxPathsCheckedPerLine, 3},
		{"MaxTasksListed", MaxTasksListed, 50},
		{"MaxAskLettersInAListing", MaxAskLettersInAListing, 60},
		{"RoundsThatDeserveAReview", RoundsThatDeserveAReview, 5},
		{"MaxRecordLineRunes", MaxRecordLineRunes, 90},
		{"MaxToolLineRunes", MaxToolLineRunes, 90},
		{"WrapUpTime in seconds", int(WrapUpTime / time.Second), 10},
	} {
		if bound.held != bound.wants {
			t.Errorf("%s is %d and the number this package was built and tested against is %d, "+
				"so change the test with the bound or change the bound back",
				bound.name, bound.held, bound.wants)
		}
	}
}

// TestTheLabelOfTheAnswerIsTheWordTheModelIsTold pins the one word a done line
// names when the answer to the user is its own proof. The task tool's
// description and the harness's own refusals both say it, so it is a name and
// not a number, and it changes for nobody.
func TestTheLabelOfTheAnswerIsTheWordTheModelIsTold(t *testing.T) {
	if TheReplyLabel != "reply" {
		t.Errorf("a done line proved by the answer names %q, and the word this package was built around is \"reply\"",
			TheReplyLabel)
	}
}

// TestTheListingCutsALongAskAndSaysSo pins what the listing bound does, because
// a number is only half a bound: the other half is what happens at it.
func TestTheListingCutsALongAskAndSaysSo(t *testing.T) {
	long := ""
	for range 20 {
		long += "words "
	}
	cut := oneLineOf(long)
	if len([]rune(cut)) != MaxAskLettersInAListing {
		t.Errorf("a long ask came out %d letters long, and the listing cuts one to %d",
			len([]rune(cut)), MaxAskLettersInAListing)
	}
	if cut[len(cut)-3:] != "..." {
		t.Errorf("the cut ask reads %q, and a line that was cut says so", cut)
	}
	short := oneLineOf("read the notes")
	if short != "read the notes" {
		t.Errorf("a short ask came out %q, and one that fits is left alone", short)
	}
}

// TestTheDetectorWindowHoldsOnlyItsLastFewCalls pins the other half of the
// identical-call bound: the window is a window, and the calls before it are
// forgotten.
func TestTheDetectorWindowHoldsOnlyItsLastFewCalls(t *testing.T) {
	held := &run{theLoop: &Loop{options: Options{Caps: capsWithAWindowOf(3)}}}
	for range 10 {
		held.rememberCall("read notes.md")
	}
	if len(held.recentCalls) != 3 {
		t.Errorf("the window holds %d calls after ten were made, and the cap it was given is 3",
			len(held.recentCalls))
	}
	held.forgetTheCalls()
	if len(held.recentCalls) != 0 {
		t.Errorf("the window holds %d calls after it was emptied", len(held.recentCalls))
	}
}
