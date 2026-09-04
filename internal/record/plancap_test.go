package record

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// planStepsOf writes a plan of the length asked for, every step waiting to be
// done, which is the shape a model writes at the start of a task.
func planStepsOf(count int) []string {
	steps := make([]string, 0, count)
	for at := range count {
		steps = append(steps, "build piece "+strconv.Itoa(at+1)+" of the work")
	}
	return steps
}

// jobTasksOf writes a job's task list of the length asked for, one task per
// piece of the work, which is what a plan too long for one task should have
// been written as.
func jobTasksOf(count int) []NewJobTask {
	tasks := make([]NewJobTask, 0, count)
	for at := range count {
		tasks = append(tasks, NewJobTask{TaskID: contract.TaskID(at + 1), Text: "build piece " + strconv.Itoa(at+1) + " of the work"})
	}
	return tasks
}

// TestATaskTakesAPlanAsLongAsTheCap holds the near side of the plan rule: a
// plan of exactly MaxPlanSteps steps is one sitting's plan, and a task keeps
// every step, numbered from one.
func TestATaskTakesAPlanAsLongAsTheCap(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	if err := keeper.Apply(t.Context(), Update{Plan: planStepsOf(MaxPlanSteps)}); err != nil {
		t.Fatalf("a plan of %d steps was refused: %v", MaxPlanSteps, err)
	}
	held := keeper.Record().Work.Plan
	if len(held) != MaxPlanSteps {
		t.Fatalf("the record holds %d plan steps, want the %d it was given", len(held), MaxPlanSteps)
	}
	if last := held[MaxPlanSteps-1]; last.Number != MaxPlanSteps || last.Done {
		t.Errorf("the last step is %+v, and it should be step %d with nothing done yet", last, MaxPlanSteps)
	}
}

// TestATaskRefusesAPlanOneStepPastTheCapAndSaysItIsAJob is the rule itself.
// Given a whole game with its tests and its browser play-testing, a small model
// kept the done list at five lines and hid the whole build in a twelve-step
// plan on one task, because the plan had no cap. The harness decides now: a
// plan past MaxPlanSteps is refused, and the refusal says what to do instead,
// in the same words as the done-list refusal.
func TestATaskRefusesAPlanOneStepPastTheCapAndSaysItIsAJob(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	err := keeper.Apply(t.Context(), Update{
		Why:  "the user wants a whole game",
		Plan: planStepsOf(MaxPlanSteps + 1),
	})
	if err == nil {
		t.Fatalf("a plan of %d steps was taken by a task, and the cap is %d", MaxPlanSteps+1, MaxPlanSteps)
	}
	if !errors.Is(err, ErrPlanTooLong) {
		t.Errorf("the refusal is %v, and it does not carry the named rule", err)
	}
	for _, told := range []string{
		strconv.Itoa(MaxPlanSteps+1) + " steps", "at most " + strconv.Itoa(MaxPlanSteps),
		"this ask is a job", "job tool", "one task per step", "one clear done line", "work the first task",
	} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
	held := keeper.Record()
	if len(held.Work.Plan) != 0 || held.Goal.Why != "" {
		t.Errorf("the refused update was written anyway: the plan has %d steps and the why reads %q",
			len(held.Work.Plan), held.Goal.Why)
	}
}

// TestARefusedPlanLeavesThePlanBeforeItStanding proves the refusal is all or
// nothing on a task that already has a plan with work done on it: the plan it
// had, check mark and result included, is exactly the plan it has afterwards.
func TestARefusedPlanLeavesThePlanBeforeItStanding(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Plan: []string{"read the notes", "draft the post", "post it"}}); err != nil {
		t.Fatalf("the first plan was refused: %v", err)
	}
	resultID, err := keeper.AddResult(ctx, "read memory/product.md, 2,100 characters", "the notes")
	if err != nil {
		t.Fatalf("cannot add a result: %v", err)
	}
	if err := keeper.MarkPlanStep(ctx, 1, resultID); err != nil {
		t.Fatalf("cannot mark the first step done: %v", err)
	}
	before := keeper.Record().Work.Plan

	if err := keeper.Apply(ctx, Update{Plan: planStepsOf(MaxPlanSteps + 1)}); !errors.Is(err, ErrPlanTooLong) {
		t.Fatalf("a plan of %d steps was refused with %v, want the too-long rule", MaxPlanSteps+1, err)
	}
	if after := keeper.Record().Work.Plan; !reflect.DeepEqual(after, before) {
		t.Errorf("the refused plan changed the record: the plan was %+v and is now %+v", before, after)
	}
}

// TestAJobIsNotHeldToATasksPlanCap holds the edge of the rule: it is a task's
// rule, because a task is one sitting. A job has no plan at all, so a plan one
// step past the cap on a job is refused as the wrong kind and never as too
// long, and the job's own task list of the same length, which is what such a
// plan should have been written as, is kept whole.
func TestAJobIsNotHeldToATasksPlanCap(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())
	ctx := t.Context()

	err := keeper.Apply(ctx, Update{Plan: planStepsOf(MaxPlanSteps + 1)})
	if !errors.Is(err, ErrWrongKind) || errors.Is(err, ErrPlanTooLong) {
		t.Errorf("a plan on a job was refused with %v, and it should be the wrong kind, not too long", err)
	}
	if err := keeper.Apply(ctx, Update{Tasks: jobTasksOf(MaxPlanSteps + 1)}); err != nil {
		t.Fatalf("a job's task list of %d tasks was refused: %v", MaxPlanSteps+1, err)
	}
	if held := len(keeper.Record().Work.Tasks); held != MaxPlanSteps+1 {
		t.Errorf("the job holds %d tasks, want the %d it was given", held, MaxPlanSteps+1)
	}
}
