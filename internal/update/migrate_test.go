package update

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aDatabaseAtVersion makes a real event log in a temporary home and moves its
// schema version to the number given, which is how a test stands in for a file
// written by another version of Coeus.
func aDatabaseAtVersion(t *testing.T, path string, version int, appliedBy string) {
	t.Helper()
	opened, err := log.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("making the event log failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}
	if version <= log.SchemaVersion {
		return
	}
	database, err := openDatabase(path)
	if err != nil {
		t.Fatalf("opening the database failed: %v", err)
	}
	defer func() { _ = database.Close() }()
	if err := recordMigration(context.Background(), database, Migration{To: version, Name: "from a later Coeus"}, appliedBy, theTestMoment); err != nil {
		t.Fatalf("writing the newer schema version failed: %v", err)
	}
}

// theTestMoment is when the migration tests date their rows from.
var theTestMoment = time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC)

func TestTheSchemaVersionIsTheHighestMigrationOrTheOneTheLogMade(t *testing.T) {
	if SchemaVersion() < log.SchemaVersion {
		t.Errorf("this program understands schema version %d, which is below the %d the log created", SchemaVersion(), log.SchemaVersion)
	}
	last := log.SchemaVersion
	for _, migration := range Migrations() {
		if migration.To != last+1 {
			t.Errorf("migration %q takes the schema to %d where the one before it left it at %d, and migrations are numbered one at a time",
				migration.Name, migration.To, last)
		}
		if migration.Name == "" || migration.Apply == nil {
			t.Errorf("migration %d has no name or nothing to do", migration.To)
		}
		last = migration.To
	}
	if SchemaVersion() != last {
		t.Errorf("this program says it understands schema version %d where its migrations end at %d", SchemaVersion(), last)
	}
}

func TestADatabaseFromANewerCoeusIsRefusedWithTheVersionToUse(t *testing.T) {
	home := testkit.NewTempHome(t)
	aDatabaseAtVersion(t, home.DatabaseFile(), SchemaVersion()+1, "0.9.0")

	err := CheckSchema(context.Background(), home.DatabaseFile())

	if err == nil {
		t.Fatalf("a database written by a newer Coeus was accepted")
	}
	if !strings.Contains(err.Error(), "0.9.0") {
		t.Errorf("the refusal does not name the version to use: %v", err)
	}
	if !strings.Contains(err.Error(), home.DatabaseFile()) {
		t.Errorf("the refusal does not name the database: %v", err)
	}
}

func TestADatabaseFromANewerCoeusThatNamesNoVersionStillRefuses(t *testing.T) {
	home := testkit.NewTempHome(t)
	aDatabaseAtVersion(t, home.DatabaseFile(), SchemaVersion()+1, "")

	err := CheckSchema(context.Background(), home.DatabaseFile())

	if err == nil {
		t.Fatalf("a database written by a newer Coeus was accepted")
	}
	if !strings.Contains(err.Error(), "update") {
		t.Errorf("the refusal does not say what to do about it: %v", err)
	}
}

func TestADatabaseThisProgramUnderstandsIsAccepted(t *testing.T) {
	home := testkit.NewTempHome(t)
	aDatabaseAtVersion(t, home.DatabaseFile(), log.SchemaVersion, "")

	if err := CheckSchema(context.Background(), home.DatabaseFile()); err != nil {
		t.Errorf("a database this program wrote was refused: %v", err)
	}
}

func TestAHomeWithNoDatabaseYetHasNothingToCheckOrMigrate(t *testing.T) {
	home := testkit.NewTempHome(t)

	if err := CheckSchema(context.Background(), home.DatabaseFile()); err != nil {
		t.Errorf("a home with no database yet was refused: %v", err)
	}
	applied, err := Migrate(context.Background(), MigrateSettings{
		Home: home, Clock: testkit.NewFakeClock(theTestMoment), Version: "0.7.0",
	})
	if err != nil {
		t.Errorf("migrating a home with no database yet failed: %v", err)
	}
	if applied != 0 {
		t.Errorf("%d migrations were applied to a database that is not there", applied)
	}
}

func TestAFileThatIsNotACoeusDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-database.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening a new file failed: %v", err)
	}
	if _, err := database.Exec("CREATE TABLE somebody_elses (one INTEGER)"); err != nil {
		t.Fatalf("writing another program's table failed: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the file failed: %v", err)
	}

	if err := CheckSchema(context.Background(), path); err == nil {
		t.Errorf("a file that is not a Coeus database was accepted")
	}
}
