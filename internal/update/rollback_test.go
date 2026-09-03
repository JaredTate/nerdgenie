package update_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/update"

	// The pure-Go SQLite driver, so that a test can write a schema version of
	// its own into the database the updater reads.
	_ "modernc.org/sqlite"
)

func TestARollbackToAVersionThatDoesNotComeUpGoesBackToTheOneThatWasRunning(t *testing.T) {
	home, service := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aProgramThatExitsAtOnce)
	working := anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, t.TempDir(), "0.7.0").Rollback(context.Background())

	if err == nil {
		t.Fatalf("a rollback to a version that never came up was reported as done")
	}
	if linkPointsAt(t, home) != working {
		t.Errorf("the current link points at %s rather than back at %s, which was working", linkPointsAt(t, home), working)
	}
	if !service.running(t) {
		t.Errorf("the version that was working was not started again:\n%s", service.told(t))
	}
	if strings.Contains(err.Error(), "did not come up again either") {
		t.Errorf("the report says the working version could not be started again, and it was never tried: %v", err)
	}
	if !strings.Contains(err.Error(), "0.7.0") {
		t.Errorf("the report does not name the version the machine is back on: %v", err)
	}
	if outcome.To != "0.7.0" {
		t.Errorf("the outcome says the machine is on %q rather than back on 0.7.0", outcome.To)
	}
}

// aDatabaseFromANewerCoeus makes the one database and writes a schema version
// above the one this program understands into it, which is what the file looks
// like after a later version of Coeus has migrated it.
func aDatabaseFromANewerCoeus(t *testing.T, home contract.Home) {
	t.Helper()
	opened, err := log.Open(context.Background(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("making the event log failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}
	database, err := sql.Open("sqlite", home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the database failed: %v", err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec("INSERT INTO schema_version (version) VALUES (?)", update.SchemaVersion()+1); err != nil {
		t.Fatalf("writing the newer schema version failed: %v", err)
	}
}

func TestARollbackIsRefusedWhenTheDatabaseIsFromANewerCoeus(t *testing.T) {
	home, service := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	working := anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	aDatabaseFromANewerCoeus(t, home)

	_, err := anUpdater(t, home, testkit.NewFakeClock(startOfTime), t.TempDir(), "0.7.0").
		Rollback(context.Background())

	if err == nil {
		t.Fatalf("a rollback under a database this program does not understand went ahead")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("the refusal does not say that the database is the trouble: %v", err)
	}
	if linkPointsAt(t, home) != working {
		t.Errorf("the current link was moved to %s although the rollback was refused", linkPointsAt(t, home))
	}
	if service.told(t) != "" {
		t.Errorf("the service was disturbed although the rollback was refused:\n%s", service.told(t))
	}
}
