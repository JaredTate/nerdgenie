package reliability_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// whatANewLifeFound runs the recovery and starts the guard the way the program
// does, and hands back what that start found.
func whatANewLifeFound(t *testing.T, home contract.Home) reliability.Startup {
	t.Helper()
	aNewLife(t, home)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())
	found, err := guard.Start(context.Background())
	if err != nil {
		t.Fatalf("starting failed: %v", err)
	}
	return found
}

func TestAFirstStartOnAFreshHomeIsACleanOne(t *testing.T) {
	home := testkit.NewTempHome(t)

	found := whatANewLifeFound(t, home)

	if found.UncleanExit {
		t.Errorf("the first start on a fresh home was called unclean")
	}
	if _, err := os.Stat(filepath.Join(home.RunFolder(), reliability.SentinelFileName)); err != nil {
		t.Errorf("the sentinel was not written while the program is running: %v", err)
	}
}

func TestAStartAfterACleanExitIsACleanOne(t *testing.T) {
	home := testkit.NewTempHome(t)
	aNewLife(t, home)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}
	if err := guard.Stop(); err != nil {
		t.Fatalf("stopping cleanly failed: %v", err)
	}

	found := whatANewLifeFound(t, home)

	if found.UncleanExit {
		t.Errorf("a start after a clean exit was called unclean")
	}
}

func TestAStartWithTheSentinelStillThereIsAnUncleanOne(t *testing.T) {
	home := testkit.NewTempHome(t)
	aNewLife(t, home)

	// Nothing marked an exit, so the sentinel of that life is still there.
	found := whatANewLifeFound(t, home)

	if !found.UncleanExit {
		t.Errorf("a start that found the sentinel of the last life was called clean")
	}
}

func TestADamagedSentinelStillMeansTheLastExitWasUnclean(t *testing.T) {
	home := testkit.NewTempHome(t)
	path := filepath.Join(home.RunFolder(), reliability.SentinelFileName)
	if err := os.WriteFile(path, []byte("half a sen"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the damaged sentinel failed: %v", err)
	}

	found := whatANewLifeFound(t, home)

	if !found.UncleanExit {
		t.Errorf("a sentinel nobody can read was taken as a clean exit")
	}
}

func TestStoppingTwiceIsNotAFailure(t *testing.T) {
	home := testkit.NewTempHome(t)
	aNewLife(t, home)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())

	if err := guard.Stop(); err != nil {
		t.Fatalf("stopping failed: %v", err)
	}
	if err := guard.Stop(); err != nil {
		t.Errorf("stopping a second time failed: %v", err)
	}
}

func TestTheRecoverySaysWhenTheSentinelCannotBeWritten(t *testing.T) {
	home := contract.NewHome(filepath.Join(t.TempDir(), "cannot-be-written-in"))
	if err := os.Mkdir(home.Root, 0o500); err != nil {
		t.Fatalf("making the read-only home failed: %v", err)
	}

	_, err := reliability.PrepareDatabase(context.Background(), reliability.RecoverySettings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
	})

	if err == nil {
		t.Errorf("the recovery said nothing when the sentinel could not be written")
	}
}

func TestTheRecoveryNeedsAClockToNameTheFileItMovesAside(t *testing.T) {
	_, err := reliability.PrepareDatabase(context.Background(), reliability.RecoverySettings{
		Home: testkit.NewTempHome(t),
	})

	if err == nil {
		t.Errorf("the recovery ran with no clock to read the time from")
	}
}

func TestACorruptDatabaseIsMovedAsideWithTheTimeInItsName(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "nerdgenie.db")
	for name, written := range map[string]string{
		"nerdgenie.db":     "not a database at all",
		"nerdgenie.db-wal": "the write-ahead file",
		"nerdgenie.db-shm": "the shared memory file",
	} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(written), contract.SecretFileMode); err != nil {
			t.Fatalf("writing %s failed: %v", name, err)
		}
	}

	movedTo, err := reliability.MoveDatabaseAside(path, startOfTime)
	if err != nil {
		t.Fatalf("moving the database aside failed: %v", err)
	}

	if !strings.Contains(movedTo, "2026-09-02") {
		t.Errorf("the database was moved to %q, which does not say when", movedTo)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the corrupt database is still where the agent will open it: %v", err)
	}
	if _, err := os.Stat(movedTo); err != nil {
		t.Errorf("the corrupt database was not kept for somebody to look at: %v", err)
	}
	for _, beside := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + beside); !os.IsNotExist(err) {
			t.Errorf("the %s file was left beside a database that is going to be replaced", beside)
		}
		if _, err := os.Stat(movedTo + beside); err != nil {
			t.Errorf("the %s file was thrown away rather than kept with the database: %v", beside, err)
		}
	}
}

func TestMovingADatabaseThatIsNotThereSaysSo(t *testing.T) {
	_, err := reliability.MoveDatabaseAside(filepath.Join(t.TempDir(), "nerdgenie.db"), time.Now())

	if err == nil {
		t.Errorf("moving a database that is not there was called a success")
	}
}
