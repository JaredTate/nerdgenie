// Forward-only numbered migrations follow the update strategy in
// docs/research/09-security-reliability.md section 5, which was read out of
// OpenClaw's doctor at ~/Code/openclaw/src/cli/update-cli/schema-preflight.ts:
// each change is numbered, each runs in a transaction, the database is copied
// first, and an older program refuses a file written by a newer one instead of
// guessing at tables it has never seen. What is added here is that the run
// happens after the new version has come up, so that a link switched back always
// lands on a schema the older program understands.

package update

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/reliability"

	// The pure-Go SQLite driver, registered under the name "sqlite", which is the
	// same one internal/log opens the file with.
	_ "modernc.org/sqlite"
)

// migrationsTable holds one row per migration that has been applied, with the
// version of Coeus that applied it, which is what lets an older binary meeting a
// newer database name the version to go back to.
const migrationsTable = "schema_migrations"

// databaseOptions are what every connection here is opened with: wait for
// another connection's write lock rather than failing at once, and keep the
// write-ahead file, which is how internal/log opens the same file.
const databaseOptions = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

// Migration is one numbered, forward-only change to the one database. It runs
// inside a transaction, so a statement that fails leaves the file as it was.
type Migration struct {
	// To is the schema version the database is at once this has been applied,
	// which is always one more than the migration before it.
	To int
	// Name says what the migration does, in a few words, for the log and for
	// whoever reads the table afterwards.
	Name string
	// Apply makes the change inside a transaction the caller commits.
	Apply func(ctx context.Context, transaction *sql.Tx) error
}

// Migrations is the whole list, in order.
//
// It is empty, because the shape of the tables has not changed since
// internal/log created them at version one. The first change to any table adds a
// Migration here taking the schema to two, and nothing else has to be touched.
func Migrations() []Migration { return nil }

// SchemaVersion is the schema version this program understands: where the
// migrations end, or the version internal/log created when there are none.
func SchemaVersion() int { return highestVersion(Migrations()) }

// highestVersion is where a list of migrations leaves the schema.
func highestVersion(list []Migration) int {
	version := log.SchemaVersion
	for _, migration := range list {
		if migration.To > version {
			version = migration.To
		}
	}
	return version
}

// MigrateSettings is what one migration run needs to know.
type MigrateSettings struct {
	// Home is the folder holding the database and the backups.
	Home contract.Home
	// Clock is what the backup taken first is named after.
	Clock contract.Clock
	// BackupFolder is where that backup goes, and is the home's backups folder
	// when it is empty.
	BackupFolder string
	// Version is the version of Coeus running the migrations, which is written
	// down beside each one.
	Version string
}

// Migrate brings the one database up to the schema this program understands and
// returns how many changes it applied. It takes a backup first, applies each
// migration in a transaction, and puts the backup back if any of them fails, so
// that the database is either where it was or where it should be and never in
// between.
func Migrate(ctx context.Context, settings MigrateSettings) (int, error) {
	return migrateWith(ctx, settings, Migrations())
}

// migrateWith is Migrate against a list of migrations, which is what lets a test
// prove the mechanism with migrations of its own.
func migrateWith(ctx context.Context, settings MigrateSettings, list []Migration) (int, error) {
	if settings.Home.Root == "" {
		return 0, errors.New("the migration needs the home folder, because that is where the database is")
	}
	if settings.Clock == nil {
		return 0, errors.New("the migration needs a clock to name the backup it takes first, so pass clock.System()")
	}
	path := settings.Home.DatabaseFile()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	}
	if err := CheckSchema(ctx, path); err != nil {
		return 0, err
	}

	pending, err := pendingMigrations(ctx, path, list)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	archive, err := reliability.Backup(ctx, reliability.BackupSettings{
		Home: settings.Home, Clock: settings.Clock, Folder: settings.BackupFolder,
	})
	if err != nil {
		return 0, fmt.Errorf("the database could not be backed up, and no migration is run without one first: %w", err)
	}
	return applyPending(ctx, settings, pending, archive)
}

// pendingMigrations is every migration the database has not had yet, in order.
func pendingMigrations(ctx context.Context, path string, list []Migration) ([]Migration, error) {
	database, err := openDatabase(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = database.Close() }()

	at, err := readSchemaVersion(ctx, database)
	if err != nil {
		return nil, err
	}
	pending := []Migration{}
	for _, migration := range list {
		if migration.To > at {
			pending = append(pending, migration)
		}
	}
	return pending, nil
}

// applyPending runs each migration in its own transaction and puts the backup
// back if any of them fails.
func applyPending(ctx context.Context, settings MigrateSettings, pending []Migration, archive string) (int, error) {
	database, err := openDatabase(settings.Home.DatabaseFile())
	if err != nil {
		return 0, err
	}
	applied := 0
	for _, migration := range pending {
		if err = applyOne(ctx, database, migration, settings.Version, settings.Clock.Now()); err != nil {
			break
		}
		applied++
	}
	if closeErr := closeDatabase(database, settings.Home.DatabaseFile()); err == nil {
		err = closeErr
	}
	if err == nil {
		return applied, nil
	}
	return 0, putTheBackupBack(ctx, settings, archive, err)
}

// applyOne runs one migration and writes down that it was applied, both in the
// same transaction, so that a change and the record of it can never disagree.
func applyOne(ctx context.Context, database *sql.DB, migration Migration, version string, now time.Time) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %d, %s, could not be started: %w", migration.To, migration.Name, err)
	}
	defer func() { _ = transaction.Rollback() }()

	if err := migration.Apply(ctx, transaction); err != nil {
		return fmt.Errorf("migration %d, %s, did not work, so the database was left as it was: %w", migration.To, migration.Name, err)
	}
	if err := recordMigration(ctx, transaction, migration, version, now); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("migration %d, %s, could not be finished: %w", migration.To, migration.Name, err)
	}
	return nil
}

// putTheBackupBack restores the database as it was before the run and reports
// both what went wrong and what was done about it.
func putTheBackupBack(ctx context.Context, settings MigrateSettings, archive string, why error) error {
	for _, beside := range []string{"-wal", "-shm"} {
		if err := os.Remove(settings.Home.DatabaseFile() + beside); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%w, and %s could not be cleared away before putting the backup back: %w", why, settings.Home.DatabaseFile()+beside, err)
		}
	}
	restore := reliability.RestoreSettings{
		Home: settings.Home, Archive: archive, OnlyTheDatabase: true, Force: true,
	}
	if err := reliability.Restore(ctx, restore); err != nil {
		return fmt.Errorf("%w, and the backup %s could not be put back either, so put it back by hand with \"coeus restore\": %w", why, archive, err)
	}
	return fmt.Errorf("%w, so the backup %s was put back and the database is as it was", why, archive)
}
