package memory

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// aDatabase opens a database file of its own with this package's tables in it,
// which is what the helpers below are exercised against directly.
func aDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "memory.db")+databaseOptions)
	if err != nil {
		t.Fatalf("cannot open a database for this test: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	for _, statement := range schemaStatements {
		if _, err := database.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("cannot make the tables for this test: %v", err)
		}
	}
	return database
}

// aClosedDatabase is a database every statement fails on, which is how the
// paths that report a broken database are reached.
func aClosedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database := aDatabase(t)
	if err := database.Close(); err != nil {
		t.Fatalf("cannot close the database for this test: %v", err)
	}
	return database
}

func TestEveryStatementSaysSoWhenTheDatabaseIsBroken(t *testing.T) {
	ctx := context.Background()
	broken := aClosedDatabase(t)
	entry := indexEntry{kind: noteEntry, reference: "memory/a.md", source: "memory/a.md", body: "some words"}

	if _, err := nextNumber(ctx, broken, "a counter"); err == nil {
		t.Error("reading a counter from a broken database returned no error")
	}
	if err := setNumber(ctx, broken, "a counter", 2); err == nil {
		t.Error("writing a counter into a broken database returned no error")
	}
	if _, err := factIsThere(ctx, broken, "m1"); err == nil {
		t.Error("looking for a fact in a broken database returned no error")
	}
	if _, _, err := factRow(ctx, broken, "m1"); err == nil {
		t.Error("reading a fact out of a broken database returned no error")
	}
	if _, err := familyOf(ctx, broken, contract.Fact{Supersedes: "m1"}); err == nil {
		t.Error("reading the family of a superseded fact from a broken database returned no error")
	}
	if _, err := mintFactID(ctx, broken, worldFacts); err == nil {
		t.Error("minting an id against a broken database returned no error")
	}
	if err := indexOneBody(ctx, broken, entry); err == nil {
		t.Error("indexing against a broken database returned no error")
	}
	if err := forgetIndexEntry(ctx, broken, noteEntry, "memory/a.md"); err == nil {
		t.Error("forgetting an entry in a broken database returned no error")
	}
	if err := insertIndexEntry(ctx, broken, entry, "a fingerprint"); err == nil {
		t.Error("inserting an entry into a broken database returned no error")
	}
	if err := updateIndexEntry(ctx, broken, entry, "a fingerprint", 1); err == nil {
		t.Error("updating an entry in a broken database returned no error")
	}
	remembering := &Memory{home: contract.NewHome(t.TempDir()), database: broken}
	if err := remembering.createTables(ctx); err == nil {
		t.Error("making the tables in a broken database returned no error")
	}
	if err := remembering.checkTheEventLogIsThere(ctx); err == nil {
		t.Error("looking for the events table in a broken database returned no error")
	}
	if err := remembering.forgetMissingNotes(ctx, newRunBudget()); err == nil {
		t.Error("listing the notes in a broken database returned no error")
	}
	if err := remembering.forgetMissingNotes(ctx, &runBudget{}); err != nil {
		t.Errorf("listing the notes with no budget left gave the error %v, and a run with nothing left "+
			"to spend must not read the index at all", err)
	}
}

func TestAnEntryIsIndexedOnceInsertedAgainWhenItChangesAndThenForgotten(t *testing.T) {
	ctx := context.Background()
	database := aDatabase(t)
	entry := indexEntry{
		kind:      noteEntry,
		reference: "memory/a.md",
		source:    "memory/a.md",
		recorded:  time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC),
		body:      "the kayaks are in the shed",
	}

	if err := indexOneBody(ctx, database, entry); err != nil {
		t.Fatalf("cannot index the entry: %v", err)
	}
	if err := indexOneBody(ctx, database, entry); err != nil {
		t.Fatalf("cannot index the same entry again: %v", err)
	}
	entry.body = "the canoes are in the barn"
	if err := indexOneBody(ctx, database, entry); err != nil {
		t.Fatalf("cannot index the entry once its words changed: %v", err)
	}

	held := ""
	row := database.QueryRowContext(ctx,
		"SELECT body FROM memory_search JOIN memory_indexed ON memory_indexed.row_id = memory_search.rowid")
	if err := row.Scan(&held); err != nil {
		t.Fatalf("cannot read the indexed entry back: %v", err)
	}
	if held != "the canoes are in the barn" {
		t.Errorf("the index holds %q, want the words the note now has", held)
	}

	if err := forgetIndexEntry(ctx, database, noteEntry, entry.reference); err != nil {
		t.Fatalf("cannot forget the entry: %v", err)
	}
	if err := forgetIndexEntry(ctx, database, noteEntry, entry.reference); err != nil {
		t.Errorf("forgetting an entry that is already gone gave the error %v", err)
	}
}

func TestACounterThatHoldsSomethingOtherThanANumberStartsAgainAtOne(t *testing.T) {
	ctx := context.Background()
	database := aDatabase(t)
	if _, err := database.ExecContext(ctx,
		"INSERT INTO memory_state (name, value) VALUES (?, ?)", "a counter", "not a number"); err != nil {
		t.Fatalf("cannot write the odd counter: %v", err)
	}
	number, err := nextNumber(ctx, database, "a counter")
	if err != nil {
		t.Fatalf("cannot read the odd counter: %v", err)
	}
	if number != 1 {
		t.Errorf("a counter holding something that is not a number read as %d, want one", number)
	}
}

func TestABudgetRunsOut(t *testing.T) {
	budget := &runBudget{left: 2}
	for spent := 1; spent <= 2; spent++ {
		if !budget.spend() {
			t.Fatalf("a budget of two would not pay for piece of work number %d", spent)
		}
	}
	if budget.spend() {
		t.Error("a budget of two paid for a third piece of work")
	}
}

func TestATimeThatCannotBeReadComesBackAsTheZeroTime(t *testing.T) {
	if !fromStoredTime("last Tuesday").IsZero() {
		t.Error("a time that cannot be read came back as something other than the zero time")
	}
	moment := time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)
	if !fromStoredTime(asStoredTime(moment)).Equal(moment) {
		t.Error("a time written down and read back is not the time it was")
	}
}

func TestACapturedIDFitsOnAFactLineHoweverLongTheTaskIs(t *testing.T) {
	long := capturedFactID(strings.Repeat("task name ", 20), 12, true)
	if !validFactID(long) {
		t.Errorf("the captured id %q could not be written on a fact line", long)
	}
	if !strings.HasPrefix(long, userFactPrefix+"c") {
		t.Errorf("the captured id %q does not say it is a fact about the user", long)
	}
	huge := capturedFactID("17", 1234567890123456789, false)
	if !validFactID(huge) {
		t.Errorf("the captured id %q for a very large event number could not be written on a fact line", huge)
	}
	if onlyIDLetters("task 17: the tweet") != "task17thetweet" {
		t.Errorf("the letters kept from a task id are %q", onlyIDLetters("task 17: the tweet"))
	}
}

func TestTextIsCutOnlyWhenItIsTooLong(t *testing.T) {
	where := "the whole of it is in the event log"
	if cutToBytes("short enough", 100, where) != "short enough" {
		t.Error("text inside the limit was cut")
	}
	cut := cutToBytes(strings.Repeat("é", 200), 100, where)
	if len(cut) > 100 {
		t.Errorf("the cut text is %d bytes, and the limit is 100", len(cut))
	}
	if !strings.Contains(cut, where) {
		t.Errorf("the cut text is %q, and it must say where the whole of it can still be read", cut)
	}
	if cutToBytes(strings.Repeat("a", 200), 10, where) == "" {
		t.Error("a limit smaller than the note it adds left nothing at all")
	}
	if cutToRunes("short enough", 0) != "short enough" {
		t.Error("a limit below one cut the text rather than leaving it alone")
	}
}

func TestTheFirstWordOfSomethingWithNoWordsInItIsEmpty(t *testing.T) {
	for _, said := range []string{"", "   ", "!!!", "..."} {
		if firstWord(said) != "" {
			t.Errorf("the first word of %q is %q, and there are no words in it", said, firstWord(said))
		}
	}
	if firstWord("Don’t do that") != "don't" {
		t.Errorf("the first word of a message with a curly apostrophe is %q", firstWord("Don’t do that"))
	}
}

func TestOnlyAMovedFactsNoteIsReadAsFacts(t *testing.T) {
	movedNotes := []string{"MEMORY-2026-09-02.md", "USER-2026-09-02-3.md"}
	for _, name := range movedNotes {
		if !movedFactsNote(name) {
			t.Errorf("%q is a note the overflow wrote, and it was not read as one", name)
		}
	}
	for _, name := range []string{"product.md", "MEMORY.md", "memory-2026-09-02.md", "USER-2026-09-02.txt"} {
		if movedFactsNote(name) {
			t.Errorf("%q was read as a note the overflow wrote, and it is not one", name)
		}
	}
}
