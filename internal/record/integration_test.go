//go:build integration

package record

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The event log this test writes to is a file of one JSON event per line, not the
// SQLite log of brief 1.1, because that package is being written in the same wave
// as this one and no package of a wave imports its neighbours. What this test
// proves is what matters here and does not depend on which log it is: a record
// written by one process is read back whole by another, out of a real file on a
// real filesystem, with every result still readable by its label. When the loop
// of wave 3 joins the two packages, the same test runs against the real log.

// TestARecordSurvivesBeingPutDownAndPickedUp writes a task to a file on disk,
// forgets everything in memory, opens the file again, and carries on: the record
// comes back whole, every result reads back in full, and winding it back three
// steps gives an earlier moment of the same task.
func TestARecordSurvivesBeingPutDownAndPickedUp(t *testing.T) {
	home := testkit.NewTempHome(t)
	path := filepath.Join(home.Root, "events.jsonl")
	ctx := context.Background()

	writing := newFileStore(path)
	keeper, err := New(ctx, writing, taskStart())
	if err != nil {
		t.Fatalf("cannot create the record: %v", err)
	}
	writeAWholeTask(t, keeper)
	wanted := keeper.Text()
	if keeper.LatestCheckpoint() != 8 {
		t.Fatalf("the written task stands at checkpoint %d", keeper.LatestCheckpoint())
	}

	reading := newFileStore(path)
	loaded, err := Load(ctx, reading, contract.RecordTask, "17")
	if err != nil {
		t.Fatalf("cannot load the record back out of %s: %v", path, err)
	}
	if loaded.Text() != wanted {
		t.Errorf("the record came back changed.\n--- written ---\n%s\n--- read ---\n%s", wanted, loaded.Text())
	}
	for _, id := range []string{"r1", "r2", "r3", "r4"} {
		text, err := loaded.Read(ctx, id)
		if err != nil {
			t.Errorf("cannot read %s back out of the file: %v", id, err)
			continue
		}
		if text != "the whole text of "+id {
			t.Errorf("%s reads back as %q", id, text)
		}
	}

	wound, err := Back(ctx, reading, contract.RecordTask, "17", 3)
	if err != nil {
		t.Fatalf("cannot wind the record back three steps: %v", err)
	}
	if wound.Record().Goal.Ask != taskStart().Ask {
		t.Errorf("winding back changed the ask to %q", wound.Record().Goal.Ask)
	}
	if len(wound.Record().Work.Results) != 2 {
		t.Errorf("three steps back left %d results, and the fourth and third came after",
			len(wound.Record().Work.Results))
	}
}

// writeAWholeTask does what a short task does: it writes its half of the record,
// gathers four results, takes a correction, and marks a step done.
func writeAWholeTask(t *testing.T, keeper *Keeper) {
	t.Helper()
	ctx := context.Background()

	if err := keeper.Apply(ctx, Update{Why: "mark the anniversary today", Plan: []string{"read the notes", "post it"}}); err != nil {
		t.Fatalf("cannot write the model's half of the record: %v", err)
	}
	for round := range 4 {
		id, err := keeper.AddResult(ctx, fmt.Sprintf("the result of round %d", round+1), fmt.Sprintf("the whole text of r%d", round+1))
		if err != nil {
			t.Fatalf("cannot add the result of round %d: %v", round+1, err)
		}
		if round == 0 {
			if err := keeper.MarkPlanStep(ctx, 1, id); err != nil {
				t.Fatalf("cannot mark the first step done: %v", err)
			}
		}
	}
	if _, err := keeper.AddCorrection(ctx, "no, lead with the date"); err != nil {
		t.Fatalf("cannot add the correction: %v", err)
	}
}

// fileStore is an event log kept as one JSON event per line in a real file. It is
// the smallest thing that lets this test cross a process boundary in spirit: the
// second reader shares nothing with the first but the file itself.
type fileStore struct {
	path  string
	guard sync.Mutex
}

// newFileStore opens the log at a path, making nothing until the first write.
func newFileStore(path string) *fileStore {
	return &fileStore{path: path}
}

// Append writes one event on the end of the file and gives it the next number.
func (store *fileStore) Append(_ context.Context, event contract.Event) (int64, error) {
	store.guard.Lock()
	defer store.guard.Unlock()

	held, err := store.read()
	if err != nil {
		return 0, err
	}
	event.Sequence = int64(len(held) + 1)
	line, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("cannot write event %d as JSON: %w", event.Sequence, err)
	}
	file, err := os.OpenFile(store.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, contract.DataFileMode)
	if err != nil {
		return 0, fmt.Errorf("cannot open the log file %s to write to it: %w", store.path, err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return 0, fmt.Errorf("cannot write event %d to %s: %w", event.Sequence, store.path, err)
	}
	return event.Sequence, nil
}

// ByTask returns every event of one record, in order.
func (store *fileStore) ByTask(_ context.Context, taskID string) ([]contract.Event, error) {
	return store.matching(func(event contract.Event) bool { return event.TaskID == taskID })
}

// ByKind returns every event of one kind, in order.
func (store *fileStore) ByKind(_ context.Context, kind contract.EventKind) ([]contract.Event, error) {
	return store.matching(func(event contract.Event) bool { return event.Kind == kind })
}

// ByID returns one event by its number.
func (store *fileStore) ByID(_ context.Context, sequence int64) (contract.Event, error) {
	held, err := store.matching(func(event contract.Event) bool { return event.Sequence == sequence })
	if err != nil {
		return contract.Event{}, err
	}
	if len(held) == 0 {
		return contract.Event{}, fmt.Errorf("there is no event numbered %d in %s", sequence, store.path)
	}
	return held[0], nil
}

// ByRange returns every event in a span of numbers.
func (store *fileStore) ByRange(_ context.Context, span contract.EventRange) ([]contract.Event, error) {
	return store.matching(func(event contract.Event) bool {
		return event.Sequence >= span.From && event.Sequence <= span.To
	})
}

// Replay hands every event to the function in order.
func (store *fileStore) Replay(_ context.Context, hand func(event contract.Event) error) error {
	held, err := store.read()
	if err != nil {
		return err
	}
	for _, event := range held {
		if err := hand(event); err != nil {
			return fmt.Errorf("the replay stopped at event %d: %w", event.Sequence, err)
		}
	}
	return nil
}

// matching returns every event the test function accepts, in order.
func (store *fileStore) matching(wanted func(event contract.Event) bool) ([]contract.Event, error) {
	held, err := store.read()
	if err != nil {
		return nil, err
	}
	found := []contract.Event{}
	for _, event := range held {
		if wanted(event) {
			found = append(found, event)
		}
	}
	return found, nil
}

// read reads the whole file back, and treats a file that is not there as a log
// with nothing in it yet.
func (store *fileStore) read() ([]contract.Event, error) {
	file, err := os.Open(store.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot open the log file %s to read it: %w", store.path, err)
	}
	defer file.Close()

	held := []contract.Event{}
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for lines.Scan() {
		event := contract.Event{}
		if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("line %d of %s does not read as an event: %w", len(held)+1, store.path, err)
		}
		held = append(held, event)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the log file %s to its end: %w", store.path, err)
	}
	return held, nil
}
