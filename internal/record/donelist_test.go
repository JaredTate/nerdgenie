package record

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// doneLinesOf writes a done list of the length asked for, every line waiting
// for its proof, which is the shape a model writes at the start of a task.
func doneLinesOf(count int) []contract.DoneLine {
	lines := make([]contract.DoneLine, 0, count)
	for at := range count {
		lines = append(lines, contract.DoneLine{Text: "piece " + strconv.Itoa(at+1) + " of the work is finished"})
	}
	return lines
}

// TestATaskTakesADoneListOfFiveLines holds the near side of the five-line
// rule: five lines is one sitting's worth of done, and a task keeps them.
func TestATaskTakesADoneListOfFiveLines(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	if err := keeper.Apply(t.Context(), Update{DoneWhen: doneLinesOf(MaxDoneLines)}); err != nil {
		t.Fatalf("a done list of %d lines was refused: %v", MaxDoneLines, err)
	}
	if held := len(keeper.Record().Goal.DoneWhen); held != MaxDoneLines {
		t.Errorf("the record holds %d done lines, want the %d it was given", held, MaxDoneLines)
	}
}

// TestATaskRefusesADoneListOfSixLinesAndSaysItIsAJob is the rule itself. A
// whole game with two hazard systems, three animation systems, a test suite and
// a round of play-testing was written down as one task with an eight-line done
// list, and the model started coding, because whether an ask is a job was left
// to its judgment and a small model never says yes. The harness decides now: a
// done list past five lines is refused, and the refusal says what to do instead.
func TestATaskRefusesADoneListOfSixLinesAndSaysItIsAJob(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	err := keeper.Apply(t.Context(), Update{
		Why:      "the user wants a whole game",
		DoneWhen: doneLinesOf(MaxDoneLines + 1),
	})
	if err == nil {
		t.Fatalf("a done list of %d lines was taken by a task, and the cap is %d", MaxDoneLines+1, MaxDoneLines)
	}
	if !errors.Is(err, ErrDoneListTooLong) {
		t.Errorf("the refusal is %v, and it does not carry the named rule", err)
	}
	for _, told := range []string{
		"6 lines", "at most 5", "this ask is a job", "job tool",
		"one task per done line", "one clear done line", "work the first task",
	} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
	held := keeper.Record()
	if len(held.Goal.DoneWhen) != 0 || held.Goal.Why != "" {
		t.Errorf("the refused update was written anyway: the done list has %d lines and the why reads %q",
			len(held.Goal.DoneWhen), held.Goal.Why)
	}
}

// TestAJobTakesADoneListLongerThanATasks holds the edge of the rule: it is a
// task's rule, because a task is one sitting, and a job made of eight tasks
// keeps eight done lines of its own.
func TestAJobTakesADoneListLongerThanATasks(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())

	if err := keeper.Apply(t.Context(), Update{DoneWhen: doneLinesOf(MaxDoneLines + 3)}); err != nil {
		t.Fatalf("a job's done list of %d lines was refused: %v", MaxDoneLines+3, err)
	}
	if held := len(keeper.Record().Goal.DoneWhen); held != MaxDoneLines+3 {
		t.Errorf("the job holds %d done lines, want the %d it was given", held, MaxDoneLines+3)
	}
}
