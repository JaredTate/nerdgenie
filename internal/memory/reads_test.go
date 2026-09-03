package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// notesIndexed is how many notes the index holds.
func notesIndexed(t *testing.T, remembering *Memory) int {
	t.Helper()
	held := 0
	row := remembering.database.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM memory_indexed WHERE kind = ?", noteEntry)
	if err := row.Scan(&held); err != nil {
		t.Fatalf("cannot count the notes the index holds: %v", err)
	}
	return held
}

func TestForgettingDeletedNotesStopsAtTheBudget(t *testing.T) {
	ctx := context.Background()
	remembering := aMemoryOn(t, aDatabase(t))
	paths := []string{}
	for number := 1; number <= 3; number++ {
		path := filepath.Join(remembering.home.MemoryFolder(), fmt.Sprintf("note-%d.md", number))
		if err := os.WriteFile(path, []byte("the kayaks are in shed number "+fmt.Sprint(number)+"\n"),
			contract.DataFileMode); err != nil {
			t.Fatalf("cannot write note number %d: %v", number, err)
		}
		paths = append(paths, path)
	}
	if err := remembering.rebuildInside(ctx, newRunBudget()); err != nil {
		t.Fatalf("cannot index the three notes: %v", err)
	}
	if held := notesIndexed(t, remembering); held != 3 {
		t.Fatalf("the index holds %d notes to begin with, want three", held)
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			t.Fatalf("cannot delete the note %s: %v", path, err)
		}
	}

	if err := remembering.rebuildInside(ctx, &runBudget{left: 1}); err != nil {
		t.Fatalf("cannot run the indexer with a budget of one: %v", err)
	}
	if held := notesIndexed(t, remembering); held != 2 {
		t.Errorf("a run with a budget of one left %d notes in the index, want two, because forgetting "+
			"the notes whose files are gone is work and has to be paid for out of the run's budget", held)
	}
}

func TestTheIndexerDoesNotReadTheWholeLogAgainOnEveryOpen(t *testing.T) {
	ctx := context.Background()
	paged := &aPagedLog{perRead: 100}
	for number := 1; number <= 5; number++ {
		body, err := json.Marshal(map[string]string{
			"role": "user", "text": fmt.Sprintf("message number %d about the kayaks", number),
		})
		if err != nil {
			t.Fatalf("cannot write the message body: %v", err)
		}
		if _, err := paged.Append(ctx, contract.Event{
			TaskID: "17", Kind: contract.EventMessage, Body: body,
		}); err != nil {
			t.Fatalf("cannot write message number %d: %v", number, err)
		}
	}
	remembering := aMemoryReading(t, paged)

	if err := remembering.rebuildInside(ctx, newRunBudget()); err != nil {
		t.Fatalf("cannot index the messages the first time: %v", err)
	}
	paged.spans = nil
	paged.replays = 0
	if err := remembering.rebuildInside(ctx, newRunBudget()); err != nil {
		t.Fatalf("cannot index the messages a second time: %v", err)
	}

	if paged.replays != 0 {
		t.Errorf("the second run replayed the whole log %d times, and a run must ask only for the "+
			"events written since the run before it", paged.replays)
	}
	if len(paged.spans) == 0 {
		t.Fatal("the second run read no span of the log at all")
	}
	if paged.spans[0].From != 6 {
		t.Errorf("the second run started reading at event %d, want 6, which is the one after the last "+
			"event the run before it read", paged.spans[0].From)
	}
}

func TestASaveOfMoreFactsThanOneBatchMayHoldIsRefused(t *testing.T) {
	ctx := context.Background()
	remembering := aMemoryOn(t, aDatabase(t))
	batch := make([]contract.Fact, 0, maxFactsPerBatch+1)
	for number := 1; number <= maxFactsPerBatch+1; number++ {
		batch = append(batch, contract.Fact{
			Text: fmt.Sprintf("fact number %d about the anniversary", number), Source: "task 17",
		})
	}

	err := remembering.Save(ctx, batch)
	if err == nil {
		t.Fatal("saving more facts than one batch may hold returned no error")
	}
	if !strings.Contains(err.Error(), "at a time") {
		t.Errorf("the error says %q, and it must say how many facts one save may carry", err)
	}
}

func TestANoteReadBackByItsIDIsCutToTheBytesANoteMayBe(t *testing.T) {
	ctx := context.Background()
	remembering := aMemoryOn(t, aDatabase(t))
	path := filepath.Join(remembering.home.MemoryFolder(), "product.md")
	// Every one of these characters is three bytes, so a note well under the
	// number of runes a note may be is well over the number of bytes.
	if err := os.WriteFile(path, []byte(strings.Repeat("…", 30000)), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if err := remembering.rebuildInside(ctx, newRunBudget()); err != nil {
		t.Fatalf("cannot index the note: %v", err)
	}

	found, err := remembering.Get(ctx, NoteIDPrefix+"memory/product.md")
	if err != nil {
		t.Fatalf("cannot read the note back by its id: %v", err)
	}
	if len(found.Text) > MaxNoteBytes {
		t.Errorf("the note came back as %d bytes, and a note may be at most %d; the cap counts bytes "+
			"and cutting it by runes lets a note of many-byte characters through", len(found.Text), MaxNoteBytes)
	}
}
