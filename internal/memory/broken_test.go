package memory

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aMemoryOn is a memory built around a database a test hands it, so that the
// paths through the files and the index can be walked without opening a whole
// home and an event log first.
func aMemoryOn(t *testing.T, database *sql.DB) *Memory {
	t.Helper()
	home := contract.NewHome(t.TempDir())
	for _, folder := range []string{home.PersonaFolder(), home.MemoryFolder()} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s for this test: %v", folder, err)
		}
	}
	return &Memory{
		home:     home,
		eventLog: testkit.NewFakeStore(),
		clock:    testkit.NewFakeClock(time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)),
		caps:     contract.DefaultConfig().MemoryCaps,
		database: database,
	}
}

// aBrokenMemory is a memory whose database has been closed under it, which is
// how the paths that report a broken index are reached without breaking a real
// database file in the middle of a real save.
func aBrokenMemory(t *testing.T) *Memory {
	t.Helper()
	return aMemoryOn(t, aClosedDatabase(t))
}

func TestTheIndexerSaysSoWhenTheDatabaseIsBroken(t *testing.T) {
	ctx := context.Background()
	remembering := aBrokenMemory(t)
	budget := &runBudget{left: maxIndexedPerRun}

	notePath := filepath.Join(remembering.home.MemoryFolder(), "product.md")
	if err := os.WriteFile(notePath, []byte("the kayaks are in the shed\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	fact := contract.Fact{ID: "m1", Text: "a fact", Source: "task 17", Recorded: remembering.clock.Now()}
	if err := os.WriteFile(remembering.home.WorldFactsFile(),
		[]byte(formatFactLine(fact)+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the memory file: %v", err)
	}

	if err := remembering.rebuild(ctx); err == nil {
		t.Error("rebuilding the index of a broken database returned no error")
	}
	if err := remembering.indexTheFactFiles(ctx, budget); err == nil {
		t.Error("indexing the memory files into a broken database returned no error")
	}
	if err := remembering.indexTheNotes(ctx, budget); err == nil {
		t.Error("indexing the notes into a broken database returned no error")
	}
	if err := remembering.indexTheMessages(ctx, budget); err == nil {
		t.Error("indexing the messages into a broken database returned no error")
	}
	if err := remembering.indexNoteText(ctx, "memory/product.md", "some words", notePath, budget); err == nil {
		t.Error("indexing one note into a broken database returned no error")
	}
	if err := remembering.recoverFacts(ctx, []contract.Fact{fact}, worldFacts, "persona/MEMORY.md", budget); err == nil {
		t.Error("recovering a fact into a broken database returned no error")
	}
	if err := remembering.indexFacts(ctx, remembering.database,
		[]storedFact{{fact: fact, family: worldFacts}}, map[string]string{"m1": "persona/MEMORY.md"}); err == nil {
		t.Error("writing a fact into a broken database returned no error")
	}
	if err := remembering.IndexEvent(ctx, contract.Event{
		Sequence: 1, Kind: contract.EventMessage, Body: []byte(`{"text":"the kayaks are in the shed"}`),
	}); err == nil {
		t.Error("indexing one message into a broken database returned no error")
	}
}

func TestSavingAndListingSaySoWhenTheDatabaseIsBroken(t *testing.T) {
	ctx := context.Background()
	remembering := aBrokenMemory(t)
	fact := contract.Fact{ID: "m1", Text: "a fact", Source: "task 17", Recorded: remembering.clock.Now()}

	if err := remembering.saveBatch(ctx, []contract.Fact{fact}); err == nil {
		t.Error("saving into a broken database returned no error")
	}
	if _, err := remembering.latestFacts(ctx, 10); err == nil {
		t.Error("listing the facts of a broken database returned no error")
	}
	if _, err := remembering.showWhatIsRemembered(ctx); err == nil {
		t.Error("showing what a broken memory holds returned no error")
	}
	if _, err := remembering.withdrawFact(ctx, "m1"); err == nil {
		t.Error("withdrawing a fact from a broken database returned no error")
	}
	if _, err := remembering.moveFactsOut(ctx, remembering.database, worldFacts,
		[]contract.Fact{fact}, map[string]string{}); err == nil {
		t.Error("writing down that a fact moved into a note returned no error on a broken database")
	}
	written, err := remembering.writeFacts(ctx, remembering.database, []storedFact{{fact: fact, family: worldFacts}})
	if err == nil {
		t.Error("writing a batch of facts returned no error on a broken database")
	}
	putFilesBack(written)
	remembering.logFileChanges(ctx, written)
}

func TestAMemoryFileThatIsOnlyHandWrittenLinesCannotBeBroughtUnderItsCap(t *testing.T) {
	ctx := context.Background()
	remembering := aMemoryOn(t, aDatabase(t))
	remembering.caps = contract.MemoryCaps{WorldFactsBytes: 20, UserFactsBytes: 20}
	byHand := "# a heading somebody wrote by hand that is already longer than the whole cap\n"
	if err := os.WriteFile(remembering.home.WorldFactsFile(), []byte(byHand), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the memory file: %v", err)
	}

	fact := contract.Fact{ID: "m1", Text: "a fact", Source: "task 17", Recorded: remembering.clock.Now()}
	_, _, err := remembering.writeFamily(ctx, remembering.database, worldFacts, []contract.Fact{fact})
	if err == nil {
		t.Fatal("a file whose hand-written lines are longer than its cap was written anyway")
	}
	for _, word := range []string{"by hand", "shorten"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("the error says %q, and it must say %q so the reader knows what to do", err, word)
		}
	}
}
