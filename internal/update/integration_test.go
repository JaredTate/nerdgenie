//go:build integration

package update

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aHomeReadyToMigrate is a temporary home with a real event log in it and the
// vault key the backup before a migration is locked with. It sits under a short
// path, because the agent that holds the database open during a migration is
// found on a Unix socket in the run folder, and a socket path may be far shorter
// than a file path.
func aHomeReadyToMigrate(t *testing.T) contract.Home {
	t.Helper()
	home := aHomeUnderAShortPath(t)
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

// aHomeUnderAShortPath makes the whole home folder layout under a short path of
// this test's own, so that the socket in its run folder has a name the operating
// system will take.
func aHomeUnderAShortPath(t *testing.T) contract.Home {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-migrate")
	if err != nil {
		t.Fatalf("cannot make a folder for the home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })
	t.Setenv("HOME", folder)

	home := contract.NewHome(filepath.Join(folder, contract.HomeFolderName))
	for _, one := range home.Folders() {
		if err := os.MkdirAll(one, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", one, err)
		}
	}
	return home
}

// aRunningAgent is what makes putting a backup back over the database
// destructive: a program holding the one database open and answering the
// readiness check on the home's socket, which is where "coeus serve" is during
// the migration step of an update.
type aRunningAgent struct {
	// eventLog is the agent's own open handle on the database.
	eventLog *log.Log
	// listener is the socket the readiness check finds it on.
	listener net.Listener
	// once makes sure the agent goes away only the first time it is stopped.
	once sync.Once
}

// anAgentHoldingTheDatabase starts one and stops it again when the test ends.
func anAgentHoldingTheDatabase(t *testing.T, home contract.Home) *aRunningAgent {
	t.Helper()
	opened, err := log.Open(context.Background(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log the agent holds failed: %v", err)
	}
	listener, err := net.Listen("unix", home.SocketFile())
	if err != nil {
		t.Fatalf("listening on the agent's socket at %s failed: %v", home.SocketFile(), err)
	}
	agent := &aRunningAgent{eventLog: opened, listener: listener}
	t.Cleanup(agent.stop)
	go agent.answer()
	return agent
}

// answer replies to whatever is asked of it, which is what the readiness check
// looks for.
func (agent *aRunningAgent) answer() {
	for {
		connection, err := agent.listener.Accept()
		if err != nil {
			return
		}
		_ = contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
			Type: contract.SocketReply, Text: "ready",
		})
		_ = connection.Close()
	}
}

// stop closes the socket and lets go of the database, which is what the service
// manager's stop does to the real agent.
func (agent *aRunningAgent) stop() {
	agent.once.Do(func() {
		_ = agent.listener.Close()
		_ = agent.eventLog.Close()
	})
}

func TestTheAgentIsStoppedBeforeTheBackupIsPutBack(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	agent := anAgentHoldingTheDatabase(t, home)
	stopped := 0
	settings := MigrateSettings{
		Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0",
		StopTheAgent: func(context.Context) error {
			stopped++
			agent.stop()
			return nil
		},
	}
	list := []Migration{
		aMigrationThatAddsATable(log.SchemaVersion + 1),
		aMigrationThatFails(log.SchemaVersion + 2),
	}

	if _, err := migrateWith(context.Background(), settings, list); err == nil {
		t.Fatalf("a migration that cannot work said it worked")
	}

	if stopped != 1 {
		t.Errorf("the agent was stopped %d times before the backup was put back rather than once", stopped)
	}
	if version := versionOfTheDatabase(t, home.DatabaseFile()); version != log.SchemaVersion {
		t.Errorf("the database is at schema version %d rather than back at %d", version, log.SchemaVersion)
	}
	if tableIsThere(t, home.DatabaseFile(), "something_new") {
		t.Errorf("the table the first migration wrote is still there after the backup was put back")
	}
}

func TestABackupIsNotPutBackWhileTheAgentStillHasTheDatabaseOpen(t *testing.T) {
	home := aHomeReadyToMigrate(t)
	anAgentHoldingTheDatabase(t, home)
	settings := MigrateSettings{
		Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0",
		StopTheAgent: func(context.Context) error { return nil },
	}
	list := []Migration{
		aMigrationThatAddsATable(log.SchemaVersion + 1),
		aMigrationThatFails(log.SchemaVersion + 2),
	}

	_, err := migrateWith(context.Background(), settings, list)

	if err == nil {
		t.Fatalf("a migration that cannot work said it worked")
	}
	if !strings.Contains(err.Error(), "still answering") {
		t.Errorf("the report does not say that the agent is still holding the database: %v", err)
	}
	if _, statErr := os.Stat(home.DatabaseFile() + "-wal"); statErr != nil {
		t.Errorf("the write-ahead file the live agent is writing into was cleared away: %v", statErr)
	}
	if !tableIsThere(t, home.DatabaseFile(), "something_new") {
		t.Errorf("the backup was put back over a database the agent still has open")
	}
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
	settings := MigrateSettings{
		Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.8.0",
		// There is no agent in this test and no service manager either, so the
		// stop is a test's own and this machine's services are left alone.
		StopTheAgent: func(context.Context) error { return nil },
	}
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
