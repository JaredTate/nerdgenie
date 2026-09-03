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
	"github.com/JaredTate/coeus/internal/memory"
)

// shippedCaps are the limits a fresh install ships with, which is what the
// tests that are not about the caps use.
var shippedCaps = contract.DefaultConfig().MemoryCaps

func TestASupersededFactIsStillSearchableAndSaysItWasSuperseded(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m1", Text: "the post leads with the features", Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the first fact: %v", err)
	}
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m2", Text: "the post leads with the date", Source: "task 17", Supersedes: "m1"},
	}); err != nil {
		t.Fatalf("cannot save the fact that supersedes the first one: %v", err)
	}

	found, err := opened.memory.Search(ctx, "the post leads with the features", 10)
	if err != nil {
		t.Fatalf("cannot search for the superseded fact: %v", err)
	}
	marked := ""
	for _, fact := range found {
		if fact.ID == "m1" {
			marked = fact.Text
		}
	}
	if marked == "" {
		t.Fatalf("the superseded fact is no longer searchable, and the search found %v", factTexts(found))
	}
	if !strings.Contains(marked, "superseded by m2") {
		t.Errorf("the superseded fact came back as %q, and it must say that m2 replaced it", marked)
	}

	byID, err := opened.memory.Get(ctx, "m1")
	if err != nil {
		t.Fatalf("cannot read the superseded fact by its id: %v", err)
	}
	if !strings.Contains(byID.Text, "superseded by m2") {
		t.Errorf("reading m1 by its id gave %q, and it must say that m2 replaced it", byID.Text)
	}
	if replacement, err := opened.memory.Get(ctx, "m2"); err != nil || replacement.Supersedes != "m1" {
		t.Errorf("the replacing fact came back as %+v with the error %v, and it must name m1", replacement, err)
	}
}

func TestASupersededUserFactStaysInTheUserFile(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "u1", Text: "the user posts in the morning", Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the user fact: %v", err)
	}
	if err := opened.memory.Save(ctx, []contract.Fact{
		{Text: "the user posts in the afternoon", Source: "task 19", Supersedes: "u1"},
	}); err != nil {
		t.Fatalf("cannot supersede the user fact: %v", err)
	}

	held, err := os.ReadFile(opened.home.UserFactsFile())
	if err != nil {
		t.Fatalf("cannot read the user facts file: %v", err)
	}
	if !strings.Contains(string(held), "the user posts in the afternoon") {
		t.Errorf("the replacing fact is not in USER.md, which holds:\n%s", held)
	}
	world, err := os.ReadFile(opened.home.WorldFactsFile())
	if err == nil && strings.Contains(string(world), "the user posts in the afternoon") {
		t.Errorf("the replacing fact went into MEMORY.md, and it belongs with the fact it replaced")
	}
}

func TestTheHintSaysNothingWhenNothingMatches(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	for _, query := range []string{"", "   ", "kayaks and canoes"} {
		hint, err := opened.memory.Hint(ctx, query)
		if err != nil {
			t.Fatalf("cannot ask for a hint for %q: %v", query, err)
		}
		if len(hint) != 0 {
			t.Errorf("the hint for %q is %v, and nothing matches it", query, hint)
		}
	}
}

func TestTheHintIsThreeLinesOfOneHundredAndTwentyCharactersAtMost(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	for number := 1; number <= 6; number++ {
		opened.saveWorldFact(t, fmt.Sprintf("%d %s about the anniversary campaign on the tenth of January",
			number, strings.Repeat("a very long fact ", 12)))
	}

	hint, err := opened.memory.Hint(ctx, "what do we know about the anniversary campaign")
	if err != nil {
		t.Fatalf("cannot ask for a hint: %v", err)
	}
	if len(hint) != contract.MemoryHintLines {
		t.Fatalf("the hint is %d lines, want %d", len(hint), contract.MemoryHintLines)
	}
	for _, line := range hint {
		if len([]rune(line)) > memory.MaxHintRunes {
			t.Errorf("the hint line is %d characters, and the cap is %d", len([]rune(line)), memory.MaxHintRunes)
		}
		if strings.ContainsAny(line, "\n\r") {
			t.Errorf("the hint line %q carries a line break, and each hint is one line", line)
		}
	}
}

func TestASearchIsCappedHoweverManyResultsAreAskedFor(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	for number := 1; number <= 60; number++ {
		opened.saveWorldFact(t, fmt.Sprintf("fact %d about the anniversary", number))
	}
	for _, limit := range []int{0, -3, 1000} {
		found, err := opened.memory.Search(ctx, "anniversary", limit)
		if err != nil {
			t.Fatalf("cannot search with the limit %d: %v", limit, err)
		}
		if len(found) > memory.MaxSearchResults {
			t.Errorf("a search with the limit %d gave %d results, and the cap is %d", limit, len(found), memory.MaxSearchResults)
		}
	}
	found, err := opened.memory.Search(ctx, "anniversary", 4)
	if err != nil {
		t.Fatalf("cannot search with a limit of four: %v", err)
	}
	if len(found) != 4 {
		t.Errorf("a search with a limit of four gave %d results", len(found))
	}
}

func TestASearchWithNoWordsInItReturnsTheNewestFirst(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	for number := 1; number <= 3; number++ {
		fact := contract.Fact{
			Text:     fmt.Sprintf("fact number %d", number),
			Source:   "task 17",
			Recorded: theTestDay.Add(time.Duration(number) * time.Hour),
		}
		if err := opened.memory.Save(ctx, []contract.Fact{fact}); err != nil {
			t.Fatalf("cannot save fact number %d: %v", number, err)
		}
	}
	found, err := opened.memory.Search(ctx, "   ", 3)
	if err != nil {
		t.Fatalf("cannot search with no words: %v", err)
	}
	if len(found) != 3 || found[0].Text != "fact number 3" {
		t.Errorf("a search with no words gave %v, and it must give the newest first", factTexts(found))
	}
}

func TestANoteInTheMemoryFolderIsSearchableAndReadableInFull(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)

	notePath := filepath.Join(opened.home.MemoryFolder(), "product.md")
	note := "# the product notes\n\nDigiByte is a blockchain that started in twenty fourteen.\n"
	if err := os.WriteFile(notePath, []byte(note), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	reopened := reopen(t, opened)

	found, err := reopened.Search(ctx, "blockchain twenty fourteen", 5)
	if err != nil {
		t.Fatalf("cannot search for the note: %v", err)
	}
	if len(found) == 0 || !strings.HasPrefix(found[0].ID, memory.NoteIDPrefix) {
		t.Fatalf("the note is not searchable, and the search found %v", found)
	}
	whole, err := reopened.Get(ctx, found[0].ID)
	if err != nil {
		t.Fatalf("cannot read the note by its id: %v", err)
	}
	if whole.Text != note {
		t.Errorf("the note came back as %q, want the whole file", whole.Text)
	}
}

func TestANoteThatIsDeletedLeavesTheIndex(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)

	notePath := filepath.Join(opened.home.MemoryFolder(), "product.md")
	if err := os.WriteFile(notePath, []byte("kayaks are boats you sit inside\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if found, err := reopen(t, opened).Search(ctx, "kayaks", 5); err != nil || len(found) == 0 {
		t.Fatalf("the note is not searchable to begin with: %v %v", found, err)
	}
	if err := os.Remove(notePath); err != nil {
		t.Fatalf("cannot delete the note: %v", err)
	}
	found, err := reopen(t, opened).Search(ctx, "kayaks", 5)
	if err != nil {
		t.Fatalf("cannot search after the note was deleted: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("the deleted note is still in the index, and the search found %v", factTexts(found))
	}
}

func TestAPastMessageIsSearchableAndReadableByItsID(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)

	body, err := json.Marshal(map[string]string{"role": "user", "text": "please post about the kayak race"})
	if err != nil {
		t.Fatalf("cannot write the message body: %v", err)
	}
	sequence, err := opened.eventLog.Append(ctx, contract.Event{
		Occurred: theTestDay, TaskID: "17", Kind: contract.EventMessage, Body: body,
	})
	if err != nil {
		t.Fatalf("cannot write the message into the log: %v", err)
	}
	if err := opened.memory.IndexEvent(ctx, contract.Event{
		Sequence: sequence, Occurred: theTestDay, TaskID: "17", Kind: contract.EventMessage, Body: body,
	}); err != nil {
		t.Fatalf("cannot index the message: %v", err)
	}

	found, err := opened.memory.Search(ctx, "kayak race", 5)
	if err != nil {
		t.Fatalf("cannot search for the message: %v", err)
	}
	if len(found) == 0 || !strings.HasPrefix(found[0].ID, memory.MessageIDPrefix) {
		t.Fatalf("the past message is not searchable, and the search found %+v", found)
	}
	if found[0].Source != "a message in task 17" {
		t.Errorf("the message's source is %q, and it must say which task it belongs to", found[0].Source)
	}
	whole, err := opened.memory.Get(ctx, found[0].ID)
	if err != nil {
		t.Fatalf("cannot read the message by its id: %v", err)
	}
	if whole.Text != "please post about the kayak race" {
		t.Errorf("the message came back as %q", whole.Text)
	}
}

func TestAMessageAlreadyInTheLogIsIndexedWhenTheMemoryIsOpened(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)

	body, err := json.Marshal(map[string]string{"role": "user", "text": "remember the paddle boards"})
	if err != nil {
		t.Fatalf("cannot write the message body: %v", err)
	}
	if _, err := opened.eventLog.Append(ctx, contract.Event{
		Occurred: theTestDay, TaskID: "17", Kind: contract.EventMessage, Body: body,
	}); err != nil {
		t.Fatalf("cannot write the message into the log: %v", err)
	}

	found, err := reopen(t, opened).Search(ctx, "paddle boards", 5)
	if err != nil {
		t.Fatalf("cannot search after opening the memory again: %v", err)
	}
	if len(found) == 0 {
		t.Error("a message already in the log was not indexed when the memory was opened")
	}
}

func TestReadingSomethingThatIsNotThereSaysSo(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()

	missing := []string{"m404", memory.NoteIDPrefix + "memory/nothing.md", memory.MessageIDPrefix + "9999",
		memory.MessageIDPrefix + "not a number", memory.NoteIDPrefix + "../../outside.md"}
	for _, id := range missing {
		if _, err := opened.memory.Get(ctx, id); err == nil {
			t.Errorf("reading %q returned no error, and it must name the id", id)
		}
	}
}

func TestASearchWrittenInTheSearchTablesOwnLanguageIsStillJustWords(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	awkward := []string{
		`"quoted words"`,
		"anniversary AND NOT January",
		"NEAR(anniversary January, 2)",
		"anniv*",
		"^anniversary",
		"anniversary OR (January NOT tenth)",
		"-anniversary",
		"{anniversary}",
		"\x00 anniversary",
		strings.Repeat("anniversary ", 200),
	}
	for _, query := range awkward {
		if _, err := opened.memory.Search(ctx, query, 5); err != nil {
			t.Errorf("searching for %q failed: %v", query, err)
		}
		if _, err := opened.memory.Hint(ctx, query); err != nil {
			t.Errorf("asking for a hint for %q failed: %v", query, err)
		}
	}
}
