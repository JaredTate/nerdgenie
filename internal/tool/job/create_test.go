package job_test

// These tests are the create action: where the job's ask comes from, and how
// its whole task list is written in one call.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/job"
)

// heldRecord is the record of the task running now, as the tool reads it: a
// struct that hands back the one record it holds.
type heldRecord struct {
	record contract.Record
}

// Record returns the record as it stands.
func (held heldRecord) Record() contract.Record { return held.record }

// newToolOverRecord builds the job tool over the fake job store and a record
// whose ask is the one given.
func newToolOverRecord(t *testing.T, ask string) (*job.Tool, *testkit.FakeJob) {
	t.Helper()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theMoment))
	records := heldRecord{record: contract.Record{Goal: contract.Goal{Ask: ask}}}
	return job.New(job.Settings{Jobs: jobs, Records: records}), jobs
}

// askOf is the ask the store holds for one job.
func askOf(t *testing.T, jobs *testkit.FakeJob, jobID string) string {
	t.Helper()
	loaded, err := jobs.Load(context.Background(), jobID)
	if err != nil {
		t.Fatalf("cannot load job %s: %v", jobID, err)
	}
	return loaded.Goal.Ask
}

// TestACreateWithNoAskTakesTheRunningTasksAskWordForWord proves the model need
// not retype the ask: a long one, longer than the working context shows, reaches
// the job byte for byte from the record of the task running now.
func TestACreateWithNoAskTakesTheRunningTasksAskWordForWord(t *testing.T) {
	longAsk := "Build a complete, polished, playable Tetris-style web game with dragons and yetis.\n\n" +
		strings.Repeat("Every piece has its own colour and its own sound, and the yeti roars when a line clears. ", 60)
	tool, jobs := newToolOverRecord(t, longAsk)

	out, err := run(t, tool, map[string]any{
		"action": "create",
		"name":   "Yeti Tetris",
		"why":    "the user wants the whole game built and tested",
		"text":   "write the failing tests for the core engine",
	})
	if err != nil {
		t.Fatalf("creating a job with no ask over a record was refused: %v", err)
	}
	if !strings.Contains(out.Text, "1") {
		t.Errorf("creating the job said %q and did not name it", out.Text)
	}
	if held := askOf(t, jobs, "1"); held != longAsk {
		t.Errorf("the job's ask is %d bytes and differs from the record's %d bytes, want the record's ask word for word", len(held), len(longAsk))
	}
}

// TestTheRecordsAskWinsOverOneTheModelWrote proves that an ask the model wrote
// beside the record's is dropped without a word: the user's words, never the
// model's.
func TestTheRecordsAskWinsOverOneTheModelWrote(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "post a tweet about the DigiByte anniversary, under 280 characters")

	if _, err := run(t, tool, map[string]any{
		"action": "create",
		"ask":    "Post an anniversary tweet.",
		"why":    "mark the anniversary publicly",
		"text":   "draft the post, done when it is under 280 characters",
	}); err != nil {
		t.Fatalf("creating a job with a differing ask was refused: %v", err)
	}
	if held := askOf(t, jobs, "1"); held != "post a tweet about the DigiByte anniversary, under 280 characters" {
		t.Errorf("the job's ask is %q, want the record's, not the model's paraphrase", held)
	}
}

// TestACreateWithNoRecordTakesTheAskTheModelWrote keeps the ask field working
// for a tool built with no record behind it.
func TestACreateWithNoRecordTakesTheAskTheModelWrote(t *testing.T) {
	tool, jobs := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"action": "create", "ask": "the ask the model wrote", "why": "the why", "text": "the first task",
	}); err != nil {
		t.Fatalf("creating a job from the ask field was refused: %v", err)
	}
	if held := askOf(t, jobs, "1"); held != "the ask the model wrote" {
		t.Errorf("the job's ask is %q, want the one the model wrote", held)
	}
}

// TestACreateWithNoAskAnywhereIsRefused covers a record that carries no ask yet
// and a model that wrote none either: there is nothing to make the job from.
func TestACreateWithNoAskAnywhereIsRefused(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "")

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "the first task"})
	if err == nil {
		t.Fatalf("a create with no ask in the record and none in the call made a job")
	}
	if !strings.Contains(err.Error(), "ask") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused create still left %d jobs behind", len(summaries))
	}
}

// TestACreateWithATaskListWritesEveryTaskInOrder proves the whole task list is
// written in one call: text first, then the list in order, the first one left
// as the task the loop runs next, and every label named in the reply.
func TestACreateWithATaskListWritesEveryTaskInOrder(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "build a whole tetris game i can play in the terminal")
	due := theMoment.Add(24 * time.Hour)

	out, err := run(t, tool, map[string]any{
		"action": "create",
		"name":   "Terminal Tetris",
		"why":    "the user wants to play tetris",
		"text":   "set up the game board, done when the empty grid renders",
		"tasks": []map[string]any{
			{"text": "draw the seven pieces, done when each one renders"},
			{"text": "add the yeti, done when it roars on a cleared line", "due_at": due.Format(time.RFC3339)},
			{"text": "play-test it, done when a full game runs through"},
		},
	})
	if err != nil {
		t.Fatalf("creating a job with a task list was refused: %v", err)
	}

	tasks := jobs.Tasks("1")
	wanted := []string{
		"set up the game board, done when the empty grid renders",
		"draw the seven pieces, done when each one renders",
		"add the yeti, done when it roars on a cleared line",
		"play-test it, done when a full game runs through",
	}
	if len(tasks) != len(wanted) {
		t.Fatalf("the job holds %d tasks, want the first task and the three listed", len(tasks))
	}
	for at, task := range tasks {
		if task.Text != wanted[at] {
			t.Errorf("task %d is %q, want %q: the list is written in order", at+1, task.Text, wanted[at])
		}
		if task.Done {
			t.Errorf("task %s is already marked done, want every task left to work", task.TaskID)
		}
		if !strings.Contains(out.Text, task.TaskID) {
			t.Errorf("creating the job said %q and did not name task %s", out.Text, task.TaskID)
		}
	}
	if tasks[2].DueAt == "" {
		t.Errorf("the dated task on the list is %v, want one that waits for its date", tasks[2])
	}
	summaries, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(summaries) != 1 || summaries[0].NextTaskID != tasks[0].TaskID {
		t.Errorf("the job's next task is %q, want the first task %q", summaries[0].NextTaskID, tasks[0].TaskID)
	}
}

// TestATaskListAloneIsEnoughForACreate proves a create needs no text when the
// list carries the first task.
func TestATaskListAloneIsEnoughForACreate(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")

	if _, err := run(t, tool, map[string]any{
		"action": "create", "why": "the why",
		"tasks": []map[string]any{{"text": "the first task"}, {"text": "the second task"}},
	}); err != nil {
		t.Fatalf("creating a job from a task list with no text was refused: %v", err)
	}
	if tasks := jobs.Tasks("1"); len(tasks) != 2 || tasks[0].Text != "the first task" {
		t.Errorf("the job holds %v, want the two listed tasks in order", tasks)
	}
}

// TestATaskListLongerThanTheCapIsRefusedAndMakesNoJob pins the bound on the
// list and proves a longer one is refused before any job is made.
func TestATaskListLongerThanTheCapIsRefusedAndMakesNoJob(t *testing.T) {
	if job.MaxTasksOnCreate != 25 {
		t.Errorf("MaxTasksOnCreate is %d, want twenty-five, the same number a listing shows", job.MaxTasksOnCreate)
	}
	tool, jobs := newToolOverRecord(t, "the ask")

	listed := make([]map[string]any, 0, job.MaxTasksOnCreate+1)
	for at := range job.MaxTasksOnCreate + 1 {
		listed = append(listed, map[string]any{"text": fmt.Sprintf("task %d", at+1)})
	}
	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": listed})
	if err == nil {
		t.Fatalf("a task list of %d was taken, and the cap is %d", len(listed), job.MaxTasksOnCreate)
	}
	if !strings.Contains(err.Error(), "add_task") {
		t.Errorf("the refusal reads %q and does not tell the model what to do with the rest", err)
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused create still left %d jobs behind", len(summaries))
	}

	if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": listed[:job.MaxTasksOnCreate]}); err != nil {
		t.Errorf("a task list of exactly %d was refused: %v", job.MaxTasksOnCreate, err)
	}
	if tasks := jobs.Tasks("1"); len(tasks) != job.MaxTasksOnCreate {
		t.Errorf("the job holds %d tasks, want all %d listed", len(tasks), job.MaxTasksOnCreate)
	}
}

// TestABadTaskOnTheListIsRefusedBeforeAnyJobIsMade proves that a task that says
// nothing, or one with a date nobody can read, refuses the whole create, so the
// store never holds a job with half its list.
func TestABadTaskOnTheListIsRefusedBeforeAnyJobIsMade(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")

	for _, broken := range [][]map[string]any{
		{{"text": "the first task"}, {"text": "   "}},
		{{"text": "the first task"}, {"text": "the second task", "due_at": "soon"}},
	} {
		if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": broken}); err == nil {
			t.Errorf("the task list %v was taken", broken)
		}
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused creates still left %d jobs behind", len(summaries))
	}
}

// TestAScheduledJobOverARecordNeedsNoAskAndNoTasks proves the two new rules
// leave a scheduled job alone: its template makes its tasks, and its ask comes
// from the record like any other.
func TestAScheduledJobOverARecordNeedsNoAskAndNoTasks(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "check the site every morning")

	if _, err := run(t, tool, map[string]any{
		"action": "create", "why": "the user wants to know it is up",
		"schedule": map[string]any{"kind": "every", "every": "24h"}, "task_template": "check the site and report",
	}); err != nil {
		t.Fatalf("creating a scheduled job with no ask and no tasks was refused: %v", err)
	}
	if held := askOf(t, jobs, "1"); held != "check the site every morning" {
		t.Errorf("the scheduled job's ask is %q, want the record's", held)
	}
	if tasks := jobs.Tasks("1"); len(tasks) != 0 {
		t.Errorf("the scheduled job holds %d tasks, want none until its clock ticks", len(tasks))
	}
}

// TestATaskListOfPlainStringsGivesUndatedTasksInOrder proves the shape a small
// model writes most, a list of strings, is taken as undated tasks in order.
func TestATaskListOfPlainStringsGivesUndatedTasksInOrder(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")

	if _, err := run(t, tool, map[string]any{
		"action": "create", "why": "the why",
		"tasks": []any{"write the failing tests", "implement the engine"},
	}); err != nil {
		t.Fatalf("creating a job from a list of strings was refused: %v", err)
	}
	tasks := jobs.Tasks("1")
	if len(tasks) != 2 || tasks[0].Text != "write the failing tests" || tasks[1].Text != "implement the engine" {
		t.Fatalf("the job holds %v, want the two strings as tasks in order", tasks)
	}
	for _, task := range tasks {
		if task.DueAt != "" {
			t.Errorf("task %s waits for %q, want a task written as a string to carry no date", task.TaskID, task.DueAt)
		}
	}
}

// TestAMixedTaskListKeepsItsOrderAndItsDate proves strings and objects can be
// listed together, each read in its own shape.
func TestAMixedTaskListKeepsItsOrderAndItsDate(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")
	due := theMoment.Add(24 * time.Hour)

	if _, err := run(t, tool, map[string]any{
		"action": "create", "why": "the why",
		"tasks": []any{
			"write the failing tests",
			map[string]any{"text": "implement the engine", "due_at": due.Format(time.RFC3339)},
			"play-test it",
		},
	}); err != nil {
		t.Fatalf("creating a job from a mixed list was refused: %v", err)
	}
	tasks := jobs.Tasks("1")
	wanted := []string{"write the failing tests", "implement the engine", "play-test it"}
	if len(tasks) != len(wanted) {
		t.Fatalf("the job holds %d tasks, want the three listed", len(tasks))
	}
	for at, task := range tasks {
		if task.Text != wanted[at] {
			t.Errorf("task %d is %q, want %q: the list is written in order", at+1, task.Text, wanted[at])
		}
	}
	if tasks[0].DueAt != "" || tasks[1].DueAt == "" || tasks[2].DueAt != "" {
		t.Errorf("the dates on the list are %q, %q, %q, want only the second task dated", tasks[0].DueAt, tasks[1].DueAt, tasks[2].DueAt)
	}
}

// TestATaskInAShapeTheToolCannotReadIsRefusedByItsPosition proves a number, a
// null, a nested list, or a boolean where a task should be refuses the call by
// the item's position, in words the model can act on, before any job is made.
func TestATaskInAShapeTheToolCannotReadIsRefusedByItsPosition(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")

	for _, broken := range []any{7, nil, []any{"a task inside a list"}, true} {
		_, err := run(t, tool, map[string]any{
			"action": "create", "why": "the why", "tasks": []any{"the first task", broken, "the third task"},
		})
		if err == nil {
			t.Errorf("a task written as %v was taken", broken)
			continue
		}
		if !strings.Contains(err.Error(), "task 2") || !strings.Contains(err.Error(), "string") {
			t.Errorf("the refusal for a task written as %v reads %q and does not name position two and the shapes to write", broken, err)
		}
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused creates still left %d jobs behind", len(summaries))
	}
}

// TestTheCapAndTheBlankRuleHoldForPlainStringTasks proves the bound on the list
// and the refusal of a task that says nothing read a string the same as an
// object.
func TestTheCapAndTheBlankRuleHoldForPlainStringTasks(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")

	listed := make([]any, 0, job.MaxTasksOnCreate+1)
	for at := range job.MaxTasksOnCreate + 1 {
		listed = append(listed, fmt.Sprintf("task %d", at+1))
	}
	if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": listed}); err == nil {
		t.Errorf("a list of %d strings was taken, and the cap is %d", len(listed), job.MaxTasksOnCreate)
	} else if !strings.Contains(err.Error(), "add_task") {
		t.Errorf("the refusal reads %q and does not tell the model what to do with the rest", err)
	}
	if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": []any{"the first task", "   "}}); err == nil {
		t.Errorf("a blank string where a task should be was taken")
	} else if !strings.Contains(err.Error(), "task 2") {
		t.Errorf("the refusal reads %q and does not name the blank task's position", err)
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused creates still left %d jobs behind", len(summaries))
	}
}

// errTheStoreRefused is the reason a broken store gives, which the model must
// read in full so that it does not try the same create again.
var errTheStoreRefused = errors.New("the job store cannot write the task because the disk is full")

// brokenJobs is the fake job store with one of its writes made to fail, so that
// a create can be watched leaving nothing behind when the store fails halfway.
type brokenJobs struct {
	*testkit.FakeJob
	// createFails makes every Create fail.
	createFails bool
	// addTaskFailsAt is which AddTask call fails, counting from one, or zero
	// for none.
	addTaskFailsAt int
	// switchOffFails makes every SwitchOff fail.
	switchOffFails bool
	// addTaskCalls counts the AddTask calls so far.
	addTaskCalls int
}

// Create fails when told to, and otherwise makes the job.
func (jobs *brokenJobs) Create(ctx context.Context, wanted contract.NewJob) (string, error) {
	if jobs.createFails {
		return "", errTheStoreRefused
	}
	return jobs.FakeJob.Create(ctx, wanted)
}

// AddTask fails on the call it was told to, and otherwise records the task.
func (jobs *brokenJobs) AddTask(ctx context.Context, wanted contract.NewTask) (string, error) {
	jobs.addTaskCalls++
	if jobs.addTaskCalls == jobs.addTaskFailsAt {
		return "", errTheStoreRefused
	}
	return jobs.FakeJob.AddTask(ctx, wanted)
}

// SwitchOff fails when told to, and otherwise switches the job off.
func (jobs *brokenJobs) SwitchOff(ctx context.Context, jobID string) error {
	if jobs.switchOffFails {
		return errors.New("the job store cannot change the job's state")
	}
	return jobs.FakeJob.SwitchOff(ctx, jobID)
}

// newToolOverBrokenJobs builds the job tool over a record and a broken store.
func newToolOverBrokenJobs(t *testing.T, broken *brokenJobs) *job.Tool {
	t.Helper()
	broken.FakeJob = testkit.NewFakeJob(testkit.NewFakeClock(theMoment))
	records := heldRecord{record: contract.Record{Goal: contract.Goal{Ask: "the ask"}}}
	return job.New(job.Settings{Jobs: broken, Records: records})
}

// runningJobs is how many of the listed jobs are running.
func runningJobs(t *testing.T, jobs contract.Job) int {
	t.Helper()
	summaries, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	running := 0
	for _, summary := range summaries {
		if summary.State == contract.JobRunning {
			running++
		}
	}
	return running
}

// TestACreateWhoseTaskListCannotBeRecordedSwitchesTheJobOffAndSaysWhy proves a
// create is all or nothing: when the store refuses a task after the job is made,
// the job is switched off so no empty running job is left for a retry to stand
// beside, and the refusal names the job and carries the store's reason in full.
// On the real store a retrying model left twenty-nine empty running jobs behind.
func TestACreateWhoseTaskListCannotBeRecordedSwitchesTheJobOffAndSaysWhy(t *testing.T) {
	broken := &brokenJobs{addTaskFailsAt: 2}
	tool := newToolOverBrokenJobs(t, broken)

	_, err := run(t, tool, map[string]any{
		"action": "create", "why": "the why", "tasks": []any{"the first task", "the second task", "the third task"},
	})
	if err == nil {
		t.Fatalf("a create whose second task the store refused was reported as done")
	}
	if !errors.Is(err, errTheStoreRefused) || !strings.Contains(err.Error(), errTheStoreRefused.Error()) {
		t.Errorf("the refusal reads %q and does not carry the store's reason in full", err)
	}
	for _, told := range []string{"job 1", "switched off"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
	if running := runningJobs(t, broken); running != 0 {
		t.Errorf("%d jobs are still running after the failed create, want none", running)
	}
	summaries, listErr := broken.List(context.Background())
	if listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	}
	if len(summaries) != 1 || summaries[0].State != contract.JobOff {
		t.Errorf("the store holds %v, want the one job switched off", summaries)
	}
}

// TestACreateWhoseJobCannotBeMadeLeavesNothing proves a store that refuses the
// job itself leaves nothing behind and its reason reaches the model in full.
func TestACreateWhoseJobCannotBeMadeLeavesNothing(t *testing.T) {
	broken := &brokenJobs{createFails: true}
	tool := newToolOverBrokenJobs(t, broken)

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": []any{"the first task"}})
	if err == nil {
		t.Fatalf("a create the store refused was reported as done")
	}
	if !errors.Is(err, errTheStoreRefused) || !strings.Contains(err.Error(), errTheStoreRefused.Error()) {
		t.Errorf("the refusal reads %q and does not carry the store's reason in full", err)
	}
	if summaries, listErr := broken.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused create still left %d jobs behind", len(summaries))
	}
}

// TestACreateThatCannotSwitchItsBrokenJobOffSaysSo proves that when the store
// refuses a task and then refuses to switch the job off, the refusal says the
// job may be running with half its list, so the user can be told.
func TestACreateThatCannotSwitchItsBrokenJobOffSaysSo(t *testing.T) {
	broken := &brokenJobs{addTaskFailsAt: 1, switchOffFails: true}
	tool := newToolOverBrokenJobs(t, broken)

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "tasks": []any{"the first task"}})
	if err == nil {
		t.Fatalf("a create whose task the store refused was reported as done")
	}
	if !errors.Is(err, errTheStoreRefused) {
		t.Errorf("the refusal reads %q and does not carry the store's reason", err)
	}
	for _, told := range []string{"job 1", "cannot change the job's state", "tell the user"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
}
