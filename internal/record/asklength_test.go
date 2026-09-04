package record

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// askOfWords writes an ask of exactly this many words, which is the shape of a
// specification somebody pastes into the terminal: the count is what the rule
// reads, so the words themselves are plain.
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

// TestATaskTakesADoneListAndAPlanOnAnAskAsLongAsTheCap holds the near side of
// the ask rule: an ask of exactly MaxAskWordsForATask words is still one
// sitting's ask, and a task on it keeps both its done list and its plan. A
// job's task is such a task, because its ask is the short text of one piece of
// the work, so this is also the case that leaves a job's tasks untouched.
func TestATaskTakesADoneListAndAPlanOnAnAskAsLongAsTheCap(t *testing.T) {
	keeper, _ := newKeeper(t, taskStartWithAnAskOf(MaxAskWordsForATask))

	if err := keeper.Apply(t.Context(), Update{DoneWhen: doneLinesOf(2), Plan: planStepsOf(3)}); err != nil {
		t.Fatalf("a done list and a plan on an ask of %d words were refused: %v", MaxAskWordsForATask, err)
	}
	held := keeper.Record()
	if len(held.Goal.DoneWhen) != 2 || len(held.Work.Plan) != 3 {
		t.Errorf("the record holds %d done lines and %d plan steps, want the 2 and 3 it was given",
			len(held.Goal.DoneWhen), len(held.Work.Plan))
	}
}

// TestATaskRefusesADoneListOnAnAskOneWordPastTheCapAndSaysItIsAJob is the rule
// itself. The done-list cap and the plan cap hold the model's own lists, and a
// small model met both by compressing: seven features became one done line and
// a whole game build a ten-step plan, so a very large ask still landed as one
// task. The ask is the one measure the model cannot compress, so a task whose
// ask runs past MaxAskWordsForATask words takes no done list at all, and the
// refusal says what to do instead, in the words of the two refusals beside it.
func TestATaskRefusesADoneListOnAnAskOneWordPastTheCapAndSaysItIsAJob(t *testing.T) {
	keeper, _ := newKeeper(t, taskStartWithAnAskOf(MaxAskWordsForATask+1))

	err := keeper.Apply(t.Context(), Update{
		Why:      "the user wants a whole game",
		DoneWhen: doneLinesOf(1),
	})
	if err == nil {
		t.Fatalf("a done list was taken on an ask of %d words, and the cap is %d", MaxAskWordsForATask+1, MaxAskWordsForATask)
	}
	if !errors.Is(err, ErrAskIsAJob) {
		t.Errorf("the refusal is %v, and it does not carry the named rule", err)
	}
	checkTheRefusalNamesTheJobTool(t, err)
	held := keeper.Record()
	if len(held.Goal.DoneWhen) != 0 || held.Goal.Why != "" {
		t.Errorf("the refused update was written anyway: the done list has %d lines and the why reads %q",
			len(held.Goal.DoneWhen), held.Goal.Why)
	}
}

// TestATaskRefusesAPlanOnAnAskOneWordPastTheCapAndSaysItIsAJob is the same rule
// on the plan, because a model that cannot write a done list on a long ask
// could otherwise still hide the work in a plan.
func TestATaskRefusesAPlanOnAnAskOneWordPastTheCapAndSaysItIsAJob(t *testing.T) {
	keeper, _ := newKeeper(t, taskStartWithAnAskOf(MaxAskWordsForATask+1))

	err := keeper.Apply(t.Context(), Update{
		Why:  "the user wants a whole game",
		Plan: planStepsOf(3),
	})
	if err == nil {
		t.Fatalf("a plan was taken on an ask of %d words, and the cap is %d", MaxAskWordsForATask+1, MaxAskWordsForATask)
	}
	if !errors.Is(err, ErrAskIsAJob) {
		t.Errorf("the refusal is %v, and it does not carry the named rule", err)
	}
	checkTheRefusalNamesTheJobTool(t, err)
	held := keeper.Record()
	if len(held.Work.Plan) != 0 || held.Goal.Why != "" {
		t.Errorf("the refused update was written anyway: the plan has %d steps and the why reads %q",
			len(held.Work.Plan), held.Goal.Why)
	}
}

// TestARefusedWriteOnALongAskLeavesWhatTheTaskAlreadyHadStanding proves the
// refusal is all or nothing on a task that already holds a stop list: a plan
// and a done list written together on a long ask land nowhere, and the stop
// list it had is exactly the stop list it has afterwards.
func TestARefusedWriteOnALongAskLeavesWhatTheTaskAlreadyHadStanding(t *testing.T) {
	keeper, _ := newKeeper(t, taskStartWithAnAskOf(MaxAskWordsForATask+1))
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{StopWhen: []string{"the account shows a login page"}}); err != nil {
		t.Fatalf("a stop list on a long ask was refused, and only the done list and the plan are held to the ask: %v", err)
	}
	before := keeper.Record()

	err := keeper.Apply(ctx, Update{DoneWhen: doneLinesOf(1), Plan: planStepsOf(2), StopWhen: []string{"a different line"}})
	if !errors.Is(err, ErrAskIsAJob) {
		t.Fatalf("a done list and a plan on a long ask were refused with %v, want the ask rule", err)
	}
	if after := keeper.Record(); !reflect.DeepEqual(after, before) {
		t.Errorf("the refused write changed the record: it was %+v and is now %+v", before, after)
	}
}

// TestAJobWithALongAskStillTakesItsTaskListAndDoneList holds the edge of the
// rule: it is a task's rule, because a task is one sitting. A job is the record
// a long ask belongs in, so a job on the same ask keeps its task list and its
// own done list whole.
func TestAJobWithALongAskStillTakesItsTaskListAndDoneList(t *testing.T) {
	start := jobStart()
	start.Ask = askOfWords(MaxAskWordsForATask + 1)
	keeper, _ := newKeeper(t, start)

	if err := keeper.Apply(t.Context(), Update{DoneWhen: doneLinesOf(3), Tasks: jobTasksOf(3)}); err != nil {
		t.Fatalf("a job's task list and done list on an ask of %d words were refused: %v", MaxAskWordsForATask+1, err)
	}
	held := keeper.Record()
	if len(held.Work.Tasks) != 3 || len(held.Goal.DoneWhen) != 3 {
		t.Errorf("the job holds %d tasks and %d done lines, want the 3 and 3 it was given",
			len(held.Work.Tasks), len(held.Goal.DoneWhen))
	}
}

// checkTheRefusalNamesTheJobTool holds the refusal to the words the model
// needs: how long the ask is, how long a task's may be, and what to do instead.
func checkTheRefusalNamesTheJobTool(t *testing.T, err error) {
	t.Helper()
	for _, told := range []string{
		strconv.Itoa(MaxAskWordsForATask+1) + " words", "at most " + strconv.Itoa(MaxAskWordsForATask),
		"this ask is a job", "job tool", "one task per piece of work", "one clear done line", "work the first task",
	} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
}
