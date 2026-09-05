package record

import (
	"strconv"
	"strings"
	"testing"
)

// aWholeGameAsk is how many words the game specification that was pasted into
// the terminal runs to, give or take: far past a paragraph, and the shape of
// ask this file holds the record to.
const aWholeGameAsk = 1300

// askOfWords writes an ask of exactly this many words, which is the shape of a
// specification somebody pastes into the terminal: the count is what matters,
// so the words themselves are plain.
func askOfWords(count int) string {
	words := make([]string, 0, count)
	for at := range count {
		words = append(words, "word"+strconv.Itoa(at+1))
	}
	return strings.Join(words, " ")
}

// taskStartWithAnAskOf is the task the tests start from, with an ask of the
// length asked for in place of the fixture's two sentences.
func taskStartWithAnAskOf(words int) Start {
	start := taskStart()
	start.Ask = askOfWords(words)
	return start
}

// TestATaskTakesADoneListAndAPlanOnAnAskOfAnyLength holds that the length of
// the ask never decides what the record takes. The record used to refuse a
// done list and a plan on any ask over 250 words and tell the model to make a
// job instead; on the live game build the small local model made the job and
// then kept working in the task anyway, so the task ran a hundred and
// forty-five rounds with no plan, no done list and nothing for the done-check
// to close on. A long ask keeps its done list and its plan, held to the same
// five lines and ten steps as any other, and the job tool stays there for the
// model to reach for.
func TestATaskTakesADoneListAndAPlanOnAnAskOfAnyLength(t *testing.T) {
	keeper, _ := newKeeper(t, taskStartWithAnAskOf(aWholeGameAsk))

	if err := keeper.Apply(t.Context(), Update{
		Why:      "the user wants a whole game",
		DoneWhen: doneLinesOf(2),
		Plan:     planStepsOf(3),
	}); err != nil {
		t.Fatalf("a done list and a plan on an ask of %d words were refused: %v", aWholeGameAsk, err)
	}
	held := keeper.Record()
	if len(held.Goal.DoneWhen) != 2 || len(held.Work.Plan) != 3 || held.Goal.Why == "" {
		t.Errorf("the record holds %d done lines, %d plan steps and the why %q, want the 2, 3 and why it was given",
			len(held.Goal.DoneWhen), len(held.Work.Plan), held.Goal.Why)
	}
	if strings.Fields(held.Goal.Ask)[aWholeGameAsk-1] != "word"+strconv.Itoa(aWholeGameAsk) {
		t.Errorf("the ask was not kept whole: it ends %q", held.Goal.Ask[len(held.Goal.Ask)-20:])
	}
}

// TestAJobWithALongAskStillTakesItsTaskListAndDoneList holds that a job on a
// long ask keeps its task list and its own done list whole, which is the record
// a long ask still belongs in when the model chooses to make one.
func TestAJobWithALongAskStillTakesItsTaskListAndDoneList(t *testing.T) {
	start := jobStart()
	start.Ask = askOfWords(aWholeGameAsk)
	keeper, _ := newKeeper(t, start)

	if err := keeper.Apply(t.Context(), Update{DoneWhen: doneLinesOf(3), Tasks: jobTasksOf(3)}); err != nil {
		t.Fatalf("a job's task list and done list on an ask of %d words were refused: %v", aWholeGameAsk, err)
	}
	held := keeper.Record()
	if len(held.Work.Tasks) != 3 || len(held.Goal.DoneWhen) != 3 {
		t.Errorf("the job holds %d tasks and %d done lines, want the 3 and 3 it was given",
			len(held.Work.Tasks), len(held.Goal.DoneWhen))
	}
}
