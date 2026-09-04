package main

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestTakingAndFreeingTheLoopIsOneAtATime(t *testing.T) {
	running := &agent{}

	if !running.takeTheLoop() {
		t.Fatal("the first caller could not take the loop, and nothing was running")
	}
	if running.takeTheLoop() {
		t.Error("a second caller took the loop while the first still held it, and one session runs one turn at a time")
	}
	running.freeTheLoop()
	if !running.takeTheLoop() {
		t.Error("the loop could not be taken again after it was freed")
	}
}

func TestTakeTheLoopWithinAMomentGivesUpWhenTheLoopStaysHeld(t *testing.T) {
	running := &agent{}
	if !running.takeTheLoop() {
		t.Fatal("could not take the loop to hold it for the test")
	}

	if running.takeTheLoopWithinAMoment() {
		t.Error("a message took a loop that was held for the whole wait, so it would run beside a task already under way")
	}
}

func TestStartTaskRefusesAChannelItIsNotRunning(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	message := contract.Inbound{Channel: "a-channel-nobody-runs", Sender: "someone", Text: "do the thing"}
	if err := running.startTask(context.Background(), newScreenTasks(), message); err == nil {
		t.Error("a message on a channel the agent is not running started a task, so its reply would go nowhere")
	}
}

func TestStartTaskHandsAMessageToTheTaskAlreadyRunning(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// A task is already running, so a new message is delivered to it rather than
	// starting a task of its own.
	if !running.takeTheLoop() {
		t.Fatal("could not take the loop to stand in for a running task")
	}
	defer running.freeTheLoop()

	message := contract.Inbound{Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName, Text: "a correction"}
	if err := running.startTask(context.Background(), newScreenTasks(), message); err != nil {
		t.Errorf("delivering a message to the running task failed: %v", err)
	}
}

func TestStopTaskAsksTheLoopToStop(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// Nothing is running, so this only proves the wiring reaches the loop's own
	// stop without trouble.
	if err := running.stopTask(context.Background()); err != nil {
		t.Errorf("asking the running task to stop failed: %v", err)
	}
}

func TestNoteToolLineFoldsTheLineOntoOneAndKeepsIt(t *testing.T) {
	running := &agent{}
	running.noteToolLine("running   the shell\ntool")
	if got := running.lastToolLine(); got != "running the shell tool" {
		t.Errorf("the last tool line is %q, want it folded onto one line", got)
	}
}
