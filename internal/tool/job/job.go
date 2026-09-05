package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The three things one call can ask for.
const (
	// ActionCreate starts a job, with or without a schedule.
	ActionCreate = "create"
	// ActionAddTask puts one more task on a job's list.
	ActionAddTask = "add_task"
	// ActionList lists every job.
	ActionList = "list"
)

// MaxListed is how many jobs one listing shows, newest first.
const MaxListed = 25

// MaxTasksOnCreate is how many tasks one create can write, counting the one
// under text with the ones listed under tasks, the same number as a listing
// shows. The rest go on with add_task.
const MaxTasksOnCreate = MaxListed

// Settings is what the job tool needs to do its work.
type Settings struct {
	// Jobs is the store of jobs and their task lists.
	Jobs contract.Job
	// Records is the record of the task running now, whose ask the job takes
	// word for word and whose ask says whether the task is a job's. This tool
	// only reads it, so the contract's one-method Records is all it asks for.
	// When it is nil the ask the model wrote is used instead.
	Records contract.Records
}

// writtenTask is one task as the model lists it on create, in either of the two
// shapes a model writes: the task's text as a bare string, or an object with
// text and due_at.
type writtenTask struct {
	// Text says what the task does, with one clear done line behind it.
	Text string
	// DueAt is when the task may start, written as a date and time, or empty.
	DueAt string
}

// taskObject is the object shape of a listed task. It is read apart from the
// item's own reader so that reading it does not call that reader again.
type taskObject struct {
	// Text says what the task does, with one clear done line behind it.
	Text string `json:"text"`
	// DueAt is when the task may start, written as a date and time, or empty.
	DueAt string `json:"due_at"`
}

// errNotATask is what a listed item that is neither a string nor an object
// comes back as. The list's reader puts the item's position in front of it, so
// the model reads both where the item is and what it is not.
var errNotATask = errors.New("this task is neither a string nor an object, so write the task's text as a string or an object with text")

// UnmarshalJSON reads one listed task: a string first, which is an undated task
// with that text, and then an object with text and due_at. Anything else, such
// as a number, a null, or a list, is refused, so that a small model's habit of
// listing its tasks as strings works and every other shape is named plainly.
func (task *writtenTask) UnmarshalJSON(written []byte) error {
	trimmed := bytes.TrimSpace(written)
	if len(trimmed) == 0 {
		return errNotATask
	}
	switch trimmed[0] {
	case '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return errNotATask
		}
		*task = writtenTask{Text: text}
		return nil
	case '{':
		var object taskObject
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return errNotATask
		}
		*task = writtenTask(object)
		return nil
	default:
		return errNotATask
	}
}

// writtenTasks is the task list as the model writes it. It reads itself one
// item at a time, so that an item the tool cannot read is refused by its
// position rather than by Go's own complaint about the whole call, and it
// holds the list to the cap before reading any item, so the loop is bounded.
type writtenTasks []writtenTask

// UnmarshalJSON reads the list, refusing one over the cap, an item that is not
// a task, and an item that says nothing, the last two by their position.
func (tasks *writtenTasks) UnmarshalJSON(written []byte) error {
	items := []json.RawMessage{}
	if err := json.Unmarshal(written, &items); err != nil {
		return listRefusal{line: "tasks is not a list, so write it as a list of tasks, each the task's text as a string or an object with text"}
	}
	if len(items) > MaxTasksOnCreate {
		return listRefusal{line: fmt.Sprintf("this create lists %d tasks and one create takes at most %d, so create the job with the first %d and put the rest on it with add_task",
			len(items), MaxTasksOnCreate, MaxTasksOnCreate)}
	}
	read := make(writtenTasks, 0, len(items))
	for at, item := range items {
		task := writtenTask{}
		if err := task.UnmarshalJSON(item); err != nil {
			return listRefusal{line: fmt.Sprintf("task %d of the list: %v", at+1, err)}
		}
		if strings.TrimSpace(task.Text) == "" {
			return listRefusal{line: fmt.Sprintf("task %d of the list says nothing, so write in one line what it does and how it is done", at+1)}
		}
		read = append(read, task)
	}
	*tasks = read
	return nil
}

// listRefusal is a refusal from the task list's own reader. It already says
// what went wrong and what to do, so readInput hands it back as it is rather
// than as a call that is not JSON.
type listRefusal struct {
	line string
}

// Error is the refusal in one line.
func (refusal listRefusal) Error() string { return refusal.line }

// writtenSchedule is a schedule as the model writes it, before it is turned into
// the one the contract carries.
type writtenSchedule struct {
	// Kind is at, every, or cron.
	Kind string `json:"kind"`
	// At is the one moment, written as a date and time.
	At string `json:"at"`
	// Every is the interval, written as a length of time such as 24h.
	Every string `json:"every"`
	// Cron is the expression, such as "0 7 * * 1-5".
	Cron string `json:"cron"`
	// Timezone is the place the schedule is read in, such as America/New_York.
	Timezone string `json:"timezone"`
}

// input is what the model writes when it calls this tool.
type input struct {
	// Action is create, add_task, or list.
	Action string `json:"action"`
	// Ask is the user's message, word for word, when the action is create.
	Ask string `json:"ask"`
	// Name is a short name for the job, a few words, when the action is create.
	Name string `json:"name"`
	// Why is the one line on why the user wants it.
	Why string `json:"why"`
	// Schedule is when the job makes its next task, or nothing.
	Schedule *writtenSchedule `json:"schedule"`
	// TaskTemplate is what a scheduled job turns into one task per tick.
	TaskTemplate string `json:"task_template"`
	// JobID says which job a task is added to.
	JobID string `json:"job_id"`
	// Text says what the task does, with one clear done line behind it.
	Text string `json:"text"`
	// Tasks is the task list on create, in order. Text and Tasks are one list,
	// Text first, and a Text that repeats the first listed task is that task
	// written twice, so it counts once.
	Tasks writtenTasks `json:"tasks"`
	// DueAt is when the task may start, written as a date and time.
	DueAt string `json:"due_at"`
}

// textIsAnotherTask says whether text names a task the list does not begin
// with: a blank text is no task, and a text that is the first listed task word
// for word is that task written twice.
func (asked input) textIsAnotherTask() bool {
	text := strings.TrimSpace(asked.Text)
	if text == "" {
		return false
	}
	return len(asked.Tasks) == 0 || strings.TrimSpace(asked.Tasks[0].Text) != text
}

// taskCount is how many tasks a create writes, counting text with the list.
func (asked input) taskCount() int {
	count := len(asked.Tasks)
	if asked.textIsAnotherTask() {
		count++
	}
	return count
}

// Tool is the job tool.
type Tool struct {
	settings Settings
}

// New returns the job tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolJob,
		Description: "Creates a named job for work too big for one sitting, naming its first task, with or without a schedule, adds a task to one, or lists them. " +
			"Use it only when the work needs more than one sitting.",
		Fields: []contract.ToolField{
			{Name: "action", Type: "string", Description: "One of create, add_task, or list.", Required: true},
			{Name: "ask", Type: "string", Description: "Leave it empty. The job takes the user's ask from the task record word for word, so there is no need to retype it."},
			{Name: "name", Type: "string", Description: "A short name for the job, a few words, such as \"Tater Tots Tetris\", shown in the job list and side panel."},
			{Name: "why", Type: "string", Description: "The one line on why the user wants it."},
			{Name: "schedule", Type: "object", Description: "When the job makes its next task: kind at, every, or cron."},
			{Name: "task_template", Type: "string", Description: "What a scheduled job turns into one task each time."},
			{Name: "job_id", Type: "string", Description: "Which job to add a task to."},
			{Name: "text", Type: "string", Description: "What the task does, with one clear done line behind it. On create it is the job's first task, given once: here or as the first item of tasks, not both."},
			{Name: "tasks", Type: "array", Description: "On create, the task list in order, each item either the task's text as a string or an object with text and, when it must wait for a date, due_at. Give the first task once, here or under text; text and tasks are one list of at most twenty-five."},
			{Name: "due_at", Type: "string", Description: "When the task may start, as a date and time."},
		},
		Classes: []contract.PermissionClass{contract.ClassIrreversible},
	}
}

// Run does what the call asks for, through the job store behind the contract.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Jobs == nil {
		return contract.ToolOutput{}, errors.New("this tool has no job store behind it, so wire the jobs in before using it")
	}
	switch asked.Action {
	case ActionCreate:
		return tool.create(ctx, asked)
	case ActionAddTask:
		return tool.addTask(ctx, asked)
	default:
		return tool.list(ctx)
	}
}

// create starts a job, writes its task list under it, and says which job it is
// and which tasks it added.
func (tool *Tool) create(ctx context.Context, asked input) (contract.ToolOutput, error) {
	ask, err := tool.askFor(asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	// A job's task does not make a job: a job's plan is its task list, so work
	// found inside one of its tasks goes on that list.
	jobID, inside, err := tool.jobOfTheRunningTask(ctx)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if inside {
		return contract.ToolOutput{}, fmt.Errorf("this task belongs to job %s; add tasks to that job with add_task instead of making a job inside it", jobID)
	}
	schedule, err := readSchedule(asked.Schedule)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	// Every task is read before anything is made, so that a date nobody can
	// read refuses the whole call and never leaves a job with half its list.
	tasks, err := tasksOf(asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	id, err := tool.settings.Jobs.Create(ctx, contract.NewJob{
		Ask:          ask,
		Name:         asked.Name,
		Why:          asked.Why,
		Schedule:     schedule,
		TaskTemplate: asked.TaskTemplate,
	})
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot create the job: %w", err)
	}
	// A job with no first task never carries its work, so record every task the
	// model listed, in order, and leave the first as the task to run next.
	// Adding each one through the store's own AddTask puts the first at the
	// front of an empty list, which is where the loop looks for the next task
	// to run. A scheduled job makes its own tasks from its template, so it may
	// list none.
	added := make([]string, 0, len(tasks))
	for _, task := range tasks {
		task.JobID = id
		taskID, err := tool.settings.Jobs.AddTask(ctx, task)
		if err != nil {
			return contract.ToolOutput{}, tool.switchOffTheHalfMadeJob(ctx, id, len(added)+1, len(tasks), err)
		}
		added = append(added, taskID)
	}
	return contract.ToolOutput{Text: createdLine(id, added)}, nil
}

// switchOffTheHalfMadeJob is the refusal for a create whose job was made but
// whose task list the store refused partway. The job is switched off first, so
// that a model which tries the same create again does not leave an empty
// running job behind each time: on the real store one such retry loop left
// twenty-nine empty jobs. The store's own reason goes back in full, because a
// retry only helps once that reason is fixed.
func (tool *Tool) switchOffTheHalfMadeJob(ctx context.Context, id string, refused int, wanted int, cause error) error {
	if err := tool.settings.Jobs.SwitchOff(ctx, id); err != nil {
		return fmt.Errorf("job %s was made but the store refused task %d of its %d and then refused to switch the job off (%v), so tell the user that job %s may be running with half its list rather than trying the same create again: %w",
			id, refused, wanted, err, id, cause)
	}
	return fmt.Errorf("job %s was made and then switched off, because the store refused task %d of its %d and a job with half its list must not run, so fix what the store refused or tell the user rather than trying the same create again: %w",
		id, refused, wanted, cause)
}

// jobOfTheRunningTask is the job whose task is running now, when the running
// task is a job's. The record carries no mark of its job, so the one signal in
// this tool's reach is the ask itself: the loop starts a job's task with the
// task's own text as its ask, so a record whose ask is, word for word, an
// unfinished task of a running job is that job's task. A finished task never
// runs again, so its words are a person's ask. It reads at most MaxListed jobs,
// and a store that cannot be read refuses the create rather than letting a job
// be made inside another.
func (tool *Tool) jobOfTheRunningTask(ctx context.Context) (string, bool, error) {
	if tool.settings.Records == nil {
		return "", false, nil
	}
	ask := strings.TrimSpace(tool.settings.Records.Record().Goal.Ask)
	if ask == "" {
		return "", false, nil
	}
	summaries, err := tool.settings.Jobs.List(ctx)
	if err != nil {
		return "", false, fmt.Errorf("cannot list the jobs to tell whether this task belongs to one, so try again once the job store answers: %w", err)
	}
	for at, summary := range summaries {
		if at >= MaxListed {
			break
		}
		if summary.State != contract.JobRunning {
			continue
		}
		held, err := tool.settings.Jobs.Load(ctx, summary.ID)
		if err != nil {
			return "", false, fmt.Errorf("cannot read job %s to tell whether this task belongs to it, so try again once the job store answers: %w", summary.ID, err)
		}
		for _, task := range held.Work.Tasks {
			if !task.Done && strings.TrimSpace(task.Text) == ask {
				return summary.ID, true, nil
			}
		}
	}
	return "", false, nil
}

// askFor is the ask the new job carries: the running task's, byte for byte,
// when the tool has that record, because the ask is the user's words and never
// the model's, so an ask the model wrote beside it is dropped. A tool with no
// record, or a record with no ask yet, takes the one the model wrote. When
// there is no ask anywhere the fault is the wiring's or the harness's, not the
// model's, because the ask field tells the model to leave it empty, so the
// refusal says so rather than sending the model to retype the user's words.
func (tool *Tool) askFor(asked input) (string, error) {
	if tool.settings.Records == nil {
		if strings.TrimSpace(asked.Ask) == "" {
			return "", errors.New("this job carries no ask because the tool was built without the task's record, which is a fault in the wiring and not in this call, so wire the record in before making a job")
		}
		return asked.Ask, nil
	}
	if held := tool.settings.Records.Record().Goal.Ask; strings.TrimSpace(held) != "" {
		return held, nil
	}
	if strings.TrimSpace(asked.Ask) == "" {
		return "", errors.New("this job carries no ask because the task's record has none yet, which is a fault in the harness and not in this call, so the record must carry the user's message before a job is made")
	}
	return asked.Ask, nil
}

// tasksOf is the task list a create writes, in order: the text first, when it
// names a task the list does not begin with, and then every task listed under
// tasks with its date read. A text that repeats the first listed task is that
// task written twice, so the listed one stands alone and keeps its date.
func tasksOf(asked input) ([]contract.NewTask, error) {
	tasks := make([]contract.NewTask, 0, len(asked.Tasks)+1)
	if asked.textIsAnotherTask() {
		tasks = append(tasks, contract.NewTask{Text: asked.Text})
	}
	for at, written := range asked.Tasks {
		due, err := readMoment(written.DueAt, fmt.Sprintf("due_at on task %d of the list", at+1))
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, contract.NewTask{Text: written.Text, DueAt: due})
	}
	return tasks, nil
}

// createdLine says what create did: the job, the task it started, and the
// tasks that follow.
func createdLine(id string, added []string) string {
	switch len(added) {
	case 0:
		return fmt.Sprintf("created job %s\n", id)
	case 1:
		return fmt.Sprintf("created job %s and started task %s\n", id, added[0])
	default:
		return fmt.Sprintf("created job %s and started task %s, with %s to follow\n", id, added[0], strings.Join(added[1:], ", "))
	}
}

// addTask puts one more task on a job's list and says what it is called.
func (tool *Tool) addTask(ctx context.Context, asked input) (contract.ToolOutput, error) {
	due, err := readMoment(asked.DueAt, "due_at")
	if err != nil {
		return contract.ToolOutput{}, err
	}
	id, err := tool.settings.Jobs.AddTask(ctx, contract.NewTask{JobID: asked.JobID, Text: asked.Text, DueAt: due})
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot add a task to job %s: %w", asked.JobID, err)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("added %s to job %s\n", id, asked.JobID)}, nil
}

// list writes out every job, newest first.
func (tool *Tool) list(ctx context.Context) (contract.ToolOutput, error) {
	found, err := tool.settings.Jobs.List(ctx)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot list the jobs: %w", err)
	}
	if len(found) == 0 {
		return contract.ToolOutput{Text: "there are no jobs\n"}, nil
	}
	written := &strings.Builder{}
	for at, summary := range found {
		if at >= MaxListed {
			fmt.Fprintf(written, "... %d more jobs\n", len(found)-MaxListed)
			break
		}
		fmt.Fprintf(written, "job %s %s: %d of %d tasks done, next %s\n",
			summary.ID, summary.State, summary.TasksDone, summary.TasksTotal, nextOf(summary))
	}
	return contract.ToolOutput{Text: written.String()}, nil
}

// nextOf is the task a job runs next, in a few words.
func nextOf(summary contract.JobSummary) string {
	if summary.NextTaskID == "" {
		return "nothing"
	}
	if summary.NextDue.IsZero() {
		return summary.NextTaskID
	}
	return summary.NextTaskID + " at " + summary.NextDue.Format(time.RFC3339)
}
