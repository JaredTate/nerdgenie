package job

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// claimedTask is one claim the table held: which job's task, and which process
// took it.
type claimedTask struct {
	jobID  string
	taskID string
	owner  string
}

// key is the id the loop writes on the ask of a job's task, the job and the
// task joined by a dot, which is how a run in the log is matched to a claim.
func (claim claimedTask) key() string {
	return claim.jobID + "." + claim.taskID
}

// runOfATask is the newest run the loop made of one job's task: the number
// it ran under, and whether that run got as far as a tool call and so left a
// record to pick up.
type runOfATask struct {
	number    string
	hasRecord bool
}

// putDownWhatADeadProcessLeft turns a dead process's claim into the mark the
// person's stop would have written, for a task somebody was attending whose
// newest run left a record: the job is paused on that task with the run's
// number, so that "continue" picks the run up under the job from its record,
// and the driver does not run the same task again from the top beside it,
// which is what a restart in the middle of a task used to do. A run that left
// no record has nothing to pick up, so its task is simply run again; a job
// that is not running is left where the person put it; and a schedule's task
// has nobody to say continue to it, so pausing the job on it would stop the
// schedule for good, and it too is run again.
func (jobs *Jobs) putDownWhatADeadProcessLeft(ctx context.Context, left []claimedTask) error {
	if len(left) == 0 {
		return nil
	}
	runs, err := jobs.newestRunsOf(ctx, left)
	if err != nil {
		return err
	}
	for _, claim := range left {
		held, there := jobs.held[claim.jobID]
		if !there || held.state.State != contract.JobRunning || held.state.facts(claim.taskID).Unattended {
			continue
		}
		task, listed := taskOnTheList(held, claim.taskID)
		run, ran := runs[claim.key()]
		if !listed || task.Done || !ran || !run.hasRecord {
			continue
		}
		mark := contract.PutDownMark{
			Task:      contract.TaskToRun{JobID: claim.jobID, TaskID: claim.taskID, Text: task.Text},
			Run:       run.number,
			HasRecord: true,
		}
		if err := jobs.putDown(ctx, claim.jobID, held, mark); err != nil {
			return fmt.Errorf("cannot put job %s down on task %s, which the process before this one was running: %w",
				claim.jobID, claim.taskID, err)
		}
	}
	return nil
}

// newestRunsOf reads the log once for the runs the loop made of the tasks
// given: the ask of a job's task is written under the run's number with the
// job and the task as its id, and a checkpoint under that number says the run
// made a record. Only the newest run of each task is kept, because that is
// the one a person's "continue" would pick up, so the two maps never hold
// more than one entry per claim.
func (jobs *Jobs) newestRunsOf(ctx context.Context, left []claimedTask) (map[string]runOfATask, error) {
	wanted := map[string]bool{}
	for _, claim := range left {
		wanted[claim.key()] = true
	}
	runs := map[string]runOfATask{}
	taskOfRun := map[string]string{}
	err := jobs.eventLog.Replay(ctx, func(event contract.Event) error {
		if !isARunsKey(event.TaskID) {
			return nil
		}
		switch event.Kind {
		case contract.EventMessage:
			asked := contract.Inbound{}
			if err := json.Unmarshal(event.Body, &asked); err != nil || !wanted[asked.ID] {
				return nil
			}
			if newest, known := runs[asked.ID]; known && runNumberOf(newest.number) >= runNumberOf(event.TaskID) {
				return nil
			}
			delete(taskOfRun, runs[asked.ID].number)
			runs[asked.ID] = runOfATask{number: event.TaskID}
			taskOfRun[event.TaskID] = asked.ID
		case contract.EventCheckpoint:
			if key, known := taskOfRun[event.TaskID]; known {
				runs[key] = runOfATask{number: event.TaskID, hasRecord: true}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot replay the event log to find the runs the process before this one left: %w", err)
	}
	return runs, nil
}

// isARunsKey says whether a log key is a task's, which is its bare number. A
// job's key begins with a letter.
func isARunsKey(logKey string) bool {
	number, err := strconv.Atoi(logKey)
	return err == nil && number > 0
}

// taskOnTheList finds one task on a job's list by its id.
func taskOnTheList(held *heldJob, taskID string) (contract.JobTask, bool) {
	for _, task := range held.keeper.Record().Work.Tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return contract.JobTask{}, false
}
