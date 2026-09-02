package record

import (
	"errors"
	"testing"
)

// jobWithTasks makes a job whose list holds three tasks, the first of which is
// finished and has its report behind it.
func jobWithTasks(t *testing.T) *Keeper {
	t.Helper()
	keeper, _ := newKeeper(t, jobStart())
	ctx := t.Context()

	tasks := []NewJobTask{
		{TaskID: "t17", Text: "post the anniversary tweet"},
		{TaskID: "t19", Text: "draft the blog piece"},
		{TaskID: "t31", Text: "post for day three", DueAt: "today at 14:00"},
	}
	if err := keeper.Apply(ctx, Update{Tasks: tasks}); err != nil {
		t.Fatalf("cannot write the task list: %v", err)
	}
	if _, err := keeper.AddReport(ctx, "posted, 236 characters, link saved", "the whole report"); err != nil {
		t.Fatalf("cannot add the report: %v", err)
	}
	if err := keeper.MarkJobTask(ctx, "t17", "j4.1"); err != nil {
		t.Fatalf("cannot mark the first task done: %v", err)
	}
	return keeper
}

// TestAJobKeepsItsTasksInOrderAndKeepsTheFinishedOnes proves the three rules a
// job's task list has of its own.
func TestAJobKeepsItsTasksInOrderAndKeepsTheFinishedOnes(t *testing.T) {
	keeper := jobWithTasks(t)
	ctx := t.Context()

	held := keeper.Record().Work.Tasks
	if len(held) != 3 || !held[0].Done || held[0].ReportID != "j4.1" {
		t.Fatalf("the task list reads %+v after one task finished", held)
	}
	if held[2].DueAt != "today at 14:00" {
		t.Errorf("the task that must wait for a date lost it: %+v", held[2])
	}

	split := []NewJobTask{
		{TaskID: "t17", Text: "post the anniversary tweet"},
		{TaskID: "t19", Text: "draft the blog piece"},
		{TaskID: "t31", Text: "post for day three", DueAt: "today at 14:00"},
		{TaskID: "t32", Text: "post for day four", DueAt: "tomorrow at 14:00"},
	}
	if err := keeper.Apply(ctx, Update{Tasks: split}); err != nil {
		t.Fatalf("a task added to the end of the list was refused: %v", err)
	}
	if grown := keeper.Record().Work.Tasks; len(grown) != 4 || !grown[0].Done || grown[0].ReportID != "j4.1" {
		t.Errorf("the finished task lost its report when the list grew: %+v", grown)
	}
}

// TestRefusesToRemoveAFinishedTask proves the rule that a task that is done stays
// on the list for good.
func TestRefusesToRemoveAFinishedTask(t *testing.T) {
	keeper := jobWithTasks(t)
	without := []NewJobTask{{TaskID: "t19", Text: "draft the blog piece"}}
	if err := keeper.Apply(t.Context(), Update{Tasks: without}); !errors.Is(err, ErrFinishedTaskRemoved) {
		t.Errorf("a finished task was taken off the list: %v", err)
	}
}

// TestRefusesATaskListThatIsOutOfOrder proves the rule that a job lists its tasks
// in order, so a later task can lean on the reports of the ones before it.
func TestRefusesATaskListThatIsOutOfOrder(t *testing.T) {
	keeper := jobWithTasks(t)
	ctx := t.Context()

	backwards := []NewJobTask{
		{TaskID: "t17", Text: "post the anniversary tweet"},
		{TaskID: "t19", Text: "draft the blog piece"},
		{TaskID: "t31", Text: "post for day three"},
		{TaskID: "t20", Text: "a task that belongs earlier"},
	}
	if err := keeper.Apply(ctx, Update{Tasks: backwards}); !errors.Is(err, ErrTasksOutOfOrder) {
		t.Errorf("a task list out of order was accepted: %v", err)
	}
	badLabel := []NewJobTask{{TaskID: "x17", Text: "post the anniversary tweet"}}
	if err := keeper.Apply(ctx, Update{Tasks: badLabel}); err == nil {
		t.Error("a task whose label is no task label was accepted")
	}
	noText := []NewJobTask{{TaskID: "t17", Text: ""}}
	if err := keeper.Apply(ctx, Update{Tasks: noText}); err == nil {
		t.Error("a task with no text was accepted")
	}
}

// TestMarksAJobTaskDoneOnlyWithARealReport proves the rule that a task marked
// done must name the report it wrote.
func TestMarksAJobTaskDoneOnlyWithARealReport(t *testing.T) {
	keeper := jobWithTasks(t)
	ctx := t.Context()

	if err := keeper.MarkJobTask(ctx, "t19", "j4.9"); !errors.Is(err, ErrJobTaskNeedsReport) {
		t.Errorf("a task was marked done by a report this job never wrote: %v", err)
	}
	if err := keeper.MarkJobTask(ctx, "t44", "j4.1"); err == nil {
		t.Error("a task that is not on the list was marked done")
	}
	task, _ := newKeeper(t, taskStart())
	if err := task.MarkJobTask(ctx, "t1", "j4.1"); !errors.Is(err, ErrWrongKind) {
		t.Errorf("a task record took a job's check mark: %v", err)
	}
}

// TestMarksAPlanStepDoneOnlyWithARealResult proves the same rule for a task's
// plan.
func TestMarksAPlanStepDoneOnlyWithARealResult(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if err := keeper.Apply(ctx, Update{Plan: []string{"read the notes", "draft the post"}}); err != nil {
		t.Fatalf("cannot write the plan: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "read the notes", "the whole file"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}

	if err := keeper.MarkPlanStep(ctx, 2, "r9"); !errors.Is(err, ErrPlanStepNeedsResult) {
		t.Errorf("a plan step was marked done by a result this task never wrote: %v", err)
	}
	if err := keeper.MarkPlanStep(ctx, 7, "r1"); err == nil {
		t.Error("a plan step that is not in the plan was marked done")
	}
	if err := keeper.MarkPlanStep(ctx, 1, "r1"); err != nil {
		t.Fatalf("a plan step naming its own result was refused: %v", err)
	}
	if held := keeper.Record().Work.Plan[0]; !held.Done || held.ResultID != "r1" {
		t.Errorf("the first plan step reads %+v after it was marked done", held)
	}
	job, _ := newKeeper(t, jobStart())
	if err := job.MarkPlanStep(ctx, 1, "r1"); !errors.Is(err, ErrWrongKind) {
		t.Errorf("a job took a plan step's check mark: %v", err)
	}
}
