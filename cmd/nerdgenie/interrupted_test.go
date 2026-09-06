package main

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestARunningTaskIsMarkedInterruptedOnRestartAndCarriedOn holds job 1: a task
// whose record still stood at running when the process was killed is reconciled
// on the next start. The rebuild marks it stopped in the log so a later start
// does not keep finding it running, names it in the interrupted set, and lets
// "continue" pick it up again under its own number.
func TestARunningTaskIsMarkedInterruptedOnRestartAndCarriedOn(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "5", fromTheTerminal, contract.StatusRunning)

	remembered, err := rememberedFromTheLog(ctx, store)
	if err != nil {
		t.Fatalf("the memory could not be rebuilt from the log: %v", err)
	}

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "5" {
		t.Errorf("continue after the restart picked up %q, want task 5, which was interrupted by the shutdown", picked)
	}
	if picked, steers := remembered.taskToCarryOn(theTerminalScreen, "start something else"); picked != "5" || !steers {
		t.Errorf("a plain message after the restart picked up task %q with steer %v, want task 5 steered by it, because an interrupted task is unfinished work", picked, steers)
	}
	if got := remembered.interrupted; len(got) != 1 || got[0] != "5" {
		t.Errorf("the interrupted set is %v, want [5], so the first status can name the task that was cut off", got)
	}
	held, ok := loadTaskRecord(ctx, store, "5")
	if !ok {
		t.Fatal("task 5 has no record after the rebuild")
	}
	if held.Header.Status == contract.StatusRunning {
		t.Error("task 5 still stands at running after the rebuild, so the next start would find it running all over again")
	}
}

// TestARunningTaskInterruptedIsResilientToASecondRestart holds that reconciling
// is idempotent: marking a running task stopped once means the second start does
// not treat it as freshly interrupted, because its record no longer stands at
// running.
func TestARunningTaskInterruptedIsResilientToASecondRestart(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "5", fromTheTerminal, contract.StatusRunning)

	if _, err := rememberedFromTheLog(ctx, store); err != nil {
		t.Fatalf("the first rebuild failed: %v", err)
	}
	remembered, err := rememberedFromTheLog(ctx, store)
	if err != nil {
		t.Fatalf("the second rebuild failed: %v", err)
	}

	if len(remembered.interrupted) != 0 {
		t.Errorf("the second start named %v as interrupted, and a task already reconciled is not interrupted again", remembered.interrupted)
	}
	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "5" {
		t.Errorf("continue on the second start picked up %q, want task 5, which is still resumable as a stopped task", picked)
	}
}

// TestNoTaskIsFalselyMarkedInterrupted holds the false-positive guard: a start
// whose newest tasks are done, waiting, stopped, or failed marks none of them
// interrupted, because only a record left at running was cut off by a shutdown.
func TestNoTaskIsFalselyMarkedInterrupted(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "2", fromTheTerminal, contract.StatusDone)
	aTaskInTheLog(t, store, "3", fromSignal, contract.StatusWaiting)
	aTaskInTheLog(t, store, "4", fromTheTerminal, contract.StatusStopped)
	aTaskInTheLog(t, store, "6", fromSignal, contract.StatusFailed)

	remembered, err := rememberedFromTheLog(ctx, store)
	if err != nil {
		t.Fatalf("the rebuild failed: %v", err)
	}

	if len(remembered.interrupted) != 0 {
		t.Errorf("the rebuild named %v as interrupted, and none of these tasks was left running", remembered.interrupted)
	}
}

// TestTheInterruptedTaskIsNamedInTheFirstStatusAScreenGets holds that the note
// the brief asks for reaches a screen the moment it attaches: the record line in
// the status a screen is first sent names the task that was interrupted and says
// to continue it.
func TestTheInterruptedTaskIsNamedInTheFirstStatusAScreenGets(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	aTaskInTheLog(t, running.events, "5", fromTheTerminal, contract.StatusRunning)
	running.rememberedTasks(context.Background())

	line := running.statusForAScreen()[contract.StatusFieldRecordLine]
	if !strings.Contains(line, "task 5") || !strings.Contains(line, "interrupted") || !strings.Contains(line, "next message picks it up") {
		t.Errorf("the first status carries the record line %q, want it to name task 5, say it was interrupted, and say the next message picks it up", line)
	}
}

// TestTwoScreensEachHaveTheirInterruptedTaskNamed holds that two screens each
// left running are reconciled and named smallest number first, and that each
// screen carries its own on.
func TestTwoScreensEachHaveTheirInterruptedTaskNamed(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "8", fromSignal, contract.StatusRunning)
	aTaskInTheLog(t, store, "5", fromTheTerminal, contract.StatusRunning)

	remembered, err := rememberedFromTheLog(ctx, store)
	if err != nil {
		t.Fatalf("the rebuild failed: %v", err)
	}

	if got := remembered.interrupted; len(got) != 2 || got[0] != "5" || got[1] != "8" {
		t.Errorf("the interrupted set is %v, want [5 8] smallest first", got)
	}
	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "5" {
		t.Errorf("the terminal's continue picked up %q, want task 5", picked)
	}
	if picked, _ := remembered.taskToCarryOn(theSignalScreen, "continue"); picked != "8" {
		t.Errorf("the Signal continue picked up %q, want task 8", picked)
	}
}

// TestAnInterruptedTaskIsNotNamedWhenANewerTaskTookItsScreen holds the filter:
// a running task that a newer, resumable task of the same screen sits above is
// reconciled in the log all the same, but it is not named, because "continue"
// would carry on the newer task and not it.
func TestAnInterruptedTaskIsNotNamedWhenANewerTaskTookItsScreen(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	aTaskInTheLog(t, store, "5", fromTheTerminal, contract.StatusRunning)
	aTaskInTheLog(t, store, "8", fromTheTerminal, contract.StatusWaiting)

	remembered, err := rememberedFromTheLog(ctx, store)
	if err != nil {
		t.Fatalf("the rebuild failed: %v", err)
	}

	if len(remembered.interrupted) != 0 {
		t.Errorf("task 5 was named %v though task 8 is the terminal's newest, so continue would not reach it", remembered.interrupted)
	}
	// It is still reconciled in the log, so a later start does not find it running.
	held, ok := loadTaskRecord(ctx, store, "5")
	if !ok || held.Header.Status == contract.StatusRunning {
		t.Error("task 5 was left at running, so the next start would keep finding it interrupted")
	}
}

// TestInterruptedRecordLineReadsForOneAndForMany holds the wording of the line
// the first status carries: nothing when none, one task named, and many joined.
func TestInterruptedRecordLineReadsForOneAndForMany(t *testing.T) {
	if got := interruptedRecordLine(nil); got != "" {
		t.Errorf("no interrupted task wrote %q, want nothing", got)
	}
	if got := interruptedRecordLine([]string{"5"}); got != "task 5 was interrupted; your next message picks it up" {
		t.Errorf("one interrupted task wrote %q", got)
	}
	if got := interruptedRecordLine([]string{"5", "8"}); got != "tasks 5, 8 were interrupted; your next message picks the newest up" {
		t.Errorf("two interrupted tasks wrote %q", got)
	}
}

// TestMarkInterruptedIsQuietWhenThereIsNoRecord holds that reconciling a number
// with no record behind it costs nothing: the write is skipped and the rebuild
// carries on, because a later start reconciles it if it is really there.
func TestMarkInterruptedIsQuietWhenThereIsNoRecord(t *testing.T) {
	markInterrupted(context.Background(), testkit.NewFakeStore(), "999")
}
