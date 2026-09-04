package main

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

func TestNoteRecordLineFoldsItKeepsItAndRollsTheBoundaryOnAStart(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// A line that says a task started rolls the data boundary through the
	// builder as well as being remembered.
	running.noteRecordLine("task 7 started" + loop.RecordLineSeparator + "write the\nnotes")
	if got := running.lastRecordLine(); got != "task 7 started"+loop.RecordLineSeparator+"write the notes" {
		t.Errorf("the last record line is %q, want it folded onto one line", got)
	}
}

func TestTheStatusForAScreenCarriesTheRunningStateAndTheLines(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	running.noteRecordLine("task 7 running the work")
	running.noteToolLine("running the shell tool")
	if !running.takeTheLoop() {
		t.Fatal("could not take the loop to stand in for a running task")
	}
	defer running.freeTheLoop()

	fields := running.statusForAScreen()

	if fields[contract.StatusFieldState] != contract.StateThinking {
		t.Errorf("the state is %q while a task runs, want thinking", fields[contract.StatusFieldState])
	}
	if fields[contract.StatusFieldRecordLine] == "" {
		t.Error("the status carries no record line, though one was noted")
	}
	if fields[contract.StatusFieldToolLine] == "" {
		t.Error("the status carries no tool line, though one was noted")
	}
	if fields[contract.StatusFieldModel] == "" {
		t.Error("the status carries no model name")
	}
	if fields[contract.StatusFieldCommands] == "" {
		t.Error("the status carries no command list for the palette")
	}
}
