//go:build integration

package update

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aHomeReadyToMigrate is a temporary home with a real event log in it and the
// vault key the backup before a migration is locked with.
func aHomeReadyToMigrate(t *testing.T) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	aDatabaseAtVersion(t, home.DatabaseFile(), log.SchemaVersion, "")

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("making the vault key failed: %v", err)
	}
	if err := os.WriteFile(home.VaultKeyFile(), []byte(identity.String()+"\n"), contract.SecretFileMode); err != nil {
		t.Fatalf("writing the vault key failed: %v", err)
	}
	return home
}

// aMigrationThatAddsATable is a migration of the shape a real one will have: one
// statement in one transaction.
func aMigrationThatAddsATable(to int) Migration {
	return Migration{
		To:   to,
		Name: "add the table this test wanted",
		Apply: func(ctx context.Context, transaction *sql.Tx) error {
			_, err := transaction.ExecContext(ctx, "CREATE TABLE something_new (fact TEXT NOT NULL)")
			return err
		},
	}
}

// aMigrationThatFails is a migration that cannot be applied, which is what has
// to leave the database as it was.
func aMigrationThatFails(to int) Migration {
	return Migration{
		To:   to,
		Name: "a migration that cannot work",
		Apply: func(ctx context.Context, transaction *sql.Tx) error {
			return errors.New("this migration was never going to work")
		},
	}
}

// tableIsThere says whether the database holds a table by that name.
func tableIsThere(t *testing.T, path string, name string) bool {
	t.Helper()
	database, err := openDatabase(path)
	if err != nil {
		t.Fatalf("opening the database failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	found := 0
	row := database.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name)
	if err := row.Scan(&found); err != nil {
		t.Fatalf("reading the tables failed: %v", err)
	}
	return found > 0
}

// versionOfTheDatabase is the schema version the file says it is at.
func versionOfTheDatabase(t *testing.T, path string) int {
	t.Helper()
	database, err := openDatabase(path)
	if err != nil {
		t.Fatalf("opening the database failed: %v", err)
	}
	defer func() { _ = database.Close() }()

	version, err := readSchemaVersion(context.Background(), database)
	if err != nil {
		t.Fatalf("reading the schema version failed: %v", err)
	}
	return version
}

func TestAMigrationIsAppliedAndWritesDownWhoAppliedIt(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	settings := MigrateSettings{Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0"}

	applied, err := migrateWith(context.Background(), settings, []Migration{aMigrationThatAddsATable(log.SchemaVersion + 1)})

	if err != nil {
		t.Fatalf("the migration failed: %v", err)
	}
	if applied != 1 {
		t.Errorf("%d migrations were applied rather than one", applied)
	}
	if !tableIsThere(t, home.DatabaseFile(), "something_new") {
		t.Errorf("the migration did not write its table")
	}
	if version := versionOfTheDatabase(t, home.DatabaseFile()); version != log.SchemaVersion+1 {
		t.Errorf("the database is at schema version %d rather than %d", version, log.SchemaVersion+1)
	}
	if err := CheckSchema(context.Background(), home.DatabaseFile()); err == nil {
		t.Errorf("a database migrated past what this program understands was not refused")
	} else if !strings.Contains(err.Error(), "0.8.0") {
		t.Errorf("the refusal does not name the version that wrote it: %v", err)
	}
}

func TestAMigrationThatFailsPutsTheBackupBackAndChangesNothing(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	settings := MigrateSettings{Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0"}
	list := []Migration{
		aMigrationThatAddsATable(log.SchemaVersion + 1),
		aMigrationThatFails(log.SchemaVersion + 2),
	}

	applied, err := migrateWith(context.Background(), settings, list)

	if err == nil {
		t.Fatalf("a migration that cannot work said it worked")
	}
	if applied != 0 {
		t.Errorf("the report says %d migrations were applied after the run was put back", applied)
	}
	if !strings.Contains(err.Error(), "put back") {
		t.Errorf("the report does not say that the backup was put back: %v", err)
	}
	if version := versionOfTheDatabase(t, home.DatabaseFile()); version != log.SchemaVersion {
		t.Errorf("the database is at schema version %d rather than back at %d", version, log.SchemaVersion)
	}
	if tableIsThere(t, home.DatabaseFile(), "something_new") {
		t.Errorf("the table the first migration wrote is still there after the backup was put back")
	}
}

func TestAMigrationRunLeavesTheEventsWhereTheyWere(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	opened, err := log.Open(context.Background(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log failed: %v", err)
	}
	if _, err := opened.Append(context.Background(), contract.Event{
		Occurred: theTestMoment, TaskID: "t1", Kind: contract.EventMessage, Body: []byte(`{"text":"hello"}`),
	}); err != nil {
		t.Fatalf("writing an event failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}
	settings := MigrateSettings{Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0"}

	if _, err := migrateWith(context.Background(), settings, []Migration{aMigrationThatAddsATable(log.SchemaVersion + 1)}); err != nil {
		t.Fatalf("the migration failed: %v", err)
	}

	database, err := openDatabase(home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the database failed: %v", err)
	}
	defer func() { _ = database.Close() }()
	events := 0
	if err := database.QueryRow("SELECT count(*) FROM events").Scan(&events); err != nil {
		t.Fatalf("counting the events failed: %v", err)
	}
	if events != 1 {
		t.Errorf("the log holds %d events after the migration rather than the one written before it", events)
	}
}

func TestAMigrationNeedsAClockAndAHome(t *testing.T) {
	home := aHomeReadyToMigrate(t)

	if _, err := Migrate(context.Background(), MigrateSettings{Home: home}); err == nil {
		t.Errorf("a migration with no clock was run")
	}
	if _, err := Migrate(context.Background(), MigrateSettings{Clock: testkit.NewFakeClock(theTestMoment)}); err == nil {
		t.Errorf("a migration with no home folder was run")
	}
}

func TestAMigrationRunOnADatabaseFromANewerCoeusIsRefused(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	aDatabaseAtVersion(t, home.DatabaseFile(), SchemaVersion()+1, "0.9.0")

	_, err := Migrate(context.Background(), MigrateSettings{
		Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.7.0",
	})

	if err == nil {
		t.Fatalf("a database from a newer Coeus was migrated backwards")
	}
	if !strings.Contains(err.Error(), "0.9.0") {
		t.Errorf("the refusal does not name the version to use: %v", err)
	}
}

func TestABackupIsWrittenBeforeTheMigrationsRun(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	settings := MigrateSettings{Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0"}

	if _, err := migrateWith(context.Background(), settings, []Migration{aMigrationThatAddsATable(log.SchemaVersion + 1)}); err != nil {
		t.Fatalf("the migration failed: %v", err)
	}

	archives, err := os.ReadDir(home.BackupsFolder())
	if err != nil {
		t.Fatalf("reading the backups folder failed: %v", err)
	}
	if len(archives) != 1 {
		t.Errorf("the backups folder holds %d archives rather than the one taken before the migration", len(archives))
	}
	if len(archives) == 1 && !strings.HasSuffix(archives[0].Name(), ".tar.age") {
		t.Errorf("the file in the backups folder is %q, which is not an archive", archives[0].Name())
	}
	if _, err := os.Stat(filepath.Join(home.BackupsFolder(), archives[0].Name())); err != nil {
		t.Errorf("the archive is not readable: %v", err)
	}
}
