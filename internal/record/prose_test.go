package record

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The record's own marks must never be mistaken for the prose the model writes.
// The real job store's AddTask failed on ordinary task text, "Write the failing
// tests for the core engine, then make them pass", because a job task's due
// date was written after the last comma of the line and read back from it, so
// every comma in a task's text was taken for a date and the read-back check
// refused the record. A retrying model then left twenty-nine empty jobs
// behind. The fake store has no read-back check of its own, which is why the
// tests here go through the keeper, where the check runs on every change.

// theTaskTextWithCommas is the text the real store refused, and what every
// test here has to get back word for word.
const theTaskTextWithCommas = "Write the failing tests for the core engine, then make them pass, one at a time"

// checkItReadsBackWordForWord prints the record the keeper holds, reads the
// text back, and holds the two to be the same record, which is the promise the
// keeper makes on every change.
func checkItReadsBackWordForWord(t *testing.T, keeper *Keeper) {
	t.Helper()
	read, err := Parse(Print(keeper.Record()))
	if err != nil {
		t.Fatalf("the record cannot be read back at all: %v\n%s", err, keeper.Text())
	}
	if !reflect.DeepEqual(read, keeper.Record()) {
		t.Errorf("the record reads back as something else:\n%s\nread back as\n%+v\nand the record holds\n%+v",
			keeper.Text(), read.Work, keeper.Record().Work)
	}
}

// TestAJobTaskWithACommaInItsTextReadsBackWordForWord is the defect itself: a
// task with a comma in its text and no due date is accepted and comes back as
// it was written.
func TestAJobTaskWithACommaInItsTextReadsBackWordForWord(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())

	err := keeper.Apply(t.Context(), Update{Tasks: []NewJobTask{{TaskID: contract.TaskID(1), Text: theTaskTextWithCommas}}})
	if err != nil {
		t.Fatalf("a job task with a comma in its text was refused: %v", err)
	}
	held := keeper.Record().Work.Tasks
	if len(held) != 1 || held[0].Text != theTaskTextWithCommas || held[0].DueAt != "" {
		t.Errorf("the job holds %+v, want one task reading %q with no due date", held, theTaskTextWithCommas)
	}
	checkItReadsBackWordForWord(t, keeper)
}

// TestAJobTaskWithACommaAndADueDateReadsBackWordForWord holds the other half:
// a due date still has a place on the line, and both it and the text may carry
// a comma.
func TestAJobTaskWithACommaAndADueDateReadsBackWordForWord(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())
	due := "tomorrow, at 14:00"

	err := keeper.Apply(t.Context(), Update{Tasks: []NewJobTask{{TaskID: contract.TaskID(1), Text: theTaskTextWithCommas, DueAt: due}}})
	if err != nil {
		t.Fatalf("a job task with a comma in its text and a due date was refused: %v", err)
	}
	held := keeper.Record().Work.Tasks
	if len(held) != 1 || held[0].Text != theTaskTextWithCommas || held[0].DueAt != due {
		t.Errorf("the job holds %+v, want one task reading %q due %q", held, theTaskTextWithCommas, due)
	}
	checkItReadsBackWordForWord(t, keeper)
}

// TestAPlanStepADoneLineAndALessonWithCommasReadBackWordForWord holds every
// other line the model writes prose into to the same promise.
func TestAPlanStepADoneLineAndALessonWithCommasReadBackWordForWord(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	err := keeper.Apply(t.Context(), Update{
		DoneWhen: []contract.DoneLine{{Text: "one post is up, under 280 characters, with the date in it"}},
		StopWhen: []string{"the account shows a login page, a captcha, or a two-factor prompt"},
		Plan:     []string{"read the notes, the product ones", "draft the post, one fact only"},
		Decision: &NewDecision{Text: "Lead with the date, not the features", Reason: "correction C1, which says so"},
		Failure:  &NewFailure{Text: "Draft 1 was 312 characters, too long", Cause: "three facts in one post, so keep to one"},
	})
	if err != nil {
		t.Fatalf("prose with commas in it was refused: %v", err)
	}
	checkItReadsBackWordForWord(t, keeper)
}

// TestTheReadBackRefusalNamesOnlyAMarkATextCannotCarry holds the check that
// caught the defect to an honest refusal: a text that really does carry the
// due mark is still refused, the refusal names that mark, and it no longer
// names a comma, which any sentence may carry.
func TestTheReadBackRefusalNamesOnlyAMarkATextCannotCarry(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())

	err := keeper.Apply(t.Context(), Update{Tasks: []NewJobTask{{TaskID: contract.TaskID(1), Text: "post for day three" + dueMark + "tomorrow"}}})
	if err == nil {
		t.Fatalf("a task text carrying the due mark %q was accepted, and it reads back as a task with a due date", dueMark)
	}
	if errors.Is(err, ErrTasksOutOfOrder) || !strings.Contains(err.Error(), dueMark) {
		t.Errorf("the refusal reads %q and does not name the due mark %q", err, dueMark)
	}
	if strings.Contains(err.Error(), `", "`) {
		t.Errorf("the refusal reads %q and still names a comma, which any sentence may carry", err)
	}
}
