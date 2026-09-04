package job_test

import (
	"context"
	"encoding/json"
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
