package log

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	// The pure-Go SQLite driver, registered under the name "sqlite", which is
	// what keeps bin/coeus a single static binary with no C compiler and no
	// system library behind it.
	_ "modernc.org/sqlite"
)

// SchemaVersion is the number the schema_version table holds. It goes up by one
// whenever the shape of a table changes, and wave 6's migrations read it to
// decide what has to be done to an older file.
const SchemaVersion = 1

// MaxEventsPerRead is the most events one read returns. A read asks for the
// caller's limit bounded by this number, so that nothing can pull the whole log
// into memory by accident. A caller that needs more walks the log with ByRange
// or with Replay, which streams.
const MaxEventsPerRead = 10000

// The two tables one Coeus log holds. A file with other tables in it belongs to
// another program, and opening it is refused.
const (
	eventsTable  = "events"
	versionTable = "schema_version"
)

// maxReadConnections is how many reads may be in flight at once. Reads are
// short and the file is local, so a handful is plenty.
const maxReadConnections = 4

// databaseOptions are the settings every connection to the file is opened with:
// wait rather than fail when another connection holds the write lock, keep the
// write-ahead file so that readers never block the writer, and let the operating
// system decide when to flush, which is safe under write-ahead mode.
const databaseOptions = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)"

// The statements that make the log's schema. There are two tables and two
// indexes, one index for each read shape that is not the sequence number
// itself, and nothing cleverer than that.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL PRIMARY KEY)`,
	`CREATE TABLE IF NOT EXISTS events (
		sequence INTEGER PRIMARY KEY AUTOINCREMENT,
		occurred TEXT NOT NULL,
		task_id  TEXT NOT NULL,
		kind     TEXT NOT NULL,
		body     BLOB NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS events_by_task ON events (task_id, sequence)`,
	`CREATE INDEX IF NOT EXISTS events_by_kind ON events (kind, sequence)`,
}

// Log is one event log: the append-only events table in one SQLite file, with a
// single connection for writing behind a mutex and a few connections for
// reading. It is what internal/contract calls a Store.
type Log struct {
	path    string
	writing sync.Mutex
	writer  *sql.DB
	reader  *sql.DB
	closed  bool
}

// Open opens the event log in one SQLite file, making the file and its schema
// when they are not there yet. The caller passes the path, which in a running
// agent is the DatabaseFile of the home folder. Opening a file that belongs to
// another program, or one written by a newer Coeus, is an error that names the
// file and says what to do about it.
func Open(ctx context.Context, path string) (*Log, error) {
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("cannot keep the event log at %s, because the database driver reads a question mark in a path as the start of its own options, so choose a path without one", path)
	}

	writer, err := openHandle(path, 1)
	if err != nil {
		return nil, err
	}
	reader, err := openHandle(path, maxReadConnections)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}

	opened := &Log{path: path, writer: writer, reader: reader}
	if err := opened.prepare(ctx); err != nil {
		_ = opened.Close()
		return nil, err
	}
	return opened, nil
}

// Close flushes what the write-ahead file holds back into the database file and
// closes every connection. Closing a log that is already closed does nothing and
// is not an error.
func (eventLog *Log) Close() error {
	eventLog.writing.Lock()
	defer eventLog.writing.Unlock()
	if eventLog.closed {
		return nil
	}
	eventLog.closed = true

	readerErr := eventLog.reader.Close()
	// The checkpoint copies the write-ahead file back into the database file and
	// empties it, so that the file left on disk is complete on its own. It
	// reports a busy result as a row rather than an error, so a reader somewhere
	// else cannot turn closing into a failure.
	_, checkpointErr := eventLog.writer.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	writerErr := eventLog.writer.Close()
	if err := errors.Join(readerErr, checkpointErr, writerErr); err != nil {
		return fmt.Errorf("cannot close the event log at %s: %w", eventLog.path, err)
	}
	return nil
}

// openHandle opens one pool of connections to the file with the settings every
// connection needs. The pool is opened lazily, so a file that cannot be used
// says so at the first query rather than here.
func openHandle(path string, connections int) (*sql.DB, error) {
	handle, err := sql.Open("sqlite", path+databaseOptions)
	if err != nil {
		return nil, fmt.Errorf("cannot open the event log at %s: %w", path, err)
	}
	handle.SetMaxOpenConns(connections)
	handle.SetMaxIdleConns(connections)
	return handle, nil
}

// prepare makes the schema when the file is new and checks it when it is not.
func (eventLog *Log) prepare(ctx context.Context) error {
	eventLog.writing.Lock()
	defer eventLog.writing.Unlock()

	tables, err := eventLog.tableNames(ctx)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return eventLog.createSchema(ctx)
	}
	if !slices.Contains(tables, eventsTable) || !slices.Contains(tables, versionTable) {
		return fmt.Errorf("the file %s is not a Coeus event log, because it holds the tables %s rather than %s and %s, so point Coeus at a different file or move this one aside",
			eventLog.path, strings.Join(tables, ", "), eventsTable, versionTable)
	}
	return eventLog.checkVersion(ctx)
}

// tableNames lists the tables already in the file, leaving out the ones SQLite
// keeps for itself. This is the first query run on the file, so it is also where
// a file that is not a database at all is found out.
func (eventLog *Log) tableNames(ctx context.Context) ([]string, error) {
	rows, err := eventLog.writer.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("cannot read the tables in %s, so it is not a database Coeus can use; point Coeus at a different file or move this one aside: %w", eventLog.path, err)
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		name := ""
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read a table name in %s: %w", eventLog.path, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot finish reading the tables in %s: %w", eventLog.path, err)
	}
	return names, nil
}

// createSchema writes the two tables, the two indexes, and the schema version
// into a file that had nothing in it, all in one transaction so that a crash
// part way through leaves the file empty rather than half made.
func (eventLog *Log) createSchema(ctx context.Context) error {
	transaction, err := eventLog.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start writing the schema into %s: %w", eventLog.path, err)
	}
	defer func() { _ = transaction.Rollback() }()

	for _, statement := range schemaStatements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("cannot make the event log's schema in %s: %w", eventLog.path, err)
		}
	}
	if _, err := transaction.ExecContext(ctx,
		"INSERT OR IGNORE INTO "+versionTable+" (version) VALUES (?)", SchemaVersion); err != nil {
		return fmt.Errorf("cannot write the schema version into %s: %w", eventLog.path, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("cannot finish writing the schema into %s: %w", eventLog.path, err)
	}
	return nil
}

// checkVersion reads the schema version out of a file that already has one and
// refuses a file written by a newer Coeus, which would have tables this version
// does not understand.
func (eventLog *Log) checkVersion(ctx context.Context) error {
	version := 0
	row := eventLog.writer.QueryRowContext(ctx,
		"SELECT version FROM "+versionTable+" ORDER BY version DESC LIMIT 1")
	if err := row.Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("the file %s has an empty schema_version table, so it is not a Coeus event log; point Coeus at a different file or move this one aside", eventLog.path)
		}
		return fmt.Errorf("cannot read the schema version of %s: %w", eventLog.path, err)
	}
	if version > SchemaVersion {
		return fmt.Errorf("the file %s was written by a newer Coeus, whose schema version is %d where this one understands %d, so update Coeus before opening this log", eventLog.path, version, SchemaVersion)
	}
	return nil
}
