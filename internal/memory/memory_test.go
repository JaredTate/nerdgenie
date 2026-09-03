package memory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/memory"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theTestDay is the moment the fake clock starts at, so that every dated note
// these tests look for has the same name.
var theTestDay = time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)

// smallCaps are file limits far below the shipped ones, so that a handful of
// facts is enough to push a file past its cap.
var smallCaps = contract.MemoryCaps{WorldFactsBytes: 400, UserFactsBytes: 300}

// The bounds these tests hold the package to, written out as the numbers they
// are rather than read back off the constants that hold them. A test that reads
// the constant moves whenever the constant moves and so pins nothing.
const (
	// theHintLineCap is the longest one line of the hint may be.
	theHintLineCap = 120
	// theSearchResultCap is the most results one search hands back.
	theSearchResultCap = 50
	// theFactTextCap is the longest one fact may be.
	theFactTextCap = 4000
)

// openedMemory is one memory opened on a real database file in a temporary home,
// with the event log behind it and a clock the test moves.
type openedMemory struct {
	memory   *memory.Memory
	home     contract.Home
	eventLog *log.Log
	clock    *testkit.FakeClock
}

// newMemory opens a memory for one test and closes it when the test ends.
func newMemory(t *testing.T, caps contract.MemoryCaps) openedMemory {
	t.Helper()
	ctx := context.Background()
	home := testkit.NewTempHome(t)

	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log for this test: %v", err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })

	clock := testkit.NewFakeClock(theTestDay)
	remembering, err := memory.Open(ctx, home, eventLog, clock, caps)
	if err != nil {
		t.Fatalf("cannot open the memory for this test: %v", err)
	}
	t.Cleanup(func() { _ = remembering.Close() })

	return openedMemory{memory: remembering, home: home, eventLog: eventLog, clock: clock}
}

// reopen opens the memory a second time on the same home, which is what makes
// the indexer run again over the files on disk and the events in the log.
func reopen(t *testing.T, opened openedMemory) *memory.Memory {
	t.Helper()
	again, err := memory.Open(context.Background(), opened.home, opened.eventLog, opened.clock, shippedCaps)
	if err != nil {
		t.Fatalf("cannot open the memory again: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	return again
}

// saveWorldFact saves one fact about the world and fails the test if it cannot.
func (opened openedMemory) saveWorldFact(t *testing.T, text string) {
	t.Helper()
	if err := opened.memory.Save(context.Background(), []contract.Fact{{Text: text, Source: "task 17"}}); err != nil {
		t.Fatalf("cannot save the fact %q: %v", text, err)
	}
}

// factTexts returns the text of every fact in a list, which is what most of
// these tests actually assert on.
func factTexts(facts []contract.Fact) []string {
	texts := make([]string, 0, len(facts))
	for _, fact := range facts {
		texts = append(texts, fact.Text)
	}
	return texts
}

// holdsText says whether any of the facts has exactly this text.
func holdsText(facts []contract.Fact, wanted string) bool {
	for _, fact := range facts {
		if fact.Text == wanted {
			return true
		}
	}
	return false
}

func TestASavePastTheCapMovesTheOldestFactsIntoADatedNote(t *testing.T) {
	opened := newMemory(t, smallCaps)

	for number := 1; number <= 12; number++ {
		opened.saveWorldFact(t, fmt.Sprintf("fact number %d about the anniversary campaign", number))
	}

	held, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	if len(held) > smallCaps.WorldFactsBytes {
		t.Fatalf("the world facts file is %d bytes and the cap is %d", len(held), smallCaps.WorldFactsBytes)
	}
	if !strings.Contains(string(held), "fact number 12 ") {
		t.Errorf("the newest fact is not in the file, which holds:\n%s", held)
	}
	if strings.Contains(string(held), "fact number 1 ") {
		t.Errorf("the oldest fact is still in the file, which holds:\n%s", held)
	}

	notePath := filepath.Join(opened.home.MemoryFolder(), "MEMORY-2026-09-02.md")
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("cannot read the dated note the oldest facts moved into: %v", err)
	}
	if !strings.Contains(string(note), "fact number 1 ") {
		t.Errorf("the dated note does not hold the oldest fact, and it holds:\n%s", note)
	}
}

func TestAFactMovedIntoADatedNoteIsStillSearchableAndStillReadableByItsID(t *testing.T) {
	opened := newMemory(t, smallCaps)
	ctx := context.Background()

	for number := 1; number <= 12; number++ {
		opened.saveWorldFact(t, fmt.Sprintf("fact number %d about the anniversary campaign", number))
	}

	found, err := opened.memory.Get(ctx, "m1")
	if err != nil {
		t.Fatalf("cannot read the fact that moved into a dated note: %v", err)
	}
	if found.Text != "fact number 1 about the anniversary campaign" {
		t.Errorf("the moved fact came back as %q", found.Text)
	}

	matches, err := opened.memory.Search(ctx, "fact number 1", 5)
	if err != nil {
		t.Fatalf("cannot search for the moved fact: %v", err)
	}
	if !holdsText(matches, "fact number 1 about the anniversary campaign") {
		t.Errorf("the moved fact is not searchable, and the search found %v", factTexts(matches))
	}
}

func TestMovingFactsIntoADatedNoteIsWrittenDownInTheLog(t *testing.T) {
	opened := newMemory(t, smallCaps)
	ctx := context.Background()
	for number := 1; number <= 12; number++ {
		opened.saveWorldFact(t, fmt.Sprintf("fact number %d about the anniversary campaign", number))
	}

	changes, err := opened.eventLog.ByKind(ctx, contract.EventFileChange)
	if err != nil {
		t.Fatalf("cannot read the file changes out of the log: %v", err)
	}
	written := map[string]bool{}
	for _, change := range changes {
		body := contract.FileChangeBody{}
		if err := json.Unmarshal(change.Body, &body); err != nil {
			t.Fatalf("cannot read a file change out of the log: %v", err)
		}
		written[filepath.Base(body.Path)] = true
	}
	for _, name := range []string{"MEMORY.md", "MEMORY-2026-09-02.md"} {
		if !written[name] {
			t.Errorf("the log holds no file change for %s, and every memory file Coeus writes is logged", name)
		}
	}
}

func TestTheTwoFilesKeepTheFactsThatBelongInThem(t *testing.T) {
	opened := newMemory(t, smallCaps)
	ctx := context.Background()

	err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "u1", Text: "the user posts at two in the afternoon", Source: "task 17"},
		{Text: "the anniversary is on the tenth of January", Source: "task 17"},
	})
	if err != nil {
		t.Fatalf("cannot save one fact for each file: %v", err)
	}

	world, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	user, err := os.ReadFile(opened.home.UserFactsFile())
	if err != nil {
		t.Fatalf("cannot read the user facts file: %v", err)
	}
	if !strings.Contains(string(world), "the anniversary is on the tenth of January") {
		t.Errorf("the world fact is not in MEMORY.md, which holds:\n%s", world)
	}
	if !strings.Contains(string(user), "the user posts at two in the afternoon") {
		t.Errorf("the user fact is not in USER.md, which holds:\n%s", user)
	}
}

func TestALineSomebodyWroteByHandIsKeptWhenTheFileIsWrittenAgain(t *testing.T) {
	opened := newMemory(t, smallCaps)

	byHand := "# facts I typed in myself\n"
	if err := os.WriteFile(opened.home.WorldFactsFile(), []byte(byHand), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file by hand: %v", err)
	}
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	held, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	if !strings.Contains(string(held), "# facts I typed in myself") {
		t.Errorf("the hand-written line was thrown away, and the file holds:\n%s", held)
	}
}

func TestASaveIsRefusedWhenAFactSaysNothingOrCannotBeWrittenDown(t *testing.T) {
	opened := newMemory(t, smallCaps)
	ctx := context.Background()

	refused := map[string][]contract.Fact{
		"a fact with no text":                {{Source: "task 17"}},
		"a fact whose text is spaces":        {{Text: "   ", Source: "task 17"}},
		"a fact with an id no line can hold": {{ID: "note:x", Text: "something", Source: "task 17"}},
		"a fact far past the text cap": {{
			Text:   strings.Repeat("a", theFactTextCap+1),
			Source: "task 17",
		}},
		"two facts with the same id": {
			{ID: "m9", Text: "the first one", Source: "task 17"},
			{ID: "m9", Text: "the second one", Source: "task 17"},
		},
		"a fact superseding one that is not there": {{
			Text: "something", Source: "task 17", Supersedes: "m404",
		}},
	}
	for what, facts := range refused {
		if err := opened.memory.Save(ctx, facts); err == nil {
			t.Errorf("saving %s returned no error, and it must say what is wrong", what)
		}
	}
}

func TestASaveThatIsRefusedChangesNothingOnDisk(t *testing.T) {
	opened := newMemory(t, smallCaps)
	ctx := context.Background()

	opened.saveWorldFact(t, "the anniversary is on the tenth of January")
	before, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}

	err = opened.memory.Save(ctx, []contract.Fact{
		{Text: "this one is fine", Source: "task 17"},
		{Text: "", Source: "task 17"},
	})
	if err == nil {
		t.Fatal("saving a batch with an empty fact in it returned no error")
	}

	after, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file again: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the file changed after a refused save, from\n%s\nto\n%s", before, after)
	}
	if _, err := opened.memory.Search(ctx, "this one is fine", 5); err != nil {
		t.Fatalf("cannot search after a refused save: %v", err)
	}
}
