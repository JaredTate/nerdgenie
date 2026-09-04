package job

import (
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

// MaxTasksOnCreate is how many tasks one create can list under tasks, the same
// number as a listing shows. The rest go on with add_task.
const MaxTasksOnCreate = MaxListed

// Records is the record of the task running now, as this tool reads it, which
// is the keeper in internal/record. Only the one method here is used, because
// this tool takes the ask from the record and never writes to it.
type Records interface {
	// Record returns a copy of the record as it stands.
	Record() contract.Record
}

// Settings is what the job tool needs to do its work.
type Settings struct {
	// Jobs is the store of jobs and their task lists.
	Jobs contract.Job
	// Records is the record of the task running now, whose ask the job takes
	// word for word. When it is nil the ask the model wrote is used instead.
	Records Records
}

// writtenTask is one task as the model lists it on create.
type writtenTask struct {
	// Text says what the task does, with one clear done line behind it.
	Text string `json:"text"`
	// DueAt is when the task may start, written as a date and time, or empty.
	DueAt string `json:"due_at"`
}

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
	// Tasks is the whole task list on create, in order, after Text when there
	// is one.
	Tasks []writtenTask `json:"tasks"`
	// DueAt is when the task may start, written as a date and time.
	DueAt string `json:"due_at"`
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
			{Name: "text", Type: "string", Description: "What the task does, with one clear done line behind it. On create it is the job's first task."},
			{Name: "tasks", Type: "array", Description: "On create, the whole task list in order, each item an object with text and, when it must wait for a date, due_at. At most twenty-five. A job without a schedule needs at least one task, here or under text."},
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
			return contract.ToolOutput{}, fmt.Errorf("created job %s and recorded %d of its tasks but cannot record the next: %w", id, len(added), err)
		}
		added = append(added, taskID)
	}
	return contract.ToolOutput{Text: createdLine(id, added)}, nil
}

// askFor is the ask the new job carries: the running task's, byte for byte,
// when the tool has that record, because the ask is the user's words and never
// the model's, so an ask the model wrote beside it is dropped. A tool with no
// record, or a record with no ask yet, takes the one the model wrote.
func (tool *Tool) askFor(asked input) (string, error) {
	ask := asked.Ask
	if tool.settings.Records != nil {
		if held := tool.settings.Records.Record().Goal.Ask; strings.TrimSpace(held) != "" {
			ask = held
		}
	}
	if strings.TrimSpace(ask) == "" {
		return "", errors.New("this job carries no ask, so pass the user's message word for word")
	}
	return ask, nil
}

// tasksOf is the task list a create writes, in order: the text first, when
// there is one, and then every task listed under tasks with its date read.
func tasksOf(asked input) ([]contract.NewTask, error) {
	tasks := make([]contract.NewTask, 0, len(asked.Tasks)+1)
	if strings.TrimSpace(asked.Text) != "" {
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
