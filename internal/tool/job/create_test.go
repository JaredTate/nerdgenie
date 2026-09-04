package job_test

// These tests are the create action: where the job's ask comes from, and how
// its whole task list is written in one call.

import (
	"context"
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
