package job_test

// These tests are the rule that text and tasks on a create are one list: text
// first, counted once when it repeats the first listed task, with the cap of
// twenty-five counting text.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/job"
)

// TestTextAndTheFirstListedTaskAreOneTaskWhenTheyMatch proves that text and
// tasks are one list: a model that names its first task under text and again
// as the first item of tasks has written it once, so the job holds exactly the
// two tasks in order and not three.
func TestTextAndTheFirstListedTaskAreOneTaskWhenTheyMatch(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")

	if _, err := run(t, tool, map[string]any{
		"action": "create", "why": "the why",
		"text":  "write tests",
		"tasks": []any{"write tests", "build it"},
	}); err != nil {
		t.Fatalf("creating a job whose text repeats the first listed task was refused: %v", err)
	}
	tasks := jobs.Tasks("1")
	if len(tasks) != 2 {
		t.Fatalf("the job holds %d tasks, want exactly two: text and the first listed task are the same task", len(tasks))
	}
	if tasks[0].Text != "write tests" || tasks[1].Text != "build it" {
		t.Errorf("the job holds %q then %q, want \"write tests\" then \"build it\"", tasks[0].Text, tasks[1].Text)
	}
}

// TestTheCapOnCreateCountsText proves the twenty-five cap counts text: text
// with twenty-five other tasks is twenty-six and is refused before any job is
// made, text with twenty-four is taken, and text repeating the first of
// twenty-five listed tasks is twenty-five and is taken.
func TestTheCapOnCreateCountsText(t *testing.T) {
	tool, jobs := newToolOverRecord(t, "the ask")
	listed := make([]any, 0, job.MaxTasksOnCreate)
	for at := range job.MaxTasksOnCreate {
		listed = append(listed, fmt.Sprintf("task %d", at+1))
	}

	_, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "the first task", "tasks": listed})
	if err == nil {
		t.Fatalf("text and %d listed tasks were taken, and together they are %d", len(listed), len(listed)+1)
	}
	for _, told := range []string{fmt.Sprint(job.MaxTasksOnCreate + 1), "add_task"} {
		if !strings.Contains(err.Error(), told) {
			t.Errorf("the refusal reads %q and does not say %q", err, told)
		}
	}
	if summaries, listErr := jobs.List(context.Background()); listErr != nil {
		t.Fatalf("listing failed: %v", listErr)
	} else if len(summaries) != 0 {
		t.Errorf("the refused create still left %d jobs behind", len(summaries))
	}

	if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "the first task", "tasks": listed[:job.MaxTasksOnCreate-1]}); err != nil {
		t.Errorf("text and %d listed tasks were refused, and together they are exactly %d: %v", job.MaxTasksOnCreate-1, job.MaxTasksOnCreate, err)
	}
	if tasks := jobs.Tasks("1"); len(tasks) != job.MaxTasksOnCreate {
		t.Errorf("the job holds %d tasks, want %d", len(tasks), job.MaxTasksOnCreate)
	}

	if _, err := run(t, tool, map[string]any{"action": "create", "why": "the why", "text": "task 1", "tasks": listed}); err != nil {
		t.Errorf("text repeating the first of %d listed tasks was refused, and that is %d tasks: %v", job.MaxTasksOnCreate, job.MaxTasksOnCreate, err)
	}
	if tasks := jobs.Tasks("2"); len(tasks) != job.MaxTasksOnCreate {
		t.Errorf("the job holds %d tasks, want %d", len(tasks), job.MaxTasksOnCreate)
	}
}

// TestTheTextAndTasksFieldsTellTheModelToGiveTheFirstTaskOnce proves the two
// field descriptions each name the other and say the first task is given once,
// so the model is told before it writes it twice.
func TestTheTextAndTasksFieldsTellTheModelToGiveTheFirstTaskOnce(t *testing.T) {
	tool, _ := newTool(t)
	described := map[string]string{}
	for _, field := range tool.Spec().Fields {
		described[field.Name] = field.Description
	}
	if !strings.Contains(described["text"], "tasks") || !strings.Contains(described["text"], "once") {
		t.Errorf("the text field says %q and does not tell the model to give the first task once, here or under tasks", described["text"])
	}
	if !strings.Contains(described["tasks"], "text") || !strings.Contains(described["tasks"], "once") {
		t.Errorf("the tasks field says %q and does not tell the model to give the first task once, here or under text", described["tasks"])
	}
}
