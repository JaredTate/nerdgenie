package record

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestLessonsAreCutToALineAndTheOldestLeaveWhenTheListIsFull is the fifth game
// build's play-test task, which died on the harness's own bookkeeping a second
// time: seventeen failures and three decisions written as paragraphs, 1,765
// tokens of failures alone, took the record past its size on a harness write
// of one read's result line, and the task failed. A lesson is one line: its
// text and its cause are cut to MaxLessonRunes. And a list of lessons holds
// the newest MaxFailuresKept or MaxDecisionsKept; the oldest leave the record
// and stay in the log, and the labels keep counting upward so nothing a
// reader saw is renumbered.
func TestLessonsAreCutToALineAndTheOldestLeaveWhenTheListIsFull(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	written := MaxFailuresKept + 2
	for number := 1; number <= written; number++ {
		long := strings.Repeat(fmt.Sprintf("lesson %d, %s; ", number, theFillerFailures[number-1]), 6)
		update := Update{
			Failure:  &NewFailure{Text: long, Cause: "because " + long},
			Decision: &NewDecision{Text: "chose " + long, Reason: long},
		}
		if err := keeper.Apply(ctx, update); err != nil {
			t.Fatalf("lesson %d was refused: %v", number, err)
		}
	}
	held := keeper.Record().Lessons
	if len(held.Failures) != MaxFailuresKept || held.Failures[0].ID != "F3" || held.Failures[MaxFailuresKept-1].ID != fmt.Sprintf("F%d", written) {
		t.Errorf("the failures read %d from %s to %s, want the newest %d from F3 to F%d", len(held.Failures), held.Failures[0].ID, held.Failures[len(held.Failures)-1].ID, MaxFailuresKept, written)
	}
	if len(held.Decisions) != MaxDecisionsKept || held.Decisions[0].ID != "D3" || held.Decisions[MaxDecisionsKept-1].ID != fmt.Sprintf("D%d", written) {
		t.Errorf("the decisions read %d from %s to %s, want the newest %d from D3 to D%d", len(held.Decisions), held.Decisions[0].ID, held.Decisions[len(held.Decisions)-1].ID, MaxDecisionsKept, written)
	}
	for _, failure := range held.Failures {
		for _, line := range []string{failure.Text, failure.Cause} {
			if utf8.RuneCountInString(line) > MaxLessonRunes || !strings.HasSuffix(line, "...") {
				t.Errorf("%s holds a line of %d runes that does not say it was cut: %q", failure.ID, utf8.RuneCountInString(line), line)
			}
		}
	}
	for _, decision := range held.Decisions {
		for _, line := range []string{decision.Text, decision.Reason} {
			if utf8.RuneCountInString(line) > MaxLessonRunes || !strings.HasSuffix(line, "...") {
				t.Errorf("%s holds a line of %d runes that does not say it was cut: %q", decision.ID, utf8.RuneCountInString(line), line)
			}
		}
	}
	parsed, err := Parse(Print(keeper.Record()))
	if err != nil {
		t.Fatalf("the record with its oldest lessons gone cannot be read back: %v", err)
	}
	if parsed.Lessons.Failures[0].ID != "F3" || parsed.Lessons.Decisions[0].ID != "D3" {
		t.Errorf("the record read back starts its lessons at %s and %s, want F3 and D3", parsed.Lessons.Failures[0].ID, parsed.Lessons.Decisions[0].ID)
	}
}

// TestALongFailureWrittenTwiceIsRefusedLikeAShortOne is the fifth game
// build's play-test task an hour after the duplicate rule went in: it wrote a
// four-hundred-character failure twice and the record took both, because the
// second was compared whole against the first as the record had cut it, and
// a whole text shares few of its words with its own first line. The words
// compared are the words the record would keep.
func TestALongFailureWrittenTwiceIsRefusedLikeAShortOne(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	long := "Done_when items 1-4 were pinned to results that do not prove them: r184 is a plain page read with no state dump, r185 is a single idle-state probe, r186 is a grep of the dev-control bindings, r187 is a file read, and none of them actually exercises movement, rotation, drops, clears, scoring, pause, restart, game over or the high score, so the done list stands on nothing and the play-test has not begun."
	if err := keeper.Apply(ctx, Update{Failure: &NewFailure{Text: long, Cause: "the lines were pinned before the play-test ran"}}); err != nil {
		t.Fatalf("the first failure was refused: %v", err)
	}
	if err := keeper.Apply(ctx, Update{Failure: &NewFailure{Text: long, Cause: "the lines were pinned before the play-test ran"}}); !errors.Is(err, ErrFailureAlreadyWritten) {
		t.Errorf("the same long failure written again gave %v, want a refusal naming F1", err)
	}
}

// TestAShortLessonIsKeptWholeAndTheCutFallsOnAWord holds the cut to what is
// needed: a lesson under the cap is kept as written, and a cut lands after a
// whole word.
func TestAShortLessonIsKeptWholeAndTheCutFallsOnAWord(t *testing.T) {
	if cut := cutToALesson("the page hangs on Start"); cut != "the page hangs on Start" {
		t.Errorf("a short lesson was changed to %q", cut)
	}
	long := strings.Repeat("word ", 60)
	cut := cutToALesson(long)
	if utf8.RuneCountInString(cut) > MaxLessonRunes || !strings.HasSuffix(cut, "word...") {
		t.Errorf("the cut reads %q, want whole words and a mark that it was cut", cut)
	}
}
