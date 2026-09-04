package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// The way the newest checkpoint of each task is read out of the log, then only
// the finished ones kept, borrows the read in cmd/coeus/resuming.go's
// tasksToPickUp, which does the same to find the tasks a restart may carry on.
// The read is written fresh here rather than shared, because the loop must not
// depend on cmd.

// recentFinishedWork gathers the tasks most recently finished out of the event
// log: those whose newest checkpoint stands at done, newest first, at most
// workingcontext.MaxRecentTasks of them, turned into the blocks the working
// context shows above the record so the model can answer "where are we" itself.
// The task numbered exclude is left out, because that is the task running now.
//
// It reads the whole log once, so it is called once when a task starts and its
// answer reused for every call that task makes, never on every call.
func recentFinishedWork(ctx context.Context, store contract.Store, exclude string) ([]workingcontext.RecentTask, error) {
	saved, err := store.ByKind(ctx, contract.EventCheckpoint)
	if err != nil {
		return nil, fmt.Errorf("cannot read the checkpoints out of the log to gather the recent work: %w", err)
	}

	// The newest checkpoint of each task, by its number. Only its text is kept,
	// because that is all the status is read from; a job's key begins with a
	// letter and so is passed over here.
	newestText := map[int]string{}
	newestNumber := map[int]int{}
	for _, event := range saved {
		number, isTask := taskNumberOf(event.TaskID)
		if !isTask {
			continue
		}
		one := record.Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			continue
		}
		if held, known := newestNumber[number]; !known || one.Number >= held {
			newestNumber[number] = one.Number
			newestText[number] = one.Text
		}
	}

	// Which of those tasks are finished, read from the newest checkpoint's
	// header, newest first. The current task is left out whatever it stands at.
	finished := []int{}
	for number, text := range newestText {
		if strconv.Itoa(number) == exclude {
			continue
		}
		held, err := record.Parse([]byte(text))
		if err != nil || held.Header.Status != contract.StatusDone {
			continue
		}
		finished = append(finished, number)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(finished)))
	if len(finished) > workingcontext.MaxRecentTasks {
		finished = finished[:workingcontext.MaxRecentTasks]
	}

	// The whole of each kept task is read back, so the ask is the user's own
	// words even when a later checkpoint holds it elsewhere, and the standing is
	// read off the record as it ended.
	recent := make([]workingcontext.RecentTask, 0, len(finished))
	for _, number := range finished {
		keeper, err := record.Load(ctx, store, contract.RecordTask, strconv.Itoa(number))
		if err != nil {
			continue
		}
		held := keeper.Record()
		recent = append(recent, workingcontext.RecentTask{
			Number:   number,
			Ask:      held.Goal.Ask,
			Standing: standingOf(held),
		})
	}
	return recent, nil
}

// standingOf is one line on where a finished task ended: the summary of its last
// result, which is what the task produced, and the word "done" when it closed on
// the user's own word rather than a result.
func standingOf(held contract.Record) string {
	if results := held.Work.Results; len(results) > 0 {
		return results[len(results)-1].Summary
	}
	return string(contract.StatusDone)
}
