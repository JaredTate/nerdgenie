package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// liftTheWorkOrder makes the job an ask written as a work order describes,
// before the model is called at all, and says whether it did. The person
// wrote the goal, the done lines, the rules and the tasks in the record's
// own shape, so the harness writes the job's record from them and the model
// never spends a round planning it; the job's first task then runs the way
// any job task does. An ask without the headings, or a task that already
// belongs to a job, is left alone, so a plain ask runs exactly as before.
func (theLoop *Loop) liftTheWorkOrder(ctx context.Context, task Task) (Outcome, bool, error) {
	if task.FromJob != nil || theLoop.options.Jobs == nil {
		return Outcome{}, false, nil
	}
	order := workorder.Parse(task.Message.Text)
	if !order.IsWorkOrder || len(order.Tasks) == 0 {
		return Outcome{}, false, nil
	}
	jobID, taskIDs, err := theLoop.makeTheJobOf(ctx, task.Message.Text, order)
	if err != nil {
		return Outcome{}, true, err
	}
	report := liftedLine(jobID, taskIDs)
	theLoop.noteRecordLine(RecordLineOf(jobID, nil, "made job", task.Message.Text))
	if err := theLoop.tell(ctx, task.Channel, report); err != nil {
		return Outcome{}, true, err
	}
	return Outcome{Status: contract.StatusDone, Report: report}, true, nil
}

// makeTheJobOf creates the job through the same store the job tool uses: the
// whole ask as the job's ask, the title as its name, the goal as its why, the
// done lines as written with their checks, the rules with tests first in
// front, and one task per line of the order of work, each keeping the
// details it names so that its own front can show those sections.
func (theLoop *Loop) makeTheJobOf(ctx context.Context, ask string, order workorder.WorkOrder) (string, []string, error) {
	doneWhen := make([]string, 0, len(order.DoneWhen))
	for _, line := range order.DoneWhen {
		doneWhen = append(doneWhen, line.Text)
	}
	jobID, err := theLoop.options.Jobs.Create(ctx, contract.NewJob{
		Ask:      ask,
		Name:     order.Name,
		Why:      order.Goal,
		DoneWhen: doneWhen,
		Rules:    order.RulesWithTestsFirst(),
		Folder:   order.Folder,
	})
	if err != nil {
		return "", nil, fmt.Errorf("cannot make the job the work order describes: %w", err)
	}
	taskIDs := make([]string, 0, len(order.Tasks))
	for _, task := range order.Tasks {
		taskID, err := theLoop.options.Jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: task.Line})
		if err != nil {
			return "", nil, fmt.Errorf("cannot put task %d of the work order on job %s: %w", len(taskIDs)+1, jobID, err)
		}
		taskIDs = append(taskIDs, taskID)
	}
	return jobID, taskIDs, nil
}

// liftedLine is what the person is told: which job the work order became, how
// many tasks it has, and that the first starts now.
func liftedLine(jobID string, taskIDs []string) string {
	if len(taskIDs) == 1 {
		return fmt.Sprintf("Made job %s from the work order, with 1 task, %s, which starts now.", jobID, taskIDs[0])
	}
	return fmt.Sprintf("Made job %s from the work order, with %d tasks, %s to %s. Task %s starts now.",
		jobID, len(taskIDs), taskIDs[0], taskIDs[len(taskIDs)-1], strings.TrimSpace(taskIDs[0]))
}
