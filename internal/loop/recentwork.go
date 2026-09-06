package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The way the newest checkpoint of each task is read out of the log, then only
// the finished ones kept, borrows the read in cmd/nerdgenie/resuming.go's
// tasksToPickUp, which does the same to find the tasks a restart may carry on.
// The read is written fresh here rather than shared, because the loop must not
// depend on cmd.

// recentFinishedWork gathers the tasks most recently finished or set aside out
// of the event log: those whose newest checkpoint stands at done, stopped or
// failed, newest first, at most workingcontext.MaxRecentTasks of them, turned
// into the blocks the working context shows above the record so the model can
// answer "where are we" itself. The task numbered exclude is left out, because
// that is the task running now, or the one being picked up again.
//
// It reads the whole log once, so it is called once when a task starts and its
// answer reused for every call that task makes, never on every call.
func recentFinishedWork(ctx context.Context, store contract.Store, exclude string) ([]workingcontext.RecentTask, error) {
	saved, err := store.ByKind(ctx, contract.EventCheckpoint)
	if err != nil {
		return nil, fmt.Errorf("cannot read the checkpoints out of the log to gather the recent work: %w", err)
	}
	newestText := newestCheckpointTextOfEachTask(saved)

	// Which of those tasks are finished, read from the newest checkpoint's
	// header, newest first. The current task is left out whatever it stands at.
	finished := []int{}
	for number, text := range newestText {
		if strconv.Itoa(number) == exclude {
			continue
		}
		held, err := record.Parse([]byte(text))
		if err != nil || !endedOrSetAside(held.Header.Status) {
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
			Status:   string(held.Header.Status),
			Ask:      held.Goal.Ask,
			Standing: standingOf(held),
		})
	}
	return recent, nil
}

// newestCheckpointTextOfEachTask is the text of the newest checkpoint of each
// task, by its number. Only the text is kept, because that is all the status
// is read from; a job's key begins with a letter and so is passed over here.
func newestCheckpointTextOfEachTask(saved []contract.Event) map[int]string {
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
	return newestText
}

// endedOrSetAside says whether a task is one recent work names: finished, or
// set aside by the person, whose work is still there on the disk.
func endedOrSetAside(status contract.RecordStatus) bool {
	return status == contract.StatusDone || status == contract.StatusStopped || status == contract.StatusFailed
}

// theSituationLinesWorthKeeping are the facts of a task's situation that still
// matter once it has ended: what it changed, and where the model said the
// work stood. On the live game build the next task was told the name of the
// game and not its folder, and went looking for it under ~/Code.
var theSituationLinesWorthKeeping = []string{"files changed in this task:", "tests:", "where the work stands:"}

// standingOf is one line on where a task ended: the summary of its last result,
// which is what the task produced, then the facts of its situation worth
// keeping, and the word "done" when it closed on the user's own word with no
// result and no situation.
func standingOf(held contract.Record) string {
	parts := []string{}
	if results := held.Work.Results; len(results) > 0 {
		parts = append(parts, results[len(results)-1].Summary)
	}
	for _, fact := range held.Work.Situation {
		for _, keep := range theSituationLinesWorthKeeping {
			if strings.HasPrefix(fact, keep) {
				parts = append(parts, fact)
			}
		}
	}
	if len(parts) == 0 {
		return string(contract.StatusDone)
	}
	return strings.Join(parts, "; ")
}
