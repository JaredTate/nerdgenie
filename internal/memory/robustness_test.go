package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/memory"
)

func TestEveryCallSaysSoOnceTheMemoryIsClosed(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")
	opened.writeEvent(t, contract.EventMessage, map[string]string{
		"role": "user", "text": "never post before nine in the morning",
	})
	if err := opened.memory.Close(); err != nil {
		t.Fatalf("cannot close the memory: %v", err)
	}

	if err := opened.memory.Save(ctx, []contract.Fact{{Text: "something", Source: "task 17"}}); err == nil {
		t.Error("saving into a closed memory returned no error")
	}
	if _, err := opened.memory.Search(ctx, "anniversary", 5); err == nil {
		t.Error("searching a closed memory returned no error")
	}
	if _, err := opened.memory.Search(ctx, "", 5); err == nil {
		t.Error("listing a closed memory returned no error")
	}
	if _, err := opened.memory.Get(ctx, "m1"); err == nil {
		t.Error("reading a fact out of a closed memory returned no error")
	}
	if _, err := opened.memory.Get(ctx, theNotePrefix+"memory/a.md"); err == nil {
		t.Error("reading a note out of a closed memory returned no error")
	}
	if _, err := opened.memory.Get(ctx, theMessagePrefix+"1"); err == nil {
		t.Error("reading a message out of a closed memory returned no error")
	}
	if _, err := opened.memory.Hint(ctx, "anniversary"); err == nil {
		t.Error("asking a closed memory for a hint returned no error")
	}
	if err := opened.memory.Capture(ctx, theCapturedTask); err == nil {
		t.Error("capturing into a closed memory returned no error")
	}
	if err := opened.memory.IndexEvent(ctx, contract.Event{
		Sequence: 1, Kind: contract.EventMessage, Body: []byte(`{"text":"hello there"}`),
	}); err == nil {
		t.Error("indexing a message into a closed memory returned no error")
	}
	for _, arguments := range []string{"", "search anniversary", "forget m1"} {
		if _, err := opened.memory.Command().Run(ctx, arguments, contract.CommandContext{}); err == nil {
			t.Errorf("/memory %q on a closed memory returned no error", arguments)
		}
	}
}

func TestASaveThatCannotWriteTheSecondFilePutsTheFirstOneBack(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	before, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	if err := os.Remove(opened.home.UserFactsFile()); err != nil && !os.IsNotExist(err) {
		t.Fatalf("cannot clear the way for the folder: %v", err)
	}
	if err := os.Mkdir(opened.home.UserFactsFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot put a folder where USER.md goes: %v", err)
	}

	err = opened.memory.Save(ctx, []contract.Fact{
		{Text: "the blog is built with Hugo", Source: "task 17"},
		{ID: "u1", Text: "the user posts in the morning", Source: "task 17"},
	})
	if err == nil {
		t.Fatal("saving into a folder where USER.md belongs returned no error")
	}

	after, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file again: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("MEMORY.md was left changed by a save that failed, from\n%s\nto\n%s", before, after)
	}
	found, err := opened.memory.Search(ctx, "Hugo", 5)
	if err != nil {
		t.Fatalf("cannot search after the failed save: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a save that failed still put %v into the index", factTexts(found))
	}
}

func TestANoteWhoseWordsChangeIsIndexedAgain(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)
	notePath := filepath.Join(opened.home.MemoryFolder(), "product.md")

	if err := os.WriteFile(notePath, []byte("the kayaks are stored in the shed\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if found, err := reopen(t, opened).Search(ctx, "kayaks shed", 5); err != nil || len(found) == 0 {
		t.Fatalf("the note is not searchable to begin with: %v %v", found, err)
	}
	if err := os.WriteFile(notePath, []byte("the canoes are stored in the barn\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note again: %v", err)
	}

	remembering := reopen(t, opened)
	fresh, err := remembering.Search(ctx, "canoes barn", 5)
	if err != nil {
		t.Fatalf("cannot search for the new words: %v", err)
	}
	if len(fresh) == 0 {
		t.Error("the note's new words are not searchable")
	}
	stale, err := remembering.Search(ctx, "kayaks shed", 5)
	if err != nil {
		t.Fatalf("cannot search for the old words: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("the note's old words are still searchable, and the search found %v", factTexts(stale))
	}
}

func TestAVeryLongCorrectionIsCutAndSaysWhereTheRestIs(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	long := "never " + strings.Repeat("say this again ", 400)
	opened.writeEvent(t, contract.EventMessage, map[string]string{"role": "user", "text": long})

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task: %v", err)
	}
	found, err := opened.memory.Search(ctx, "never say this again", 3)
	if err != nil {
		t.Fatalf("cannot search for the long correction: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("a correction longer than a fact may be was not kept at all")
	}
	if !strings.Contains(found[0].Text, "the whole of it is in the event log") {
		t.Errorf("the long correction came back as %q, and it must say where the rest of it is", found[0].Text)
	}
	if len(found[0].Text) > theFactTextCap {
		t.Errorf("the long correction is %d bytes, and one fact may be at most %d", len(found[0].Text), theFactTextCap)
	}
}

func TestADatedNoteThatIsFullMakesTheNextOne(t *testing.T) {
	opened := newMemory(t, smallCaps)
	full := filepath.Join(opened.home.MemoryFolder(), "MEMORY-2026-09-02.md")
	if err := os.WriteFile(full, []byte(strings.Repeat("a note that is already full\n", 2500)), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the full note: %v", err)
	}

	for number := 1; number <= 12; number++ {
		opened.saveWorldFact(t, "fact number about the anniversary campaign")
	}
	next := filepath.Join(opened.home.MemoryFolder(), "MEMORY-2026-09-02-2.md")
	held, err := os.ReadFile(next)
	if err != nil {
		t.Fatalf("the second note of the day was not made: %v", err)
	}
	if !strings.Contains(string(held), "the anniversary campaign") {
		t.Errorf("the second note of the day holds:\n%s", held)
	}
}

func TestAMemoryFileTooBigToReadIsRefusedRatherThanLoaded(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	huge := strings.Repeat("this line is not a fact and there are a great many of them\n", 20000)
	if err := os.WriteFile(opened.home.WorldFactsFile(), []byte(huge), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the huge file: %v", err)
	}

	if err := opened.memory.Save(ctx, []contract.Fact{{Text: "something", Source: "task 17"}}); err == nil {
		t.Error("saving into a memory file too big to read returned no error")
	}
	again, err := memory.Open(ctx, opened.home, opened.eventLog, opened.clock, shippedCaps)
	if err == nil {
		_ = again.Close()
		t.Error("opening a memory whose file is too big to read returned no error")
	}
}

func TestAMintedIDStepsPastOneThatIsAlreadyTaken(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m1", Text: "the fact that took the first number", Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the fact with an id of its own: %v", err)
	}
	opened.saveWorldFact(t, "the fact that had to be given a number")

	found, err := opened.memory.Get(ctx, "m2")
	if err != nil {
		t.Fatalf("the minted id did not step past the one already taken: %v", err)
	}
	if found.Text != "the fact that had to be given a number" {
		t.Errorf("m2 came back as %q", found.Text)
	}
}

func TestAHandWrittenLineIsSearchableAsANoteOfItsOwn(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	held, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	byHand := "# the office keeps a spare key under the mat\n" + string(held)
	if err := os.WriteFile(opened.home.WorldFactsFile(), []byte(byHand), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file by hand: %v", err)
	}

	found, err := reopen(t, opened).Search(ctx, "spare key under the mat", 5)
	if err != nil {
		t.Fatalf("cannot search for the hand-written line: %v", err)
	}
	if len(found) == 0 || !strings.HasPrefix(found[0].ID, theNotePrefix) {
		t.Errorf("the hand-written line is not searchable, and the search found %+v", found)
	}
}
