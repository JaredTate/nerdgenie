package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// maxMintAttempts is how many numbers the id minter walks past before giving
// up, so that a memory full of hand-written ids cannot make it spin.
const maxMintAttempts = 1000

// maxFactsPerBatch is how many facts one save may carry. A save reads a row of
// the index for every fact in the batch before it writes anything, so a batch
// with no limit is a read with no limit; the biggest batch anything in Coeus
// writes is one finished task's capture, which is MaxCapturedFacts.
const maxFactsPerBatch = 500

// storedFact is one fact ready to be written down, together with the family of
// file it belongs in.
type storedFact struct {
	fact   contract.Fact
	family factFamily
}

// prepareFacts checks and fills in every fact of a batch: the text on one line,
// the date from the clock when there is none, an id minted when there is none,
// and the file the fact belongs in. It refuses the whole batch rather than part
// of it, so that a save either happens or does not.
func (memory *Memory) prepareFacts(ctx context.Context, transaction runner, facts []contract.Fact) ([]storedFact, error) {
	prepared := make([]storedFact, 0, len(facts))
	seen := map[string]bool{}
	for _, fact := range facts {
		one, err := memory.prepareOneFact(ctx, transaction, fact)
		if err != nil {
			return nil, err
		}
		if seen[one.fact.ID] {
			return nil, fmt.Errorf("this batch holds two facts with the id %q, so give every fact its own id", one.fact.ID)
		}
		seen[one.fact.ID] = true
		prepared = append(prepared, one)
	}
	return prepared, nil
}

// prepareOneFact holds the rules one fact has to pass before it is written
// down.
func (memory *Memory) prepareOneFact(ctx context.Context, transaction runner, fact contract.Fact) (storedFact, error) {
	fact.Text = oneLine(fact.Text)
	if fact.Text == "" {
		return storedFact{}, fmt.Errorf("the fact %q has no text, so give every fact something to say", fact.ID)
	}
	if len(fact.Text) > MaxFactTextBytes {
		return storedFact{}, fmt.Errorf("the fact %q is %d bytes and one fact may be at most %d, so write it as a note in the memory folder instead", fact.ID, len(fact.Text), MaxFactTextBytes)
	}
	fact.Source = withoutSeparators(fact.Source)
	if fact.Recorded.IsZero() {
		fact.Recorded = memory.clock.Now()
	}
	fact.Recorded = fact.Recorded.UTC().Truncate(time.Second)
	if !writableDate(fact.Recorded) {
		return storedFact{}, fmt.Errorf("the fact %q is dated %s, and a fact line can only hold a year between one and nine thousand nine hundred and ninety-nine", fact.ID, fact.Recorded)
	}

	family, err := familyOf(ctx, transaction, fact)
	if err != nil {
		return storedFact{}, err
	}
	if fact.ID == "" {
		if fact.ID, err = mintFactID(ctx, transaction, family); err != nil {
			return storedFact{}, err
		}
	}
	if !validFactID(fact.ID) {
		return storedFact{}, fmt.Errorf("the id %q cannot be written on a fact line, so use letters, digits, hyphens, underscores, and full stops", fact.ID)
	}
	taken, err := factIsThere(ctx, transaction, fact.ID)
	if err != nil {
		return storedFact{}, err
	}
	if taken {
		return storedFact{}, fmt.Errorf("memory already holds a fact with the id %q, so supersede it instead of writing over it", fact.ID)
	}
	return storedFact{fact: fact, family: family}, nil
}

// familyOf says which file a fact belongs in: the file the fact it supersedes
// went to, or the one its id points at, which is USER.md for an id beginning
// with the user prefix and MEMORY.md for everything else.
func familyOf(ctx context.Context, transaction runner, fact contract.Fact) (factFamily, error) {
	if fact.Supersedes == "" {
		if strings.HasPrefix(fact.ID, UserFactPrefix) {
			return userFacts, nil
		}
		return worldFacts, nil
	}
	held := ""
	row := transaction.QueryRowContext(ctx, "SELECT family FROM memory_facts WHERE id = ?", fact.Supersedes)
	switch err := row.Scan(&held); {
	case errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("memory holds no fact with the id %q, so a new fact cannot supersede it", fact.Supersedes)
	case err != nil:
		return "", fmt.Errorf("cannot read the fact %q that this one supersedes: %w", fact.Supersedes, err)
	}
	return factFamily(held), nil
}

// factIsThere says whether an id is already used, because an id is never used
// twice: a fact that replaces another supersedes it and keeps its own id.
func factIsThere(ctx context.Context, transaction runner, id string) (bool, error) {
	found := 0
	row := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM memory_facts WHERE id = ?", id)
	if err := row.Scan(&found); err != nil {
		return false, fmt.Errorf("cannot look for a fact with the id %q: %w", id, err)
	}
	return found > 0, nil
}

// mintFactID gives a fact with no id of its own the next free number in its
// family, such as "m7" for a fact about the world and "u3" for one about the
// user.
func mintFactID(ctx context.Context, transaction runner, family factFamily) (string, error) {
	counter := "next " + string(family) + " fact"
	next, err := nextNumber(ctx, transaction, counter)
	if err != nil {
		return "", err
	}
	prefix := "m"
	if family == userFacts {
		prefix = UserFactPrefix
	}
	for attempt := 0; attempt < maxMintAttempts; attempt++ {
		id := prefix + strconv.Itoa(next+attempt)
		taken, err := factIsThere(ctx, transaction, id)
		if err != nil {
			return "", err
		}
		if !taken {
			if err := setNumber(ctx, transaction, counter, next+attempt+1); err != nil {
				return "", err
			}
			return id, nil
		}
	}
	return "", fmt.Errorf("cannot find a free id for a fact about the %s after %d tries, so give the fact an id of its own", family, maxMintAttempts)
}

// writeFacts writes every fact of a batch into the file it belongs in, moving
// the oldest facts out of a file that would pass its cap, and then puts all of
// them into the index. It returns every file it wrote, so that a failure after
// this point can put them back.
func (memory *Memory) writeFacts(ctx context.Context, transaction runner, prepared []storedFact) ([]writtenFile, error) {
	written := []writtenFile{}
	placed := map[string]string{}
	for _, family := range []factFamily{worldFacts, userFacts} {
		batch := factsOfFamily(prepared, family)
		if len(batch) == 0 {
			continue
		}
		files, where, err := memory.writeFamily(ctx, transaction, family, batch)
		written = append(written, files...)
		if err != nil {
			return written, err
		}
		for id, path := range where {
			placed[id] = path
		}
	}
	if err := memory.indexFacts(ctx, transaction, prepared, placed); err != nil {
		return written, err
	}
	return written, nil
}

// factsOfFamily picks the facts of a batch that belong in one of the two files.
func factsOfFamily(prepared []storedFact, family factFamily) []contract.Fact {
	found := []contract.Fact{}
	for _, stored := range prepared {
		if stored.family == family {
			found = append(found, stored.fact)
		}
	}
	return found
}

// writeFamily writes one memory file with its new facts on the end, moving the
// oldest facts into a dated note for as long as the file would be over its cap.
// It returns the files it wrote and where each fact ended up.
func (memory *Memory) writeFamily(ctx context.Context, transaction runner, family factFamily, batch []contract.Fact) ([]writtenFile, map[string]string, error) {
	file := memory.fileOf(family)
	kept, held, err := readFactFile(file.path)
	if err != nil {
		return nil, nil, err
	}

	placed := map[string]string{}
	for _, fact := range batch {
		placed[fact.ID] = memory.insideTheHome(file.path)
	}
	moved, staying := splitAtTheCap(kept, append(held, batch...), file.limit)

	written := []writtenFile{}
	if len(moved) > 0 {
		note, err := memory.moveFactsOut(ctx, transaction, family, moved, placed)
		written = append(written, note)
		if err != nil {
			return written, placed, err
		}
	}
	if renderedSize(kept, staying) > file.limit {
		return written, placed, fmt.Errorf("the memory file %s cannot be brought under its limit of %d bytes, because the lines somebody wrote in it by hand are already longer than that, so shorten them", file.path, file.limit)
	}
	personaFile, err := writeFileAtomically(file.path, renderFactFile(kept, staying))
	written = append(written, personaFile)
	return written, placed, err
}

// moveFactsOut appends the oldest facts of a file to today's dated note and
// records that the note is where they now live.
func (memory *Memory) moveFactsOut(ctx context.Context, transaction runner, family factFamily, moved []contract.Fact, placed map[string]string) (writtenFile, error) {
	file := memory.fileOf(family)
	path, err := memory.datedNote(family)
	if err != nil {
		return writtenFile{}, err
	}
	note, err := appendFactsToNote(path, file.stem, moved)
	if err != nil {
		return note, err
	}
	reference := memory.insideTheHome(path)
	for _, fact := range moved {
		placed[fact.ID] = reference
		if _, err := transaction.ExecContext(ctx,
			"UPDATE memory_facts SET path = ? WHERE id = ?", reference, fact.ID); err != nil {
			return note, fmt.Errorf("cannot write down that the fact %q moved into %s: %w", fact.ID, reference, err)
		}
	}
	return note, nil
}

// splitAtTheCap takes facts off the front of a file, which are the oldest ones
// in it, until what is left fits inside the cap.
func splitAtTheCap(kept []string, facts []contract.Fact, limit int) (moved []contract.Fact, staying []contract.Fact) {
	size := renderedSize(kept, facts)
	taken := 0
	for taken < len(facts) && size > limit {
		size -= len(formatFactLine(facts[taken])) + 1
		taken++
	}
	return facts[:taken], facts[taken:]
}
