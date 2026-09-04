package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestASaveIntoAFolderThatCannotBeWrittenSaysSo(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	persona := opened.home.PersonaFolder()
	if err := os.Chmod(persona, 0o500); err != nil {
		t.Fatalf("cannot make the persona folder read only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(persona, contract.HomeFolderMode) })

	if err := opened.memory.Save(ctx, []contract.Fact{
		{Text: "the anniversary is on the tenth of January", Source: "task 17"},
	}); err == nil {
		t.Fatal("saving into a folder that cannot be written returned no error")
	}
	if _, err := os.Stat(opened.home.WorldFactsFile()); err == nil {
		t.Error("a save that failed left MEMORY.md behind")
	}
	found, err := opened.memory.Search(ctx, "anniversary", 5)
	if err != nil {
		t.Fatalf("cannot search after the failed save: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a save that failed still put %v into the index", factTexts(found))
	}
}

func TestAMessageWithNothingReadableInItIsPassedOver(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	bodies := []string{`[1, 2, 3]`, `{"text":""}`, `{"text":"   "}`}
	for number, body := range bodies {
		err := opened.memory.IndexEvent(ctx, contract.Event{
			Sequence: int64(number + 1), Occurred: theTestDay, Kind: contract.EventMessage, Body: []byte(body),
		})
		if err != nil {
			t.Errorf("indexing the message body %s gave the error %v", body, err)
		}
	}
	found, err := opened.memory.Search(ctx, "", 10)
	if err != nil {
		t.Fatalf("cannot list what memory holds: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a message with nothing readable in it was indexed as %v", factTexts(found))
	}
}

func TestAMessageWithNoTaskOfItsOwnStillSaysWhereItCameFrom(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	err := opened.memory.IndexEvent(ctx, contract.Event{
		Sequence: 1, Occurred: theTestDay, Kind: contract.EventMessage,
		Body: []byte(`{"role":"user","text":"the kayaks are in the shed"}`),
	})
	if err != nil {
		t.Fatalf("cannot index the message: %v", err)
	}
	found, err := opened.memory.Search(ctx, "kayaks shed", 5)
	if err != nil {
		t.Fatalf("cannot search for the message: %v", err)
	}
	if len(found) == 0 || found[0].Source != "a past message" {
		t.Errorf("a message belonging to no task came back as %+v", found)
	}
}

func TestAMessageTheLogCannotReadBackSaysSoRatherThanGuessing(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	sequence, err := opened.eventLog.Append(ctx, contract.Event{
		Occurred: theTestDay, TaskID: "17", Kind: contract.EventMessage, Body: []byte(`[1, 2, 3]`),
	})
	if err != nil {
		t.Fatalf("cannot write the message into the log: %v", err)
	}
	err = opened.memory.IndexEvent(ctx, contract.Event{
		Sequence: sequence, Occurred: theTestDay, TaskID: "17", Kind: contract.EventMessage,
		Body: []byte(`{"role":"user","text":"the kayaks are in the shed"}`),
	})
	if err != nil {
		t.Fatalf("cannot index the message: %v", err)
	}
	if _, err := opened.memory.Get(ctx, theMessagePrefix+"1"); err == nil {
		t.Error("reading back a message the log cannot make sense of returned no error")
	}
}

func TestAnEventWithNothingCaptureCanVerifyLeavesNoFact(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	for _, kind := range []contract.EventKind{
		contract.EventFileChange, contract.EventMessage, contract.EventToolCall,
	} {
		if _, err := opened.eventLog.Append(ctx, contract.Event{
			Occurred: theTestDay, TaskID: theCapturedTask, Kind: kind, Body: []byte(`[1, 2, 3]`),
		}); err != nil {
			t.Fatalf("cannot write the unreadable %s into the log: %v", kind, err)
		}
	}
	opened.writeEvent(t, contract.EventFileChange, contract.FileChangeBody{Path: ""})
	opened.writeEvent(t, contract.EventCheckpoint, map[string]string{"text": "a checkpoint"})

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture a task whose events say nothing: %v", err)
	}
	if _, err := os.Stat(opened.home.WorldFactsFile()); err == nil {
		t.Error("a task whose events say nothing still left a memory file behind")
	}
}

func TestANoteTooBigToReadIsRefusedRatherThanLoaded(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)
	notePath := filepath.Join(opened.home.MemoryFolder(), "product.md")
	if err := os.WriteFile(notePath, []byte("the kayaks are in the shed\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	remembering := reopen(t, opened)

	huge := strings.Repeat("the kayaks are in the shed and so are the paddles\n", 25000)
	if err := os.WriteFile(notePath, []byte(huge), contract.DataFileMode); err != nil {
		t.Fatalf("cannot make the note too big to read: %v", err)
	}
	if _, err := remembering.Get(ctx, theNotePrefix+"memory/product.md"); err == nil {
		t.Error("reading a note too big to read returned no error")
	}
}

func TestAFactDatedOutsideTheYearsAFactLineCanHoldIsRefused(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	tooEarly := time.Date(-5, time.January, 1, 0, 0, 0, 0, time.UTC)
	err := opened.memory.Save(ctx, []contract.Fact{
		{Text: "a fact from before the years a date can hold", Source: "task 17", Recorded: tooEarly},
	})
	if err == nil {
		t.Error("saving a fact dated before the year one returned no error")
	}
}

func TestANoteInAFolderInsideTheMemoryFolderIsIndexedToo(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)
	folder := filepath.Join(opened.home.MemoryFolder(), "projects")
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder inside the memory folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "kayaks.md"),
		[]byte("the kayaks are stored in the shed behind the house\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "notes.txt"),
		[]byte("this file is not markdown and is not indexed\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file that is not markdown: %v", err)
	}

	remembering := reopen(t, opened)
	found, err := remembering.Search(ctx, "kayaks shed behind the house", 5)
	if err != nil {
		t.Fatalf("cannot search for the note in the folder: %v", err)
	}
	if len(found) == 0 || found[0].ID != theNotePrefix+"memory/projects/kayaks.md" {
		t.Errorf("the note in the folder came back as %+v", found)
	}
	plain, err := remembering.Search(ctx, "this file is not markdown", 5)
	if err != nil {
		t.Fatalf("cannot search for the file that is not markdown: %v", err)
	}
	if len(plain) != 0 {
		t.Errorf("a file that is not markdown was indexed as %v", factTexts(plain))
	}
}

func TestTheUserFileOverflowsIntoADatedNoteOfItsOwn(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, smallCaps)
	for number := 1; number <= 12; number++ {
		fact := contract.Fact{
			ID:     "u" + strings.Repeat("a", number),
			Text:   "the user likes the paddle boards more than the kayaks",
			Source: "task 17",
		}
		if err := opened.memory.Save(ctx, []contract.Fact{fact}); err != nil {
			t.Fatalf("cannot save user fact number %d: %v", number, err)
		}
	}

	held, err := os.ReadFile(filepath.Join(opened.home.MemoryFolder(), "USER-2026-09-02.md"))
	if err != nil {
		t.Fatalf("the user facts that left USER.md are in no dated note: %v", err)
	}
	if !strings.Contains(string(held), "moved out of USER.md") {
		t.Errorf("the dated note does not say where its facts came from:\n%s", held)
	}
	found, err := reopen(t, opened).Get(ctx, "ua")
	if err != nil {
		t.Fatalf("a user fact in a dated note was not found again after a restart: %v", err)
	}
	if found.Text != "the user likes the paddle boards more than the kayaks" {
		t.Errorf("the moved user fact came back as %q", found.Text)
	}
}

func TestALongQueryIsCutToItsFirstWords(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	manyWords := []string{}
	for number := 0; number < 80; number++ {
		manyWords = append(manyWords, "word"+strings.Repeat("y", number+1))
	}
	long := strings.Join(manyWords, " ") + " " + strings.Repeat("z", 300)
	if _, err := opened.memory.Search(ctx, long, 5); err != nil {
		t.Errorf("searching with a very long query failed: %v", err)
	}
}
