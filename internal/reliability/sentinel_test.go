package reliability_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aSentinel builds the lifecycle sentinel on a temporary home.
func aSentinel(t *testing.T) (*reliability.Sentinel, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	return reliability.NewSentinel(home, testkit.NewFakeClock(startOfTime)), home
}

func TestAFirstStartOnAFreshHomeIsACleanOne(t *testing.T) {
	sentinel, home := aSentinel(t)

	unclean, err := sentinel.Start()
	if err != nil {
		t.Fatalf("starting failed: %v", err)
	}
	if unclean {
		t.Errorf("the first start on a fresh home was called unclean")
	}
	if _, err := os.Stat(filepath.Join(home.RunFolder(), reliability.SentinelFileName)); err != nil {
		t.Errorf("the sentinel was not written while the program is running: %v", err)
	}
}

func TestAStartAfterACleanExitIsACleanOne(t *testing.T) {
	sentinel, _ := aSentinel(t)
	if _, err := sentinel.Start(); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}
	if err := sentinel.MarkExited(); err != nil {
		t.Fatalf("marking the clean exit failed: %v", err)
	}

	unclean, err := sentinel.Start()
	if err != nil {
		t.Fatalf("the second start failed: %v", err)
	}
	if unclean {
		t.Errorf("a start after a clean exit was called unclean")
	}
}

func TestAStartWithTheSentinelStillThereIsAnUncleanOne(t *testing.T) {
	sentinel, _ := aSentinel(t)
	if _, err := sentinel.Start(); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}

	unclean, err := sentinel.Start()
	if err != nil {
		t.Fatalf("the second start failed: %v", err)
	}
	if !unclean {
		t.Errorf("a start that found the sentinel of the last life was called clean")
	}
}

func TestADamagedSentinelStillMeansTheLastExitWasUnclean(t *testing.T) {
	sentinel, home := aSentinel(t)
	path := filepath.Join(home.RunFolder(), reliability.SentinelFileName)
	if err := os.WriteFile(path, []byte("half a sen"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the damaged sentinel failed: %v", err)
	}

	unclean, err := sentinel.Start()
	if err != nil {
		t.Fatalf("starting over a damaged sentinel failed: %v", err)
	}
	if !unclean {
		t.Errorf("a sentinel nobody can read was taken as a clean exit")
	}
}

func TestMarkingTheExitTwiceIsNotAFailure(t *testing.T) {
	sentinel, _ := aSentinel(t)
	if _, err := sentinel.Start(); err != nil {
		t.Fatalf("starting failed: %v", err)
	}

	if err := sentinel.MarkExited(); err != nil {
		t.Fatalf("marking the exit failed: %v", err)
	}
	if err := sentinel.MarkExited(); err != nil {
		t.Errorf("marking the exit a second time failed: %v", err)
	}
}

func TestTheSentinelSaysWhenItCannotBeWritten(t *testing.T) {
	home := contract.NewHome(filepath.Join(t.TempDir(), "cannot-be-written-in"))
	if err := os.Mkdir(home.Root, 0o500); err != nil {
		t.Fatalf("making the read-only home failed: %v", err)
	}
	sentinel := reliability.NewSentinel(home, testkit.NewFakeClock(startOfTime))

	if _, err := sentinel.Start(); err == nil {
		t.Errorf("the sentinel said nothing when it could not be written")
	}
}

func TestACorruptDatabaseIsMovedAsideWithTheTimeInItsName(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "coeus.db")
	for name, written := range map[string]string{
		"coeus.db":     "not a database at all",
		"coeus.db-wal": "the write-ahead file",
		"coeus.db-shm": "the shared memory file",
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
	_, err := reliability.MoveDatabaseAside(filepath.Join(t.TempDir(), "coeus.db"), time.Now())

	if err == nil {
		t.Errorf("moving a database that is not there was called a success")
	}
}
