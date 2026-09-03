package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// MaxTasksListed is how many tasks the tasks command prints, newest first.
const MaxTasksListed = 50

// MaxAskLettersInAListing is how much of a task's ask one line of the listing
// shows before it says the rest was cut, so that fifty tasks are fifty lines.
const MaxAskLettersInAListing = 60

// TasksCommand is the "/tasks" command: what is running, waiting, and done;
// "/tasks 17" prints one record; "/tasks 17 back 3" winds one back three
// checkpoints and lets the model try another path.
func (theLoop *Loop) TasksCommand() contract.Command {
	return contract.Command{
		Name: "tasks",
		Help: "Show what is running, waiting, and done. /tasks 17 prints one record, /tasks 17 back 3 winds it back three checkpoints.",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			words := strings.Fields(arguments)
			switch {
			case len(words) == 0:
				return theLoop.listTasks(ctx)
			case len(words) == 1:
				return theLoop.printTask(ctx, words[0])
			case len(words) == 3 && words[1] == "back":
				return theLoop.windTaskBack(ctx, words[0], words[2])
			default:
				return "", fmt.Errorf("that is not a way to ask about tasks, so write %q, %q, or %q",
					"/tasks", "/tasks 17", "/tasks 17 back 3")
			}
		},
	}
}

// StopCommand is the "/stop" command, which stops the running task as soon as
// the tool call in flight has finished.
func (theLoop *Loop) StopCommand() contract.Command {
	return contract.Command{
		Name: "stop",
		Help: "Stop the task that is running now.",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			running := theLoop.Running()
			theLoop.Stop()
			if running == "" {
				return "Nothing is running, so there was nothing to stop.", nil
			}
			return "Task " + running + " will stop as soon as the tool it is running has finished.", nil
		},
	}
}

// listTasks prints one line per task the log knows about, newest first. The log
// is read once: every task's newest checkpoint is already in that one read, and
// loading each task instead would replay that task's whole checkpoint history
// again for one line of text.
func (theLoop *Loop) listTasks(ctx context.Context) (string, error) {
	newest, err := theLoop.newestCheckpointOfEachTask(ctx)
	if err != nil {
		return "", err
	}
	if len(newest) == 0 {
		return "There are no tasks yet.", nil
	}
	numbers := slices.Sorted(maps.Keys(newest))
	slices.Reverse(numbers)
	if len(numbers) > MaxTasksListed {
		numbers = numbers[:MaxTasksListed]
	}
	lines := []string{}
	for _, number := range numbers {
		held, err := record.Parse([]byte(newest[number].Text))
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("task %s  %s  %s", held.Header.ID, held.Header.Status, oneLineOf(held.Goal.Ask)))
	}
	return strings.Join(lines, "\n"), nil
}

// newestCheckpointOfEachTask reads the log once and keeps the last checkpoint
// each task saved, which is where that task stands now. A job's checkpoints are
// left out, because a job's key begins with a letter and a task's is its number.
func (theLoop *Loop) newestCheckpointOfEachTask(ctx context.Context) (map[int]record.Checkpoint, error) {
	saved, err := theLoop.options.Store.ByKind(ctx, contract.EventCheckpoint)
	if err != nil {
		return nil, fmt.Errorf("cannot read the log to list the tasks: %w", err)
	}
	newest := map[int]record.Checkpoint{}
	for _, event := range saved {
		number, isTask := taskNumberOf(event.TaskID)
		if !isTask {
			continue
		}
		one := record.Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			continue
		}
		if held, there := newest[number]; !there || one.Number >= held.Number {
			newest[number] = one
		}
	}
	return newest, nil
}

// printTask prints one record as the model reads it.
func (theLoop *Loop) printTask(ctx context.Context, number string) (string, error) {
	keeper, err := record.Load(ctx, theLoop.options.Store, contract.RecordTask, number)
	if err != nil {
		return "", fmt.Errorf("cannot show task %s: %w", number, err)
	}
	return keeper.Text(), nil
}

// windTaskBack reloads a record from the checkpoint the given number of steps
// before the latest, so that the model can try another path. Nothing in the log
// is lost: the checkpoints in between stay where they are.
func (theLoop *Loop) windTaskBack(ctx context.Context, number string, stepsWritten string) (string, error) {
	steps, err := strconv.Atoi(stepsWritten)
	if err != nil {
		return "", fmt.Errorf("%q is not a number of checkpoints to wind back, so write one such as %q", stepsWritten, "3")
	}
	keeper, err := record.Back(ctx, theLoop.options.Store, contract.RecordTask, number, steps)
	if err != nil {
		return "", fmt.Errorf("cannot wind task %s back %d checkpoints: %w", number, steps, err)
	}
	return fmt.Sprintf("Task %s is back at checkpoint %d.\n\n%s", number, keeper.LatestCheckpoint(), keeper.Text()), nil
}

// oneLineOf is a piece of text on one line and short enough for a listing.
func oneLineOf(text string) string {
	one := strings.Join(strings.Fields(text), " ")
	letters := []rune(one)
	if len(letters) <= MaxAskLettersInAListing {
		return one
	}
	return string(letters[:MaxAskLettersInAListing-3]) + "..."
}
