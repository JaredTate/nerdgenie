// The index over the memory files and every past message is the design of
// Hermes' session search at ~/Code/hermes-agent/tools/session_search_tool.py,
// written fresh in Go: one full-text search table in the same SQLite file the
// rest of the state lives in, ranked by the search table's own relevance
// measure, with no model call anywhere in it. What Coeus adds is that the facts
// and the notes ride in the same table as the messages, so that one search
// covers everything the agent knows.

package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The three kinds of thing the index holds, which is also what the id of a
// search result says it is.
const (
	// factEntry is one fact from one of the two memory files or a dated note.
	factEntry = "fact"
	// noteEntry is one markdown note in the memory folder, or the lines
	// somebody wrote by hand at the top of a memory file.
	noteEntry = "note"
	// messageEntry is one message from the event log.
	messageEntry = "message"
)

// The prefixes that say what a search result is when it is not a fact, so that
// one id space covers the facts, the notes, and the past messages.
const (
	// noteIDPrefix begins the id of a search result that is a note, and the
	// rest of the id is the note's path inside the home folder.
	noteIDPrefix = "note:"
	// messageIDPrefix begins the id of a search result that is a past message,
	// and the rest of the id is that message's number in the event log.
	messageIDPrefix = "msg:"
)

// The four tables this package owns inside the one SQLite file. internal/log
// owns the events table and the schema version, and nothing here touches them.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS memory_facts (
		id            TEXT PRIMARY KEY,
		text          TEXT NOT NULL,
		source        TEXT NOT NULL,
		recorded      TEXT NOT NULL,
		supersedes    TEXT NOT NULL,
		superseded_by TEXT NOT NULL,
		family        TEXT NOT NULL,
		path          TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS memory_facts_by_time ON memory_facts (recorded)`,
	`CREATE TABLE IF NOT EXISTS memory_state (
		name  TEXT NOT NULL PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS memory_indexed (
		kind        TEXT NOT NULL,
		reference   TEXT NOT NULL,
		row_id      INTEGER NOT NULL,
		fingerprint TEXT NOT NULL,
		recorded    TEXT NOT NULL,
		source      TEXT NOT NULL,
		PRIMARY KEY (kind, reference)
	)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS memory_search USING fts5(body, tokenize='unicode61')`,
}

// runner is what a statement runs on: the database itself, or one transaction
// inside it, so that the same code writes a fact either way.
type runner interface {
	// ExecContext runs a statement that returns no rows.
	ExecContext(ctx context.Context, statement string, arguments ...any) (sql.Result, error)
	// QueryContext runs a statement that returns rows.
	QueryContext(ctx context.Context, statement string, arguments ...any) (*sql.Rows, error)
	// QueryRowContext runs a statement that returns at most one row.
	QueryRowContext(ctx context.Context, statement string, arguments ...any) *sql.Row
}

// createTables makes this package's tables when they are not there yet, in one
// transaction, so that a crash part way through leaves none of them.
func (memory *Memory) createTables(ctx context.Context) error {
	transaction, err := memory.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start making the memory index in %s: %w", memory.home.DatabaseFile(), err)
	}
	defer func() { _ = transaction.Rollback() }()

	for _, statement := range schemaStatements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("cannot make the memory index in %s: %w", memory.home.DatabaseFile(), err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("cannot finish making the memory index in %s: %w", memory.home.DatabaseFile(), err)
	}
	return nil
}

// indexFacts writes a batch of prepared facts into the fact table and the
// search table, and marks every fact they supersede as superseded.
func (memory *Memory) indexFacts(ctx context.Context, transaction runner, prepared []storedFact, placed map[string]string) error {
	for _, stored := range prepared {
		fact := stored.fact
		_, err := transaction.ExecContext(ctx,
			`INSERT INTO memory_facts (id, text, source, recorded, supersedes, superseded_by, family, path)
			 VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
			fact.ID, fact.Text, fact.Source, asStoredTime(fact.Recorded), fact.Supersedes,
			string(stored.family), placed[fact.ID])
		if err != nil {
			return fmt.Errorf("cannot write the fact %q into the memory index: %w", fact.ID, err)
		}
		if fact.Supersedes != "" {
			if _, err := transaction.ExecContext(ctx,
				"UPDATE memory_facts SET superseded_by = ? WHERE id = ?", fact.ID, fact.Supersedes); err != nil {
				return fmt.Errorf("cannot mark the fact %q as superseded by %q: %w", fact.Supersedes, fact.ID, err)
			}
		}
		if err := indexOneBody(ctx, transaction, indexEntry{
			kind:      factEntry,
			reference: fact.ID,
			source:    fact.Source,
			recorded:  fact.Recorded,
			body:      fact.Text + " " + fact.Source,
		}); err != nil {
			return err
		}
	}
	return nil
}

// indexEntry is one thing the search table holds: what kind of thing it is,
// what it points at, and the words that are searched.
type indexEntry struct {
	kind      string
	reference string
	source    string
	recorded  time.Time
	body      string
}

// indexOneBody puts one entry into the search table, or brings it up to date
// when its words have changed, and does nothing at all when they have not.
func indexOneBody(ctx context.Context, transaction runner, entry indexEntry) error {
	digest := sha256.Sum256([]byte(entry.body))
	fingerprint := hex.EncodeToString(digest[:])

	rowID := int64(0)
	held := ""
	row := transaction.QueryRowContext(ctx,
		"SELECT row_id, fingerprint FROM memory_indexed WHERE kind = ? AND reference = ?",
		entry.kind, entry.reference)
	switch err := row.Scan(&rowID, &held); {
	case errors.Is(err, sql.ErrNoRows):
		return insertIndexEntry(ctx, transaction, entry, fingerprint)
	case err != nil:
		return fmt.Errorf("cannot look for the %s %q in the memory index: %w", entry.kind, entry.reference, err)
	case held == fingerprint:
		return nil
	}
	return updateIndexEntry(ctx, transaction, entry, fingerprint, rowID)
}

// insertIndexEntry adds an entry the index did not hold before.
func insertIndexEntry(ctx context.Context, transaction runner, entry indexEntry, fingerprint string) error {
	result, err := transaction.ExecContext(ctx, "INSERT INTO memory_search (body) VALUES (?)", entry.body)
	if err != nil {
		return fmt.Errorf("cannot put the %s %q into the memory search table: %w", entry.kind, entry.reference, err)
	}
	rowID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("cannot find the row the %s %q was written into: %w", entry.kind, entry.reference, err)
	}
	if _, err := transaction.ExecContext(ctx,
		`INSERT INTO memory_indexed (kind, reference, row_id, fingerprint, recorded, source)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.kind, entry.reference, rowID, fingerprint, asStoredTime(entry.recorded), entry.source); err != nil {
		return fmt.Errorf("cannot write down that the %s %q is indexed: %w", entry.kind, entry.reference, err)
	}
	return nil
}

// updateIndexEntry replaces the words of an entry whose file has changed.
func updateIndexEntry(ctx context.Context, transaction runner, entry indexEntry, fingerprint string, rowID int64) error {
	if _, err := transaction.ExecContext(ctx,
		"UPDATE memory_search SET body = ? WHERE rowid = ?", entry.body, rowID); err != nil {
		return fmt.Errorf("cannot bring the %s %q up to date in the memory search table: %w", entry.kind, entry.reference, err)
	}
	if _, err := transaction.ExecContext(ctx,
		"UPDATE memory_indexed SET fingerprint = ?, recorded = ?, source = ? WHERE kind = ? AND reference = ?",
		fingerprint, asStoredTime(entry.recorded), entry.source, entry.kind, entry.reference); err != nil {
		return fmt.Errorf("cannot write down that the %s %q was brought up to date: %w", entry.kind, entry.reference, err)
	}
	return nil
}

// forgetIndexEntry takes an entry out of the index, which happens when the file
// behind it is no longer in the memory folder.
func forgetIndexEntry(ctx context.Context, transaction runner, kind string, reference string) error {
	rowID := int64(0)
	row := transaction.QueryRowContext(ctx,
		"SELECT row_id FROM memory_indexed WHERE kind = ? AND reference = ?", kind, reference)
	switch err := row.Scan(&rowID); {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("cannot look for the %s %q in the memory index: %w", kind, reference, err)
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM memory_search WHERE rowid = ?", rowID); err != nil {
		return fmt.Errorf("cannot take the %s %q out of the memory search table: %w", kind, reference, err)
	}
	if _, err := transaction.ExecContext(ctx,
		"DELETE FROM memory_indexed WHERE kind = ? AND reference = ?", kind, reference); err != nil {
		return fmt.Errorf("cannot take the %s %q out of the memory index: %w", kind, reference, err)
	}
	return nil
}

// asStoredTime writes a moment the way every time in this index is written: in
// universal time, to the second, in the form that sorts as text.
func asStoredTime(moment time.Time) string {
	return moment.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// fromStoredTime reads a moment back, and reads an unreadable one as the zero
// time rather than failing a whole search over one bad row.
func fromStoredTime(held string) time.Time {
	moment, err := time.Parse(time.RFC3339, held)
	if err != nil {
		return time.Time{}
	}
	return moment.UTC()
}

// nextNumber reads one of the counters in the state table, which is one when
// the counter has never been used.
func nextNumber(ctx context.Context, transaction runner, counter string) (int, error) {
	held := ""
	row := transaction.QueryRowContext(ctx, "SELECT value FROM memory_state WHERE name = ?", counter)
	switch err := row.Scan(&held); {
	case errors.Is(err, sql.ErrNoRows):
		return 1, nil
	case err != nil:
		return 0, fmt.Errorf("cannot read the memory counter %q: %w", counter, err)
	}
	number, err := strconv.Atoi(held)
	if err != nil || number < 1 {
		return 1, nil
	}
	return number, nil
}

// setNumber writes one of the counters in the state table.
func setNumber(ctx context.Context, transaction runner, counter string, number int) error {
	if _, err := transaction.ExecContext(ctx,
		`INSERT INTO memory_state (name, value) VALUES (?, ?)
		 ON CONFLICT (name) DO UPDATE SET value = excluded.value`,
		counter, strconv.Itoa(number)); err != nil {
		return fmt.Errorf("cannot write the memory counter %q: %w", counter, err)
	}
	return nil
}

// factRow reads one fact out of the index exactly as it was written down, with
// no mark on it saying whether something later replaced it.
func factRow(ctx context.Context, transaction runner, id string) (contract.Fact, string, error) {
	fact := contract.Fact{ID: id}
	recorded, supersededBy := "", ""
	row := transaction.QueryRowContext(ctx,
		"SELECT text, source, recorded, supersedes, superseded_by FROM memory_facts WHERE id = ?", id)
	switch err := row.Scan(&fact.Text, &fact.Source, &recorded, &fact.Supersedes, &supersededBy); {
	case errors.Is(err, sql.ErrNoRows):
		return contract.Fact{}, "", fmt.Errorf("memory holds no fact with the id %q, so search for it instead", id)
	case err != nil:
		return contract.Fact{}, "", fmt.Errorf("cannot read the fact %q out of the memory index: %w", id, err)
	}
	fact.Recorded = fromStoredTime(recorded)
	return fact, supersededBy, nil
}
