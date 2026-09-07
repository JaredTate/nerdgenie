package job

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// Create starts a job, with or without a schedule, and returns its identifier.
// The job's record is created in the log at once, so a job exists the moment it
// is made and survives a restart before its first task has run.
func (jobs *Jobs) Create(ctx context.Context, wanted contract.NewJob) (string, error) {
	if wanted.Ask == "" {
		return "", errors.New("a job needs the user's ask before it can be created, so pass the message word for word")
	}
	// The ask rides at the top of every one of the job's tasks, so work that
	// restarts the agent is refused here as it is in a task's own text.
	if err := checkItCannotRestartTheAgent(wanted.Ask); err != nil {
		return "", err
	}
	if err := checkTheSchedule(wanted.Schedule); err != nil {
		return "", err
	}
	if err := checkTheTemplate(wanted.Schedule, wanted.TaskTemplate); err != nil {
		return "", err
	}

	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	if len(jobs.order) >= MaxJobs {
		return "", fmt.Errorf("there are already %d jobs, which is as many as one agent keeps, so finish or switch some off before making another", MaxJobs)
	}

	jobID := strconv.Itoa(jobs.nextJob)
	keeper, err := record.New(ctx, jobs.eventLog, record.Start{Kind: contract.RecordJob, ID: jobID, Ask: wanted.Ask})
	if err != nil {
		return "", fmt.Errorf("cannot create the record of job %s: %w", jobID, err)
	}
	if wanted.Name != "" || wanted.Why != "" {
		if err := keeper.Apply(ctx, record.Update{Name: wanted.Name, Why: wanted.Why}); err != nil {
			return "", fmt.Errorf("cannot write the name and why of job %s: %w", jobID, err)
		}
	}
	if err := writeTheWorkOrdersParts(ctx, keeper, jobID, wanted); err != nil {
		return "", err
	}
	if wanted.Folder != "" {
		if err := keeper.SetSituation(ctx, []string{contract.ProjectFolderLine + wanted.Folder}); err != nil {
			return "", fmt.Errorf("cannot write the project folder of job %s: %w", jobID, err)
		}
	}

	held := &heldJob{keeper: keeper}
	starting := jobState{State: contract.JobRunning, Schedule: wanted.Schedule, Template: wanted.TaskTemplate}
	if wanted.Schedule != nil {
		firstRun, err := nextRun(*wanted.Schedule, jobs.clock.Now())
		if err != nil {
			return "", err
		}
		starting.NextRun = firstRun
	}
	if err := jobs.saveState(ctx, jobID, held, starting); err != nil {
		return "", err
	}

	jobs.nextJob++
	jobs.order = append(jobs.order, jobID)
	jobs.held[jobID] = held
	jobs.wake()
	return jobID, nil
}

// AddTask puts one more task on a job's list and returns its identifier. A
// finished task is never removed, so the list only ever grows, up to the cap one
// job holds.
func (jobs *Jobs) AddTask(ctx context.Context, wanted contract.NewTask) (string, error) {
	if wanted.Text == "" {
		return "", errors.New("a task needs one line saying what it does, so pass the text of it")
	}
	if err := checkItCannotRestartTheAgent(wanted.Text); err != nil {
		return "", err
	}

	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(wanted.JobID)
	if err != nil {
		return "", err
	}
	return jobs.addTask(ctx, wanted.JobID, held, wanted.Text, wanted.DueAt, false)
}

// addTask writes one task onto a job's list and remembers when it may start and
// whether a schedule made it. The caller holds the lock.
func (jobs *Jobs) addTask(ctx context.Context, jobID string, held *heldJob, text string, dueAt time.Time, unattended bool) (string, error) {
	listed := held.keeper.Record().Work.Tasks
	if len(listed) >= MaxTasksPerJob {
		return "", fmt.Errorf("job %s already holds %d tasks, which is as many as one job holds, so start a fresh job for the rest of the work", jobID, MaxTasksPerJob)
	}

	taskID := contract.TaskID(highestTaskNumber(listed) + 1)
	written := make([]record.NewJobTask, 0, len(listed)+1)
	for _, task := range listed {
		written = append(written, record.NewJobTask{TaskID: task.TaskID, Text: task.Text, DueAt: task.DueAt})
	}
	written = append(written, record.NewJobTask{TaskID: taskID, Text: text, DueAt: dueInPlainWords(dueAt)})
	if err := held.keeper.Apply(ctx, record.Update{Tasks: written}); err != nil {
		return "", fmt.Errorf("cannot put task %s on the list of job %s: %w", taskID, jobID, err)
	}

	changed := held.state.withTask(taskID, taskFacts{DueAt: dueAt, Unattended: unattended})
	if err := jobs.saveState(ctx, jobID, held, changed); err != nil {
		return "", err
	}
	if err := jobs.writeProgress(ctx, jobID, held); err != nil {
		return "", err
	}
	jobs.wake()
	return taskID, nil
}

// highestTaskNumber is the largest number the task list has reached, which is
// what the next task counts up from. A job lists its tasks in order, so a new
// task always takes a number above every one before it.
func highestTaskNumber(listed []contract.JobTask) int {
	highest := 0
	for _, task := range listed {
		if number, valid := contract.ParseTaskID(task.TaskID); valid && number > highest {
			highest = number
		}
	}
	return highest
}

// checkTheTemplate holds the rule that a schedule needs a template to make its
// tasks from, that a job without a schedule has no use for one, and that neither
// may be work that restarts the agent.
func checkTheTemplate(schedule *contract.Schedule, template string) error {
	if schedule != nil && template == "" {
		return errors.New("a job with a schedule makes one task from its template on every tick, so pass the text of the template")
	}
	if schedule == nil && template != "" {
		return errors.New("a job with no schedule never makes a task from a template, so give it a schedule or leave the template out")
	}
	return checkItCannotRestartTheAgent(template)
}

// writeTheWorkOrdersParts puts the done lines and the rules a work order gave
// into the job's record, the done lines unproved and the rules as the record's
// rules in the person's words, so that every task of the job reads them in
// the job summary. A job made without them is left as the model will write it.
func writeTheWorkOrdersParts(ctx context.Context, keeper *record.Keeper, jobID string, wanted contract.NewJob) error {
	if len(wanted.DoneWhen) > 0 {
		if err := keeper.Apply(ctx, record.Update{DoneWhen: doneLinesOf(wanted.DoneWhen)}); err != nil {
			return fmt.Errorf("cannot write the work order's done list into job %s: %w", jobID, err)
		}
	}
	for _, rule := range wanted.Rules {
		if _, err := keeper.AddCorrection(ctx, rule); err != nil {
			return fmt.Errorf("cannot write the work order's rule %q into job %s: %w", rule, jobID, err)
		}
	}
	return nil
}
