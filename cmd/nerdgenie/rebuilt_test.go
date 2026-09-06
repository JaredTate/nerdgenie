package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The two screens these tests write tasks from: the terminal, and one person
// over Signal.
var (
	fromTheTerminal = contract.Inbound{Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName}
	fromSignal      = contract.Inbound{Channel: "signal", Sender: "+15125550123"}
)

// theSignalScreen is the name the Signal person's screen goes by.
const theSignalScreen = "signal:+15125550123"

// aTaskInTheLog writes what the loop writes for one task: the ask under the
// task's number, saying which screen it came from, then the record, standing
// where the test says.
func aTaskInTheLog(t *testing.T, store contract.Store, number string, from contract.Inbound, standing contract.RecordStatus) {
	t.Helper()
	ctx := context.Background()
	ask := "the ask of task " + number
	body, err := json.Marshal(contract.Inbound{Channel: from.Channel, Sender: from.Sender, Text: ask})
	if err != nil {
		t.Fatalf("cannot write the ask of task %s as an event: %v", number, err)
	}
	if _, err := store.Append(ctx, contract.Event{TaskID: number, Kind: contract.EventMessage, Body: body}); err != nil {
		t.Fatalf("cannot write the ask of task %s into the log: %v", number, err)
	}
	keeper, err := record.New(ctx, store, record.Start{
		Kind: contract.RecordTask, ID: number, Origin: from.Channel, Ask: ask, RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot start the record of task %s: %v", number, err)
	}
	// A record cannot close while a done line has nothing behind it, so a task
	// that ended done is given one line the person's own reply proves.
	if standing == contract.StatusDone {
		proved := []contract.DoneLine{{Text: "the ask is answered", Done: true, UserReply: "yes, it is"}}
		if err := keeper.Apply(ctx, record.Update{DoneWhen: proved}); err != nil {
			t.Fatalf("cannot write the done list of task %s: %v", number, err)
		}
	}
	if err := keeper.SetStatus(ctx, standing); err != nil {
		t.Fatalf("cannot set task %s to %s: %v", number, standing, err)
	}
}

// rebuilt is the memory rebuilt from a log, failing the test when the log could
// not be read.
func rebuilt(t *testing.T, store contract.Store) *screenTasks {
	t.Helper()
	remembered, err := rememberedFromTheLog(context.Background(), store)
	if err != nil {
		t.Fatalf("the memory could not be rebuilt from the log: %v", err)
	}
	return remembered
}

// TestTheMemoryOfAStoppedTaskIsRebuiltFromTheLog is the fourth Tetris run: the
// person stopped a task, the service restarted, and "continue" started a fresh
// task because the memory of the stopped one lived in the process.
func TestTheMemoryOfAStoppedTaskIsRebuiltFromTheLog(t *testing.T) {
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "3", fromTheTerminal, contract.StatusStopped)

	remembered := rebuilt(t, store)

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "3" {
		t.Errorf("continue after the restart picked up %q, want task 3, which the log says stopped", picked)
	}
	if picked, steers := remembered.taskToCarryOn(theTerminalScreen, "write the release notes"); picked != "3" || !steers {
		t.Errorf("a plain message after the restart picked up task %q with steer %v, want task 3 steered by it, because a stopped task is unfinished work", picked, steers)
	}
}

// TestTheMemoryOfAWaitingTaskIsRebuiltFromTheLog proves a question the model
// asked before the restart is still answered by the next message after it, and
// only from the screen that was asked.
func TestTheMemoryOfAWaitingTaskIsRebuiltFromTheLog(t *testing.T) {
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "4", fromSignal, contract.StatusWaiting)

	remembered := rebuilt(t, store)

	if picked, _ := remembered.taskToCarryOn(theSignalScreen, "the top one"); picked != "4" {
		t.Errorf("the answer from the Signal screen picked up %q, want task 4, which the log says is waiting on it", picked)
	}
	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "" {
		t.Errorf("the terminal picked up task %q, which was asked over Signal", picked)
	}
}

// TestATaskThatEndedIsNotRebuiltIntoTheMemory proves a finished task is not
// remembered: it takes no message, so "continue" after the restart starts a
// fresh task rather than picking a done one up. A task still marked running is
// not among these any more: a shutdown left it running, so the rebuild marks it
// interrupted and carries it on, which TestARunningTaskIsMarkedInterruptedOnRestartAndCarriedOn
// holds.
func TestATaskThatEndedIsNotRebuiltIntoTheMemory(t *testing.T) {
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "3", fromTheTerminal, contract.StatusDone)

	remembered := rebuilt(t, store)

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "" {
		t.Errorf("a task the log says is done was picked up as %q after the restart", picked)
	}
}

// TestAFailedTaskIsRebuiltFromTheLog proves a task cut off by an error before a
// restart is picked up again by "continue" under the number it already had,
// rather than starting a fresh task, so a crash mid-task loses no work.
func TestAFailedTaskIsRebuiltFromTheLog(t *testing.T) {
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "9", fromTheTerminal, contract.StatusFailed)

	remembered := rebuilt(t, store)

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "9" {
		t.Errorf("continue after the restart picked up %q, want task 9, which the log says failed", picked)
	}
	if picked, steers := remembered.taskToCarryOn(theTerminalScreen, "start something else"); picked != "9" || !steers {
		t.Errorf("a plain message after the restart picked up task %q with steer %v, want task 9 steered by it, because a failed task is unfinished work", picked, steers)
	}
}

// TestOnlyTheNewestWaitingOrStoppedTaskOfAScreenIsRebuilt proves the look-back
// is one task per screen: the newest that can be picked up, and not one before
// it.
func TestOnlyTheNewestWaitingOrStoppedTaskOfAScreenIsRebuilt(t *testing.T) {
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "2", fromTheTerminal, contract.StatusStopped)
	aTaskInTheLog(t, store, "5", fromTheTerminal, contract.StatusStopped)
	aTaskInTheLog(t, store, "7", fromTheTerminal, contract.StatusDone)
	aTaskInTheLog(t, store, "6", fromSignal, contract.StatusWaiting)

	remembered := rebuilt(t, store)

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "5" {
		t.Errorf("the terminal's continue picked up %q, want task 5, the newest of its tasks that stopped", picked)
	}
	if picked, _ := remembered.taskToCarryOn(theSignalScreen, "yes"); picked != "6" {
		t.Errorf("the Signal answer picked up %q, want task 6, which is that screen's own", picked)
	}
}

// TestATaskWhoseAskIsNotInTheLogIsPassedOver proves a record with no message
// under its number is left out rather than guessed at, because there is nothing
// to say which screen may pick it up.
func TestATaskWhoseAskIsNotInTheLogIsPassedOver(t *testing.T) {
	store := testkit.NewFakeStore()
	keeper, err := record.New(context.Background(), store, record.Start{
		Kind: contract.RecordTask, ID: "3", Origin: "terminal", Ask: "an ask nobody wrote down", RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot start the record: %v", err)
	}
	if err := keeper.SetStatus(context.Background(), contract.StatusStopped); err != nil {
		t.Fatalf("cannot stop the record: %v", err)
	}

	remembered := rebuilt(t, store)

	if held := remembered.howManyScreens(); held != 0 {
		t.Errorf("%d screens are remembered, and no screen can be told from a record whose ask is not in the log", held)
	}
}

// TestTheRebuildLooksBackOnlySoFar holds the bound: the newest tasks are looked
// at, as many as there are screens to remember, and the log is asked about no
// more of them than that, so a long log cannot make the start slow.
func TestTheRebuildLooksBackOnlySoFar(t *testing.T) {
	counting := &countingStore{Store: testkit.NewFakeStore()}
	for number := 1; number <= maxTasksLookedBackAt+5; number++ {
		from := contract.Inbound{Channel: "signal", Sender: "+1512555" + strconv.Itoa(number)}
		aTaskInTheLog(t, counting, strconv.Itoa(number), from, contract.StatusWaiting)
	}
	counting.byTask = 0

	remembered := rebuilt(t, counting)

	newest := "signal:+1512555" + strconv.Itoa(maxTasksLookedBackAt+5)
	if picked, _ := remembered.taskToCarryOn(newest, "yes"); picked != strconv.Itoa(maxTasksLookedBackAt+5) {
		t.Errorf("the newest screen picked up %q, want its own task, because the look-back starts from the newest", picked)
	}
	if picked, _ := remembered.taskToCarryOn("signal:+15125551", "yes"); picked != "" {
		t.Errorf("the oldest screen picked up task %q, and it is past the look-back", picked)
	}
	if counting.byTask > maxTasksLookedBackAt {
		t.Errorf("the log was asked about %d tasks, and the look-back is bounded at %d", counting.byTask, maxTasksLookedBackAt)
	}
}

// TestALogThatCannotBeReadLeavesTheMemoryEmptyAndSaysSo proves a broken log
// costs the memory and nothing more: the error is said, and what comes back
// still answers, so the agent starts and every message starts a fresh task.
func TestALogThatCannotBeReadLeavesTheMemoryEmptyAndSaysSo(t *testing.T) {
	remembered, err := rememberedFromTheLog(context.Background(), unreadableStore{Store: testkit.NewFakeStore()})

	if err == nil {
		t.Error("a log that cannot be read rebuilt the memory without a word")
	}
	if remembered == nil {
		t.Fatal("a log that cannot be read left no memory at all, and the agent needs one to start")
	}
	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "" {
		t.Errorf("an empty memory picked up task %q", picked)
	}
}

// countingStore counts how many tasks the rebuild asked the log about.
type countingStore struct {
	contract.Store
	byTask int
}

// ByTask counts the call and hands it on.
func (counting *countingStore) ByTask(ctx context.Context, taskID string) ([]contract.Event, error) {
	counting.byTask++
	return counting.Store.ByTask(ctx, taskID)
}

// unreadableStore is a log whose reads all fail, the way a damaged file's would.
type unreadableStore struct {
	contract.Store
}

// ByKind always fails.
func (unreadableStore) ByKind(context.Context, contract.EventKind) ([]contract.Event, error) {
	return nil, errors.New("the log cannot be read, so check the database file")
}
