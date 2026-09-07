package loop

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/orientation"
)

// NewestResultsShown is how many of the task's results a fresh window of a task
// that is not new opens with, in full. Six, because the model read back 2.6
// results by id after a restart and 1.6 after a rewind when it was given two.
const NewestResultsShown = 6

// openTheWindow begins a task's conversation: the orientation first, with the
// task's newest results when it is being picked up rather than started, and
// then the ask as the person wrote it, last, because the last thing the model
// reads is what it answers. With the block after the ask, the fresh run of
// 6 September wrote no plan in any task but the first, where the run before
// it had written one in every task.
func (running *run) openTheWindow(ctx context.Context, task Task) {
	running.rememberTheOrientation(ctx, task.ResumeID != "")
	running.remember(contract.Message{Role: contract.RoleUser, Text: task.Message.Text})
	running.keepThrough = len(running.messages)
}

// TheFreshWindowLine is what the model reads, right before the ask, when the
// conversation reached its cap and was opened afresh.
const TheFreshWindowLine = "The conversation reached its size cap, so this is a fresh window on the same task: " +
	"the block above holds the newest results in full, the record holds every step and every result by its id, " +
	"and \"read r7\" brings any of them back. Go on from where the record says the work stands."

// FreshWindowMarker is the word in the log event a fresh window writes, so
// that a run's numbers can count them.
const FreshWindowMarker = "fresh window"

// reopenTheWindowIfFull opens a fresh window when the round has filled the
// one in hand past MaxMessagesKept, the way a pick-up opens one: the
// orientation with the newest six results in full, the line saying what
// happened, and the ask last, because the last thing the model reads is what
// it answers. The messages before the cap leave; every step and result of
// theirs is in the record and the log. The window used to lose its oldest
// half here instead, and run ten's polish task paid two re-reads of a hundred
// and fifteen thousand tokens, four minutes each, for a window that began in
// the middle of a round. The fresh window is written into the log as an event
// of its own, so that a run's report can count them.
func (running *run) reopenTheWindowIfFull(ctx context.Context) error {
	if !running.windowIsFull() {
		return nil
	}
	left := len(running.messages)
	running.messages = nil
	running.rememberTheOrientation(ctx, true)
	running.remember(contract.Message{Role: contract.RoleUser, Text: TheFreshWindowLine})
	running.remember(contract.Message{Role: contract.RoleUser, Text: running.task.Message.Text})
	running.keepThrough = len(running.messages)
	return running.theLoop.logEvent(ctx, running.taskID(), contract.EventRecordChange, struct {
		Marker       string `json:"marker"`
		Round        int    `json:"round"`
		MessagesLeft int    `json:"messagesLeft"`
	}{Marker: FreshWindowMarker, Round: running.roundsUsed, MessagesLeft: left})
}

// rememberTheOrientation puts the facts of the machine in front of the model
// as the first harness message of a fresh window: what is in the working
// folder, which ports are listening, and, when the task is not new, its newest
// results in full. It rides as a message of its own right before the ask, or
// before the rewind line, so that the ask stays last and, from the second
// round on, the block sits in the cached part of the conversation rather than
// in the changing tail. Every
// fresh window used to cost rounds of finding bearings: after each of four
// serve restarts on 6 September 2026 the model re-read about six results by
// their ids, and at the start of tasks it listed the folder and asked which
// ports were listening.
func (running *run) rememberTheOrientation(ctx context.Context, withResults bool) {
	facts := orientation.Facts{Folder: running.theLoop.options.WorkingDirectory}
	if withResults {
		facts.Results = running.newestResults(ctx)
	}
	running.remember(contract.Message{Role: contract.RoleUser, Text: orientation.Block(facts)})
}

// newestResults reads the last few results of the task's record in full,
// oldest first, and leaves out any that cannot be read.
func (running *run) newestResults(ctx context.Context) []orientation.Result {
	if running.keeper == nil {
		return nil
	}
	lines := running.keeper.Record().Work.Results
	if len(lines) > NewestResultsShown {
		lines = lines[len(lines)-NewestResultsShown:]
	}
	results := []orientation.Result{}
	for _, line := range lines {
		text, err := running.keeper.Read(ctx, line.ID)
		if err != nil {
			continue
		}
		results = append(results, orientation.Result{ID: line.ID, Text: text})
	}
	return results
}
