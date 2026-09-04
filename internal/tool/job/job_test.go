package job_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/job"
)

// theMoment is the time the fake clock reads in every test here.
var theMoment = time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC)

// newTool builds the job tool over the fake job store.
func newTool(t *testing.T) (*job.Tool, *testkit.FakeJob) {
	t.Helper()
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theMoment))
	return job.New(job.Settings{Jobs: jobs}), jobs
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *job.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolJob {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolJob)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "action,ask,name,why,schedule,task_template,job_id,text,tasks,due_at" {
		t.Errorf("the tool takes the fields %v", names)
	}
	if len(spec.Classes) != 1 || spec.Classes[0] != contract.ClassIrreversible {
		t.Errorf("the tool claims the classes %v, and making a job cannot be undone", spec.Classes)
	}
}

func TestAJobWithNoScheduleIsCreatedAndItsTasksAreAddedAndListed(t *testing.T) {
	tool, jobs := newTool(t)

	created, err := run(t, tool, map[string]any{
		"action": "create",
		"ask":    "write up the release notes for every version this year",
		"why":    "the user wants one page per release",
		"text":   "gather the versions, done when the list is written",
	})
	if err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}
	if !strings.Contains(created.Text, "1") {
		t.Errorf("creating a job said %q and did not name the job", created.Text)
	}

	if _, err := run(t, tool, map[string]any{
		"action": "add_task", "job_id": "1", "text": "write up version one, done when the page is saved",
	}); err != nil {
		t.Fatalf("adding a task failed: %v", err)
	}
	if tasks := jobs.Tasks("1"); len(tasks) != 2 {
		t.Fatalf("the job holds %d tasks, want the first task and the one that was added", len(tasks))
	}

	listed, err := run(t, tool, map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	testkit.Golden(t, "the_jobs.txt", []byte(listed.Text))
}

func TestACreateWithNoFirstTaskIsRefusedAndNamesTask(t *testing.T) {
	tool, jobs := newTool(t)

	_, err := run(t, tool, map[string]any{
		"action": "create",
		"ask":    "build a whole tetris game i can play in the terminal",
		"why":    "the user wants to play tetris",
	})
	if err == nil {
		t.Fatalf("a create with no first task was allowed to make an empty job")
	}
	if !strings.Contains(err.Error(), "task") {
		t.Errorf("the refusal reads %q and does not tell the model to name a task", err)
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused create still left %d jobs behind", len(summaries))
	}
}

func TestACreateWithAFirstTaskStartsItUnderTheJob(t *testing.T) {
	tool, jobs := newTool(t)

	out, err := run(t, tool, map[string]any{
		"action": "create",
		"ask":    "build a whole tetris game i can play in the terminal",
		"why":    "the user wants to play tetris",
		"text":   "set up the game board, done when the empty grid renders",
	})
	if err != nil {
		t.Fatalf("creating a job with a first task failed: %v", err)
	}

	tasks := jobs.Tasks("1")
	if len(tasks) != 1 {
		t.Fatalf("the new job holds %d tasks, want the one first task", len(tasks))
	}
	if tasks[0].Done {
		t.Errorf("the first task is already marked done, want it left to work")
	}
	if !strings.Contains(out.Text, tasks[0].TaskID) {
		t.Errorf("creating a job said %q and did not name the task it started", out.Text)
	}

	summaries, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(summaries) != 1 || summaries[0].NextTaskID != tasks[0].TaskID {
		t.Errorf("the job's next task is %q, want the first task %q", summaries[0].NextTaskID, tasks[0].TaskID)
	}
}

func TestAJobWithAScheduleKeepsIt(t *testing.T) {
	tool, _ := newTool(t)

	for _, written := range []map[string]any{
		{"kind": "at", "at": theMoment.Add(time.Hour).Format(time.RFC3339)},
		{"kind": "every", "every": "24h"},
		{"kind": "cron", "cron": "0 7 * * 1-5", "timezone": "America/New_York"},
	} {
		output, err := run(t, tool, map[string]any{
			"action": "create", "ask": "check the site every morning", "why": "the user wants to know it is up",
			"schedule": written, "task_template": "check the site and report",
		})
		if err != nil {
			t.Fatalf("creating a job with the schedule %v failed: %v", written, err)
		}
		if output.Text == "" {
			t.Errorf("creating a scheduled job said nothing")
		}
	}
}

func TestATaskWithADateWaitsForIt(t *testing.T) {
	tool, jobs := newTool(t)
	if _, err := run(t, tool, map[string]any{
		"action": "create", "ask": "the ask", "why": "the why", "text": "the first task",
	}); err != nil {
		t.Fatalf("creating a job failed: %v", err)
	}

	due := theMoment.Add(48 * time.Hour)
	if _, err := run(t, tool, map[string]any{
		"action": "add_task", "job_id": "1", "text": "the task", "due_at": due.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("adding a task with a date failed: %v", err)
	}
	tasks := jobs.Tasks("1")
	if len(tasks) != 2 {
		t.Fatalf("the job holds %d tasks, want the first task and the dated one", len(tasks))
	}
	if tasks[1].DueAt == "" {
		t.Errorf("the dated task on the list is %v, want one that waits for its date", tasks[1])
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"action": "dance"},
		{"action": "create"},
		{"action": "create", "ask": "the ask", "schedule": map[string]any{"kind": "sometimes"}},
		{"action": "create", "ask": "the ask", "schedule": map[string]any{"kind": "every", "every": "soon"}},
		{"action": "create", "ask": "the ask", "schedule": map[string]any{"kind": "at", "at": "tomorrow"}},
		{"action": "add_task", "text": "the task"},
		{"action": "add_task", "job_id": "1"},
		{"action": "add_task", "job_id": "99", "text": "the task"},
		{"action": "add_task", "job_id": "1", "text": "the task", "due_at": "soon"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoJobsWiredInSaysSo(t *testing.T) {
	tool := job.New(job.Settings{})

	_, err := run(t, tool, map[string]any{"action": "list"})
	if err == nil {
		t.Fatalf("the jobs were listed with no job store behind the tool")
	}
	if !strings.Contains(err.Error(), "job") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

func TestListingWithNoJobsAtAllSaysSo(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("listing an empty set of jobs failed: %v", err)
	}
	if !strings.Contains(output.Text, "no jobs") {
		t.Errorf("listing an empty set of jobs said %q", output.Text)
	}
}

// TestAJobIsCreatedWithTheNameTheModelGivesIt proves the short name the model
// passes on create reaches the store and becomes the job's listed title, which
// is what the side panel and "/jobs" show in place of the whole ask.
func TestAJobIsCreatedWithTheNameTheModelGivesIt(t *testing.T) {
	tool, jobs := newTool(t)
	if _, err := run(t, tool, map[string]any{
		"action": "create",
		"ask":    "Build a complete, polished, playable Tetris-style web game with dragons and yetis.",
		"name":   "Tater Tots Tetris",
		"why":    "the user wants the whole game built and tested",
		"text":   "write the failing tests for the core engine",
	}); err != nil {
		t.Fatalf("creating a named job was refused: %v", err)
	}
	listed, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("cannot list the jobs: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("the store holds %d jobs, want one", len(listed))
	}
	if listed[0].Title != "Tater Tots Tetris" {
		t.Errorf("the job lists by the title %q, want the name the model gave it", listed[0].Title)
	}
}

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
