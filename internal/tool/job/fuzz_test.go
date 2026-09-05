package job

// This file is in the package itself rather than beside the other tests in
// job_test, because readInput is the tool's own reader and is not exported.

import (
	"strings"
	"testing"
)

// FuzzReadInput holds the rules for whatever the model writes as a call: the
// reader never panics, a call it takes names one of the three actions, and a
// create it takes writes at most the cap of tasks counting text, with no task
// that says nothing.
func FuzzReadInput(f *testing.F) {
	for _, seed := range []string{
		`{"action":"create","ask":"the ask","why":"the why","text":"the first task"}`,
		`{"action":"create","why":"the why","tasks":["write the failing tests","implement the engine"]}`,
		`{"action":"create","why":"the why","text":"the first task","tasks":[{"text":"the second task","due_at":"2026-03-02T14:00:00Z"},"the third task"]}`,
		`{"action":"create","why":"the why","text":"write tests","tasks":["write tests","build it"]}`,
		`{"action":"create","text":"the first task","tasks":[` + strings.Repeat(`"a task",`, MaxTasksOnCreate-1) + `"the last one"]}`,
		`{"action":"add_task","job_id":"1","text":"the task","due_at":"2026-03-01 14:00"}`,
		`{"action":"add_task","job_id":"1","tasks":["the task listed alone"]}`,
		`{"action":"add_task","job_id":"1","tasks":["one task","two tasks"]}`,
		`{"action":"list"}`,
		`{"action":"create","ask":"check the site every morning","schedule":{"kind":"every","every":"24h"},"task_template":"check the site and report"}`,
		`not json`,
		`{"action":"dance"}`,
		`{"action":"create","tasks":[` + strings.Repeat(`"a task",`, MaxTasksOnCreate) + `"one too many"]}`,
		`{"action":"create","tasks":["the first task",7]}`,
		`{"action":"create","tasks":["the first task",null]}`,
		`{"action":"create","tasks":"do it"}`,
		`{"action":"create","tasks":[{"text":"   "}]}`,
		`{"action":"list","tasks":[{"text":" "}]}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, written []byte) {
		asked, err := readInput(written)
		if err != nil {
			return
		}
		switch asked.Action {
		case ActionCreate, ActionAddTask, ActionList:
		default:
			t.Fatalf("the reader took the action %q, and it knows three", asked.Action)
		}
		if len(asked.Tasks) > MaxTasksOnCreate {
			t.Fatalf("the reader took %d tasks, and the cap is %d", len(asked.Tasks), MaxTasksOnCreate)
		}
		if asked.Action == ActionCreate && asked.taskCount() > MaxTasksOnCreate {
			t.Fatalf("the reader took a create of %d tasks counting text, and the cap is %d", asked.taskCount(), MaxTasksOnCreate)
		}
		for at, task := range asked.Tasks {
			if strings.TrimSpace(task.Text) == "" {
				t.Fatalf("the reader took task %d of the list with nothing in it", at+1)
			}
		}
	})
}
