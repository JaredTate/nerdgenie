// How a task takes its number: one above the highest the log has ever held,
// so that no two tasks are ever written under one number.

package loop

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// nextTaskNumber is the number the next task takes: one above the highest the
// log has ever held. The log is read once and the count is kept afterwards.
//
// Both the checkpoints and the messages are counted, because a task that
// answered with no tools made no record and so no checkpoint, but its ask was
// written under its number; counting checkpoints alone handed that number out
// again on the next start, and the log then held two tasks' words under one.
func (theLoop *Loop) nextTaskNumber(ctx context.Context) (string, error) {
	theLoop.guard.Lock()
	defer theLoop.guard.Unlock()
	if !theLoop.counted {
		for _, kind := range []contract.EventKind{contract.EventCheckpoint, contract.EventMessage} {
			highest, err := theLoop.highestTaskNumberAmong(ctx, kind)
			if err != nil {
				return "", err
			}
			theLoop.highest = max(theLoop.highest, highest)
		}
		theLoop.counted = true
	}
	theLoop.highest++
	return strconv.Itoa(theLoop.highest), nil
}

// highestTaskNumberAmong is the highest task number any event of one kind in the
// log was written under, and zero when there is none.
func (theLoop *Loop) highestTaskNumberAmong(ctx context.Context, kind contract.EventKind) (int, error) {
	saved, err := theLoop.options.Store.ByKind(ctx, kind)
	if err != nil {
		return 0, fmt.Errorf("cannot read the %s events of the log to number the next task: %w", kind, err)
	}
	highest := 0
	for _, event := range saved {
		if number, isTask := taskNumberOf(event.TaskID); isTask && number > highest {
			highest = number
		}
	}
	return highest, nil
}

// taskNumberOf reads a task's own number out of a log key. A job's key begins
// with a letter, so this returns false for one.
func taskNumberOf(logKey string) (int, bool) {
	number, err := strconv.Atoi(logKey)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
}
