package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// maxAskLettersInTheStatus is how much of a task's or a job's ask the status
// answer shows before it says the rest was cut, so that the answer stays a few
// short lines and not a wall of the user's own words read back.
const maxAskLettersInTheStatus = 80

// nothingInFlight is what a status question is answered with when nothing is
// running and no task was ever recorded.
const nothingInFlight = "Nothing in flight."

// statusAnswer answers a status question straight out of the records, with no
// model call and no new task: the running job's progress when a job's task is
// running, the current or most recent task's header line and where it stands,
// and "nothing in flight" when there is neither. It is what a person who asks
// "where are we" is told, in place of a fresh blind task that would see an empty
// record and answer "no active work".
func (running *agent) statusAnswer(ctx context.Context) string {
	lines := []string{}
	if line := running.runningJobLine(ctx); line != "" {
		lines = append(lines, line)
	}
	if held, found := running.currentOrLatestTask(ctx); found {
		lines = append(lines, taskHeaderLine(held), taskStandingLine(held))
	}
	if len(lines) == 0 {
		return nothingInFlight
	}
	return strings.Join(lines, "\n")
}

// currentOrLatestTask is the task the answer is about: the one running now when
// there is one, and otherwise the highest-numbered task the log holds. It is
// false when no task record was ever written.
func (running *agent) currentOrLatestTask(ctx context.Context) (contract.Record, bool) {
	if running.loop != nil {
		if number := running.loop.Running(); number != "" {
			if held, ok := loadTaskRecord(ctx, running.events, number); ok {
				return held, true
			}
		}
	}
	return latestTaskRecord(ctx, running.events)
}

// loadTaskRecord reloads one task's record from its latest checkpoint, and is
// false when there is no such task or its checkpoints cannot be read.
func loadTaskRecord(ctx context.Context, store contract.Store, number string) (contract.Record, bool) {
	keeper, ok := loadTaskKeeper(ctx, store, number)
	if !ok {
		return contract.Record{}, false
	}
	return keeper.Record(), true
}

// loadTaskKeeper reloads one task's keeper from its latest checkpoint, which is
// what a reader that also wants the checkpoint's number asks for, and is false
// when there is no such task or its checkpoints cannot be read.
func loadTaskKeeper(ctx context.Context, store contract.Store, number string) (*record.Keeper, bool) {
	keeper, err := record.Load(ctx, store, contract.RecordTask, number)
	if err != nil {
		return nil, false
	}
	return keeper, true
}

// latestTaskRecord is the highest-numbered task the log holds, read from its
// latest checkpoint. A job's checkpoints are passed over, because a job's key
// begins with a letter and a task's is its number.
func latestTaskRecord(ctx context.Context, store contract.Store) (contract.Record, bool) {
	number, there := latestTaskNumber(ctx, store)
	if !there {
		return contract.Record{}, false
	}
	return loadTaskRecord(ctx, store, number)
}

// latestTaskNumber is the number of the newest task the log holds a
// checkpoint for, or nothing when it holds none.
func latestTaskNumber(ctx context.Context, store contract.Store) (string, bool) {
	if store == nil {
		return "", false
	}
	saved, err := store.ByKind(ctx, contract.EventCheckpoint)
	if err != nil {
		return "", false
	}
	highest := 0
	for _, event := range saved {
		number, err := strconv.Atoi(event.TaskID)
		if err != nil || number < 1 {
			continue
		}
		if number > highest {
			highest = number
		}
	}
	if highest == 0 {
		return "", false
	}
	return strconv.Itoa(highest), true
}

// taskHeaderLine is the one line that says which task it is, where it stands, the
// channel it came in on, and a short cut of the ask, in the manner of the tasks
// listing.
func taskHeaderLine(held contract.Record) string {
	parts := []string{"task " + held.Header.ID, string(held.Header.Status)}
	if held.Header.Origin != "" {
		parts = append(parts, "from "+held.Header.Origin)
	}
	line := strings.Join(parts, "  ")
	if ask := shortAsk(held.Goal.Ask); ask != "" {
		line += "  " + ask
	}
	return line
}

// taskStandingLine is the one line that says where the work stands: the few facts
// the harness keeps when it has them, the count of plan steps done when it does
// not, and a plain word that nothing has been recorded yet when there is neither.
func taskStandingLine(held contract.Record) string {
	if len(held.Work.Situation) > 0 {
		return "where it stands: " + onOneLine(strings.Join(held.Work.Situation, "; "))
	}
	if done, total := stepsDone(held.Work.Plan); total > 0 {
		return fmt.Sprintf("where it stands: %d of %d steps done", done, total)
	}
	return "where it stands: nothing recorded yet"
}

// runningJobLine is the one line of progress for the job whose task is running
// now, and is empty when the running task is a person's or nothing runs. A job
// whose record cannot be read is still named by its number and its task, because
// a person watching a job wants to know which one it was.
func (running *agent) runningJobLine(ctx context.Context) string {
	if running.loop == nil || running.jobs == nil {
		return ""
	}
	fromJob, isJob := running.loop.RunningJobTask()
	if !isJob {
		return ""
	}
	held, err := running.jobs.Load(ctx, fromJob.JobID)
	if err != nil {
		return fmt.Sprintf("job %s  running task %s", fromJob.JobID, fromJob.TaskID)
	}
	line := fmt.Sprintf("job %s  %d of %d tasks done", fromJob.JobID, held.Header.TasksDone, held.Header.TasksTotal)
	if ask := shortAsk(held.Goal.Ask); ask != "" {
		line += "  " + ask
	}
	return line + fmt.Sprintf("  (running task %s)", fromJob.TaskID)
}

// stepsDone is how many of a plan's steps are done and how many there are.
func stepsDone(plan []contract.PlanStep) (done int, total int) {
	for _, step := range plan {
		if step.Done {
			done++
		}
	}
	return done, len(plan)
}

// shortAsk folds an ask onto one line and cuts it to the share the status answer
// shows, so that a long ask does not fill the reply. An ask inside its share
// comes back whole.
func shortAsk(ask string) string {
	one := onOneLine(ask)
	runes := []rune(one)
	if len(runes) <= maxAskLettersInTheStatus {
		return one
	}
	return strings.TrimRight(string(runes[:maxAskLettersInTheStatus]), " ") + "..."
}
