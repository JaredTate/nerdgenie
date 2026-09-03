package command_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aLogThatRefusesAReplay is an event log that will not be walked from the
// beginning. "/undo" wants one turn, and one turn is a span of sequence
// numbers, so reading the whole log to find it is work that grows with every
// turn the agent has ever taken.
type aLogThatRefusesAReplay struct {
	*testkit.FakeStore
}

// Replay always fails, which is how a test sees that "/undo" never calls it.
func (aLogThatRefusesAReplay) Replay(context.Context, func(contract.Event) error) error {
	return errors.New("the whole log was replayed, and the last turn is read by range")
}

func TestUndoReadsTheLastTurnByRangeRatherThanReplayingTheLog(t *testing.T) {
	folder := t.TempDir()
	notes := writeFile(t, folder, "notes.md", "the words the agent wrote")

	store := testkit.NewFakeStore()
	appendMessage(t, store, "rewrite the notes")
	appendFileChange(t, store, contract.FileChangeBody{
		Path: notes, Existed: true, PriorContents: []byte("the words the user wrote"), Mode: uint32(contract.DataFileMode),
	})

	answer := runOne(t, command.Deps{Store: aLogThatRefusesAReplay{store}}, "/undo")

	restored, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("reading the restored file failed: %v", err)
	}
	if string(restored) != "the words the user wrote" {
		t.Errorf("the file came back as %q rather than what it held before the turn", restored)
	}
	if !strings.Contains(answer, notes) {
		t.Errorf("the undo command does not say which file it put back: %q", answer)
	}
}

func TestUndoDropsTheFileChangesMadeBeforeAnyMessage(t *testing.T) {
	folder := t.TempDir()
	notes := writeFile(t, folder, "notes.md", "written before any turn began")

	store := testkit.NewFakeStore()
	appendFileChange(t, store, contract.FileChangeBody{
		Path: notes, Existed: true, PriorContents: []byte("something older still"), Mode: uint32(contract.DataFileMode),
	})
	appendMessage(t, store, "what is the time")

	answer := runOne(t, command.Deps{Store: aLogThatRefusesAReplay{store}}, "/undo")

	if !strings.Contains(answer, "changed no files") {
		t.Errorf("the undo command does not say that the last turn changed nothing: %q", answer)
	}
	kept, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("reading the file back failed: %v", err)
	}
	if string(kept) != "written before any turn began" {
		t.Errorf("the undo command put back a change made before any message, and that change belongs to no turn")
	}
}
