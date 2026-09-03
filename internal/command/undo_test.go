package command_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// appendMessage writes the event that starts a turn, so that everything after
// it belongs to the last turn.
func appendMessage(t *testing.T, store *testkit.FakeStore, text string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		t.Fatalf("writing the message body failed: %v", err)
	}
	if _, err := store.Append(context.Background(), contract.Event{TaskID: "17", Kind: contract.EventMessage, Body: body}); err != nil {
		t.Fatalf("appending the message event failed: %v", err)
	}
}

// appendFileChange writes one file-change event, which is what the write and
// edit tools record before they touch a file and what "/undo" puts back.
func appendFileChange(t *testing.T, store *testkit.FakeStore, change contract.FileChangeBody) {
	t.Helper()
	body, err := json.Marshal(change)
	if err != nil {
		t.Fatalf("writing the file-change body failed: %v", err)
	}
	if _, err := store.Append(context.Background(), contract.Event{TaskID: "17", Kind: contract.EventFileChange, Body: body}); err != nil {
		t.Fatalf("appending the file-change event failed: %v", err)
	}
}

// writeFile puts a file on disk for a test to undo, and gives back its path.
func writeFile(t *testing.T, folder string, name string, content string) string {
	t.Helper()
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(content), contract.DataFileMode); err != nil {
		t.Fatalf("writing the file %s failed: %v", path, err)
	}
	return path
}

func TestUndoPutsBackWhatTheLastTurnWroteAndRemovesWhatItMade(t *testing.T) {
	folder := t.TempDir()
	notes := writeFile(t, folder, "notes.md", "the words the agent wrote")
	draft := writeFile(t, folder, "draft.md", "a file that did not exist before")

	store := testkit.NewFakeStore()
	appendMessage(t, store, "write up the anniversary")
	appendFileChange(t, store, contract.FileChangeBody{
		Path: notes, Existed: true, PriorContents: []byte("the words the user wrote"), Mode: uint32(contract.DataFileMode),
	})
	appendFileChange(t, store, contract.FileChangeBody{Path: draft, Existed: false})

	answer := runOne(t, command.Deps{Store: store}, "/undo")

	restored, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("reading the restored file failed: %v", err)
	}
	if string(restored) != "the words the user wrote" {
		t.Errorf("the file came back as %q rather than what it held before the turn", restored)
	}
	if _, err := os.Stat(draft); !os.IsNotExist(err) {
		t.Errorf("the file the turn made is still there, and undo should have taken it away")
	}
	for _, named := range []string{"notes.md", "draft.md"} {
		if !strings.Contains(answer, named) {
			t.Errorf("the undo command does not say that it touched %s: %q", named, answer)
		}
	}
}

func TestUndoGoesBackToBeforeTheTurnWhenOneFileWasWrittenTwice(t *testing.T) {
	folder := t.TempDir()
	notes := writeFile(t, folder, "notes.md", "the third version")

	store := testkit.NewFakeStore()
	appendMessage(t, store, "rewrite the notes")
	appendFileChange(t, store, contract.FileChangeBody{
		Path: notes, Existed: true, PriorContents: []byte("the first version"), Mode: uint32(contract.DataFileMode),
	})
	appendFileChange(t, store, contract.FileChangeBody{
		Path: notes, Existed: true, PriorContents: []byte("the second version"), Mode: uint32(contract.DataFileMode),
	})

	runOne(t, command.Deps{Store: store}, "/undo")

	restored, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("reading the restored file failed: %v", err)
	}
	if string(restored) != "the first version" {
		t.Errorf("the file came back as %q rather than the version from before the turn began", restored)
	}
}

func TestUndoLeavesAnEarlierTurnAlone(t *testing.T) {
	folder := t.TempDir()
	earlier := writeFile(t, folder, "earlier.md", "written in the turn before")
	later := writeFile(t, folder, "later.md", "written in the last turn")

	store := testkit.NewFakeStore()
	appendMessage(t, store, "the first ask")
	appendFileChange(t, store, contract.FileChangeBody{
		Path: earlier, Existed: true, PriorContents: []byte("what earlier.md held first"), Mode: uint32(contract.DataFileMode),
	})
	appendMessage(t, store, "the second ask")
	appendFileChange(t, store, contract.FileChangeBody{
		Path: later, Existed: true, PriorContents: []byte("what later.md held first"), Mode: uint32(contract.DataFileMode),
	})

	runOne(t, command.Deps{Store: store}, "/undo")

	untouched, err := os.ReadFile(earlier)
	if err != nil {
		t.Fatalf("reading the earlier file failed: %v", err)
	}
	if string(untouched) != "written in the turn before" {
		t.Errorf("the undo command reached back past the last turn: earlier.md came back as %q", untouched)
	}
}

func TestUndoRefusesWhenThereIsNoTurn(t *testing.T) {
	answer := runOne(t, command.Deps{Store: testkit.NewFakeStore()}, "/undo")

	if !strings.Contains(answer, "no turn") {
		t.Errorf("the undo command does not say that there is no turn to undo: %q", answer)
	}
}

func TestUndoSaysWhenTheLastTurnChangedNoFiles(t *testing.T) {
	store := testkit.NewFakeStore()
	appendFileChange(t, store, contract.FileChangeBody{Path: "/nowhere/at/all", Existed: false})
	appendMessage(t, store, "what is the time")

	answer := runOne(t, command.Deps{Store: store}, "/undo")

	if !strings.Contains(answer, "changed no files") {
		t.Errorf("the undo command does not say that the last turn changed nothing: %q", answer)
	}
}

func TestUndoRefusesATurnThatChangedMoreFilesThanTheLimit(t *testing.T) {
	folder := t.TempDir()
	store := testkit.NewFakeStore()
	appendMessage(t, store, "rewrite everything")
	for at := 0; at <= command.MaxUndoFiles; at++ {
		appendFileChange(t, store, contract.FileChangeBody{
			Path: filepath.Join(folder, "file.md"), Existed: false,
		})
	}

	answer := runOne(t, command.Deps{Store: store}, "/undo")

	if !strings.Contains(answer, "changed more than") {
		t.Errorf("the undo command does not refuse a turn past the limit: %q", answer)
	}
}

func TestUndoSaysWhenThereIsNoEventLog(t *testing.T) {
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/undo", contract.CommandContext{Channel: terminalChannel()}); err == nil {
		t.Fatalf("the undo command claimed to put files back with no event log at all")
	}
}
