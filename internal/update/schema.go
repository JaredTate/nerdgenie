package update

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrDatabaseFromANewerNerdGenie is what CheckSchema refuses with when the file was
// written by a later version. It is a value rather than a sentence alone so
// that the program starting can tell this refusal from any other trouble and
// leave with contract.ExitBadConfiguration, which is the code that stops the
// service manager from starting the same losing program again and again.
var ErrDatabaseFromANewerNerdGenie = errors.New("the database was written by a newer version of Nerd Genie")

// executor is the part of a database handle that writing a row needs, so that
// the same code writes inside a transaction and outside one.
type executor interface {
	// ExecContext runs one statement.
	ExecContext(ctx context.Context, statement string, arguments ...any) (sql.Result, error)
}

// CheckSchema refuses a database written by a newer Nerd Genie and names the version
// to use instead, which is the version that applied the newest migration the
// file has had. A file that is not there yet, and a file this program
// understands, are both fine and say nothing.
//
// This is what an older binary calls before it opens the database for work: it
// refuses loudly and changes nothing, rather than reading tables it has never
// seen.
func CheckSchema(ctx context.Context, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	database, err := openDatabase(path)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	at, err := readSchemaVersion(ctx, database)
	if err != nil {
		return err
	}
	if at <= SchemaVersion() {
		return nil
	}
	return fmt.Errorf("%w: %s is at schema version %d and this Nerd Genie understands %d%s",
		ErrDatabaseFromANewerNerdGenie, path, at, SchemaVersion(), whichVersionToUse(ctx, database, at))
}

// whichVersionToUse is the sentence naming the version that wrote the newer
// schema, or the plain advice to update when the file does not say.
func whichVersionToUse(ctx context.Context, database *sql.DB, at int) string {
	written := ""
	row := database.QueryRowContext(ctx,
		"SELECT applied_by FROM "+migrationsTable+" WHERE version = ? LIMIT 1", at)
	if err := row.Scan(&written); err != nil || written == "" {
		return "; run \"nerdgenie update\" to get a version that understands it"
	}
	return fmt.Sprintf("; use Nerd Genie %s or newer, which is the version that wrote it", written)
}

// readSchemaVersion is the highest version the file says it has been brought up
// to. A file with no version table is not a Nerd Genie database at all.
func readSchemaVersion(ctx context.Context, database *sql.DB) (int, error) {
	version := 0
	row := database.QueryRowContext(ctx, "SELECT max(version) FROM schema_version")
	if err := row.Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("the database has an empty schema version table, so it is not a Nerd Genie database; point Nerd Genie at a different file or move this one aside")
		}
		return 0, fmt.Errorf("the schema version could not be read, so this is not a Nerd Genie database: %w", err)
	}
	return version, nil
}

// recordMigration writes down that the schema has been brought to a version and
// which Nerd Genie did it, in the two tables that together answer "how far has this
// file come and what wrote it".
func recordMigration(ctx context.Context, database executor, migration Migration, version string, now time.Time) error {
	if _, err := database.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+migrationsTable+` (
		version    INTEGER NOT NULL PRIMARY KEY,
		applied_at TEXT NOT NULL,
		applied_by TEXT NOT NULL,
		name       TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("the table of applied migrations could not be made: %w", err)
	}
	if _, err := database.ExecContext(ctx,
		"INSERT OR REPLACE INTO "+migrationsTable+" (version, applied_at, applied_by, name) VALUES (?, ?, ?, ?)",
		migration.To, now.UTC().Format(time.RFC3339Nano), version, migration.Name); err != nil {
		return fmt.Errorf("migration %d could not be written down as applied: %w", migration.To, err)
	}
	if _, err := database.ExecContext(ctx,
		"INSERT OR IGNORE INTO schema_version (version) VALUES (?)", migration.To); err != nil {
		return fmt.Errorf("the new schema version %d could not be written: %w", migration.To, err)
	}
	return nil
}

// openDatabase opens the one SQLite file with the settings every connection to
// it uses. It holds one connection, because a migration is one writer.
func openDatabase(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path+databaseOptions)
	if err != nil {
		return nil, fmt.Errorf("the database %s could not be opened, so check that the home folder is the right one: %w", path, err)
	}
	database.SetMaxOpenConns(1)
	return database, nil
}

// closeDatabase copies the write-ahead file back into the database and closes
// the connection, so that the file left on disk is complete on its own, which is
// what makes putting a backup over it safe.
func closeDatabase(database *sql.DB, path string) error {
	_, checkpointErr := database.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	closeErr := database.Close()
	if err := errors.Join(checkpointErr, closeErr); err != nil {
		return fmt.Errorf("the database %s could not be closed after the migrations: %w", path, err)
	}
	return nil
}
