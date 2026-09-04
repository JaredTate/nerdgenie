package job

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"encoding/json"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return input{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an action in it: %w", err)
		}
	}
	switch asked.Action {
	case ActionCreate:
		// The ask is not checked here, because which ask the job takes depends
		// on whether the tool has the running task's record, and create knows.
		if asked.Schedule == nil && strings.TrimSpace(asked.Text) == "" && len(asked.Tasks) == 0 {
			return input{}, errors.New("this job has no first task, so a job needs at least one task: create it with its task list under tasks, then work the first task")
		}
		if len(asked.Tasks) > MaxTasksOnCreate {
			return input{}, fmt.Errorf("this create lists %d tasks and one create takes at most %d, so create the job with the first %d and put the rest on it with add_task",
				len(asked.Tasks), MaxTasksOnCreate, MaxTasksOnCreate)
		}
		for at, task := range asked.Tasks {
			if strings.TrimSpace(task.Text) == "" {
				return input{}, fmt.Errorf("task %d of the list says nothing, so write in one line what it does and how it is done", at+1)
			}
		}
	case ActionAddTask:
		if strings.TrimSpace(asked.JobID) == "" {
			return input{}, errors.New("this call names no job, so say which job the task belongs to")
		}
		if strings.TrimSpace(asked.Text) == "" {
			return input{}, errors.New("this task says nothing, so write in one line what it does and how it is done")
		}
	case ActionList:
		return asked, nil
	default:
		return input{}, fmt.Errorf("the action %q is not one this tool knows, so use create, add_task, or list", asked.Action)
	}
	return asked, nil
}

// readSchedule turns the schedule the model wrote into the one the contract
// carries, and refuses one that says nothing a clock could act on.
func readSchedule(written *writtenSchedule) (*contract.Schedule, error) {
	if written == nil {
		return nil, nil
	}
	schedule := &contract.Schedule{Kind: contract.ScheduleKind(written.Kind), Timezone: written.Timezone}
	switch schedule.Kind {
	case contract.ScheduleAt:
		moment, err := readMoment(written.At, "at")
		if err != nil {
			return nil, err
		}
		if moment.IsZero() {
			return nil, errors.New("a schedule of the kind at needs the moment to run, so write it as a date and a time")
		}
		schedule.At = moment
	case contract.ScheduleEvery:
		gap, err := time.ParseDuration(strings.TrimSpace(written.Every))
		if err != nil || gap <= 0 {
			return nil, fmt.Errorf("cannot read %q as a length of time, so write one such as 24h or 30m", written.Every)
		}
		schedule.Every = gap
	case contract.ScheduleCron:
		if strings.TrimSpace(written.Cron) == "" {
			return nil, errors.New("a schedule of the kind cron needs the expression, so write one such as 0 7 * * 1-5")
		}
		schedule.Cron = written.Cron
	default:
		return nil, fmt.Errorf("the schedule kind %q is not one this tool knows, so use at, every, or cron", written.Kind)
	}
	if err := checkTimezone(schedule.Timezone); err != nil {
		return nil, err
	}
	return schedule, nil
}

// checkTimezone refuses a place no clock on this machine knows, so that a
// schedule is never read in a timezone that does not exist.
func checkTimezone(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("the timezone %q is not one this machine knows, so use one such as America/New_York: %w", name, err)
	}
	return nil
}

// readMoment reads a date and a time as the model wrote it, and returns the zero
// time when the model wrote none.
func readMoment(written string, field string) (time.Time, error) {
	trimmed := strings.TrimSpace(written)
	if trimmed == "" {
		return time.Time{}, nil
	}
	for _, shape := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
		if moment, err := time.Parse(shape, trimmed); err == nil {
			return moment, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read %s as a date and a time, so write %q as something like 2026-03-01T14:00:00Z",
		field, trimmed)
}
