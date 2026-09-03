package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"

	// The pure-Go SQLite driver, registered under the name "sqlite". It is the
	// same driver internal/log opens the one database file with, and it carries
	// the full-text search tables this package's index is built on.
	_ "modernc.org/sqlite"
)

// maxConnections is how many connections to the database file this package
// keeps. Memory reads far more often than it writes, and a handful is plenty.
const maxConnections = 4

// databaseOptions are the settings every connection is opened with, and they
// are the ones internal/log uses, because both open the same file: wait rather
// than fail when somebody else holds the write lock, keep the write-ahead file
// so readers never block the writer, and let the operating system decide when
// to flush.
const databaseOptions = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)"

// Memory is the agent's memory across tasks: the two fact files under the
// persona folder, the notes in the memory folder, and the index over all of it
// and every past message. It is what internal/contract calls a Memory.
type Memory struct {
	home     contract.Home
	eventLog contract.Store
	clock    contract.Clock
	caps     contract.MemoryCaps
	database *sql.DB
	writing  sync.Mutex
	closed   bool
}

// A memory is the one real memory, so the compiler is asked to say at once if
// it ever stops matching the contract every other package writes against.
var _ contract.Memory = (*Memory)(nil)

// Open opens the memory in a home folder: it makes the persona and memory
// folders when they are not there, creates its own tables in the one SQLite
// file, and brings the index up to date once, inside its per-run cap. The event
// log must already have been opened on the same file, because internal/log owns
// that file's identity, and opening a database that is not a Coeus event log is
// refused with an error saying so.
func Open(ctx context.Context, home contract.Home, eventLog contract.Store, clock contract.Clock, caps contract.MemoryCaps) (*Memory, error) {
	if eventLog == nil {
		return nil, errors.New("cannot open the memory without an event log, so open internal/log first and pass it in")
	}
	if clock == nil {
		return nil, errors.New("cannot open the memory without a clock, so pass the clock from internal/clock or a test clock")
	}
	if caps.WorldFactsBytes <= 0 || caps.UserFactsBytes <= 0 {
		return nil, fmt.Errorf("cannot open the memory with the file limits %d and %d, because both have to be above zero", caps.WorldFactsBytes, caps.UserFactsBytes)
	}
	path := home.DatabaseFile()
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("cannot keep the memory index in %s, because the database driver reads a question mark in a path as the start of its own options, so choose a path without one", path)
	}

	database, err := sql.Open("sqlite", path+databaseOptions)
	if err != nil {
		return nil, fmt.Errorf("cannot open the memory index in %s: %w", path, err)
	}
	database.SetMaxOpenConns(maxConnections)
	database.SetMaxIdleConns(maxConnections)

	opened := &Memory{home: home, eventLog: eventLog, clock: clock, caps: caps, database: database}
	if err := opened.prepare(ctx); err != nil {
		_ = opened.Close()
		return nil, err
	}
	if err := opened.rebuild(ctx); err != nil {
		_ = opened.Close()
		return nil, err
	}
	return opened, nil
}

// Close lets go of every connection to the database file. Closing a memory that
// is already closed does nothing and is not an error.
func (memory *Memory) Close() error {
	memory.writing.Lock()
	defer memory.writing.Unlock()
	if memory.closed {
		return nil
	}
	memory.closed = true
	if err := memory.database.Close(); err != nil {
		return fmt.Errorf("cannot close the memory index in %s: %w", memory.home.DatabaseFile(), err)
	}
	return nil
}

// prepare makes the folders and the tables this package owns, after checking
// that the file really is the event log's file.
func (memory *Memory) prepare(ctx context.Context) error {
	for _, folder := range []string{memory.home.PersonaFolder(), memory.home.MemoryFolder()} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			return fmt.Errorf("cannot make the folder %s the memory keeps its files in: %w", folder, err)
		}
	}
	if err := memory.checkTheEventLogIsThere(ctx); err != nil {
		return err
	}
	return memory.createTables(ctx)
}

// checkTheEventLogIsThere refuses a database file the event log has not made
// its tables in, because internal/log decides what file this is and refuses a
// file holding tables it does not know.
func (memory *Memory) checkTheEventLogIsThere(ctx context.Context) error {
	name := ""
	row := memory.database.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'events'")
	switch err := row.Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("the file %s is not a Coeus event log yet, so open internal/log on it before the memory", memory.home.DatabaseFile())
	case err != nil:
		return fmt.Errorf("cannot read the tables in %s to check that it is a Coeus event log: %w", memory.home.DatabaseFile(), err)
	default:
		return nil
	}
}

// Save writes a batch of facts in one step: either every fact reaches both the
// files and the index, or none of them does and nothing on disk has changed.
func (memory *Memory) Save(ctx context.Context, facts []contract.Fact) error {
	if len(facts) == 0 {
		return nil
	}
	memory.writing.Lock()
	defer memory.writing.Unlock()
	return memory.saveBatch(ctx, facts)
}

// saveBatch does the work of Save with the write lock already held: one
// database transaction, the files written inside it, and the commit last, so
// that a file that cannot be written rolls the whole batch back.
func (memory *Memory) saveBatch(ctx context.Context, facts []contract.Fact) error {
	transaction, err := memory.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start writing facts into the memory index: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()

	prepared, err := memory.prepareFacts(ctx, transaction, facts)
	if err != nil {
		return err
	}
	written, err := memory.writeFacts(ctx, transaction, prepared)
	if err != nil {
		putFilesBack(written)
		return err
	}
	if err := transaction.Commit(); err != nil {
		putFilesBack(written)
		return fmt.Errorf("cannot finish writing facts into the memory index: %w", err)
	}
	memory.logFileChanges(ctx, written)
	return nil
}

// logFileChanges writes down every memory file this save wrote, with what the
// file held before, which is what an undo puts back. It runs after the commit,
// so the log never claims a change that was rolled back. A log that cannot be
// written is not worth failing a save that has already happened, so the error
// is folded into the file change's own record and nothing more.
func (memory *Memory) logFileChanges(ctx context.Context, written []writtenFile) {
	for _, file := range written {
		body, err := file.asEventBody()
		if err != nil {
			continue
		}
		_, _ = memory.eventLog.Append(ctx, contract.Event{
			Occurred: memory.clock.Now().UTC(),
			Kind:     contract.EventFileChange,
			Body:     body,
		})
	}
}
