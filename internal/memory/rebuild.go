package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxIndexedPerRun is how many files and messages one run of the indexer takes
// in. A very large log or a very full memory folder is caught up over several
// runs rather than in one that never ends.
const MaxIndexedPerRun = 2000

// indexedThroughCounter names the state row holding the number of the last
// event the indexer has read, so that a later run carries on from there.
const indexedThroughCounter = "indexed through event"

// runBudget is how much work one run of the indexer may do.
type runBudget struct {
	left int
}

// spend takes one piece of work out of the budget and says whether there was
// any left to take.
func (budget *runBudget) spend() bool {
	if budget.left <= 0 {
		return false
	}
	budget.left--
	return true
}

// rebuild brings the index up to date with what is on disk and in the event
// log, inside one run's budget. It runs once when the memory is opened.
func (memory *Memory) rebuild(ctx context.Context) error {
	memory.writing.Lock()
	defer memory.writing.Unlock()

	budget := &runBudget{left: MaxIndexedPerRun}
	if err := memory.indexTheFactFiles(ctx, budget); err != nil {
		return err
	}
	if err := memory.indexTheNotes(ctx, budget); err != nil {
		return err
	}
	return memory.indexTheMessages(ctx, budget)
}

// indexTheFactFiles reads the two memory files back off disk, so that a fact
// somebody added by hand, or one written just before a crash, is in the index
// too. The lines somebody wrote by hand are indexed as a note of their own.
func (memory *Memory) indexTheFactFiles(ctx context.Context, budget *runBudget) error {
	for _, family := range []factFamily{worldFacts, userFacts} {
		file := memory.fileOf(family)
		kept, facts, err := readFactFile(file.path)
		if err != nil {
			return err
		}
		reference := memory.insideTheHome(file.path)
		if err := memory.recoverFacts(ctx, facts, family, reference, budget); err != nil {
			return err
		}
		if len(kept) == 0 {
			if err := forgetIndexEntry(ctx, memory.database, noteEntry, reference); err != nil {
				return err
			}
			continue
		}
		if err := memory.indexNoteText(ctx, reference, strings.Join(kept, "\n"), file.path, budget); err != nil {
			return err
		}
	}
	return nil
}

// indexTheNotes walks the memory folder and indexes every markdown file in it,
// reading a dated note the overflow wrote as the facts inside it rather than as
// one block of text, and forgets the notes whose files are gone.
func (memory *Memory) indexTheNotes(ctx context.Context, budget *runBudget) error {
	folder := memory.home.MemoryFolder()
	walkErr := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		if movedFactsNote(entry.Name()) {
			return memory.indexMovedFactsNote(ctx, path, entry.Name(), budget)
		}
		held, readErr := readWholeFile(path)
		if readErr != nil {
			return nil
		}
		return memory.indexNoteText(ctx, memory.insideTheHome(path), string(held), path, budget)
	})
	if walkErr != nil {
		return fmt.Errorf("cannot walk the memory folder %s to index it: %w", folder, walkErr)
	}
	return memory.forgetMissingNotes(ctx)
}

// indexMovedFactsNote reads the facts back out of a dated note the overflow
// wrote, so that a fact that has left a memory file is still in the index.
func (memory *Memory) indexMovedFactsNote(ctx context.Context, path string, name string, budget *runBudget) error {
	family := worldFacts
	if strings.HasPrefix(name, "USER-") {
		family = userFacts
	}
	_, facts, err := readFactFile(path)
	if err != nil {
		return nil
	}
	return memory.recoverFacts(ctx, facts, family, memory.insideTheHome(path), budget)
}

// recoverFacts puts into the index every fact that is written in a file but not
// in the fact table, which is how the files on disk stay the truth.
func (memory *Memory) recoverFacts(ctx context.Context, facts []contract.Fact, family factFamily, reference string, budget *runBudget) error {
	for _, fact := range facts {
		there, err := factIsThere(ctx, memory.database, fact.ID)
		if err != nil {
			return err
		}
		if there || !budget.spend() {
			continue
		}
		prepared := []storedFact{{fact: fact, family: family}}
		if err := memory.indexFacts(ctx, memory.database, prepared, map[string]string{fact.ID: reference}); err != nil {
			return err
		}
	}
	return nil
}

// indexNoteText puts one note's words into the index, dated by when the file
// was last written.
func (memory *Memory) indexNoteText(ctx context.Context, reference string, body string, path string, budget *runBudget) error {
	if strings.TrimSpace(body) == "" || !budget.spend() {
		return nil
	}
	entry := indexEntry{kind: noteEntry, reference: reference, source: reference, body: body}
	if info, err := os.Stat(path); err == nil {
		entry.recorded = info.ModTime()
	}
	return indexOneBody(ctx, memory.database, entry)
}

// forgetMissingNotes takes out of the index every note whose file is no longer
// on disk, so that a search never offers something the user has deleted.
func (memory *Memory) forgetMissingNotes(ctx context.Context) error {
	rows, err := memory.database.QueryContext(ctx,
		"SELECT reference FROM memory_indexed WHERE kind = ?", noteEntry)
	if err != nil {
		return fmt.Errorf("cannot list the notes the memory index holds: %w", err)
	}
	gone := []string{}
	for rows.Next() {
		reference := ""
		if err := rows.Scan(&reference); err != nil {
			rows.Close()
			return fmt.Errorf("cannot read the name of an indexed note: %w", err)
		}
		path, err := memory.pathInsideTheHome(reference)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			gone = append(gone, reference)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot finish listing the notes the memory index holds: %w", err)
	}
	for _, reference := range gone {
		if err := forgetIndexEntry(ctx, memory.database, noteEntry, reference); err != nil {
			return err
		}
	}
	return nil
}

// indexTheMessages reads the event log from where the last run stopped and puts
// every message into the index, inside this run's budget.
func (memory *Memory) indexTheMessages(ctx context.Context, budget *runBudget) error {
	from, err := nextNumber(ctx, memory.database, indexedThroughCounter)
	if err != nil {
		return err
	}
	already := int64(from - 1)
	reached := already
	outOfBudget := errors.New("this run of the memory indexer has used up its budget")

	replayErr := memory.eventLog.Replay(ctx, func(event contract.Event) error {
		if event.Sequence <= already {
			return nil
		}
		if event.Kind == contract.EventMessage {
			if !budget.spend() {
				return outOfBudget
			}
			if err := memory.indexMessage(ctx, event); err != nil {
				return err
			}
		}
		reached = event.Sequence
		return nil
	})
	if replayErr != nil && !errors.Is(replayErr, outOfBudget) {
		return fmt.Errorf("cannot read the event log to index the messages in it: %w", replayErr)
	}
	return setNumber(ctx, memory.database, indexedThroughCounter, int(reached)+1)
}

// IndexEvent puts one new event into the index, which is what the loop calls
// when it writes a message into the event log. Anything that is not a message
// is nothing memory searches, and is quietly passed over.
func (memory *Memory) IndexEvent(ctx context.Context, event contract.Event) error {
	if event.Kind != contract.EventMessage {
		return nil
	}
	memory.writing.Lock()
	defer memory.writing.Unlock()

	if err := memory.indexMessage(ctx, event); err != nil {
		return err
	}
	from, err := nextNumber(ctx, memory.database, indexedThroughCounter)
	if err != nil {
		return err
	}
	if int64(from) > event.Sequence {
		return nil
	}
	return setNumber(ctx, memory.database, indexedThroughCounter, int(event.Sequence)+1)
}

// indexMessage puts what one message said into the index, under an id built
// from its number in the event log. An event whose body this package cannot
// read is passed over rather than failing the whole run, because the log holds
// events written by every other package too.
func (memory *Memory) indexMessage(ctx context.Context, event contract.Event) error {
	body := messageBody{}
	if err := json.Unmarshal(event.Body, &body); err != nil {
		return nil
	}
	text := oneLine(body.Text)
	if text == "" {
		return nil
	}
	source := "a past message"
	if event.TaskID != "" {
		source = "a message in task " + event.TaskID
	}
	return indexOneBody(ctx, memory.database, indexEntry{
		kind:      messageEntry,
		reference: strconv.FormatInt(event.Sequence, 10),
		source:    source,
		recorded:  event.Occurred,
		body:      text,
	})
}
