package log

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestLog opens a log in a folder the test framework removes afterwards and
// closes it when the test ends.
func newTestLog(t *testing.T) *Log {
	t.Helper()
	opened, err := Open(context.Background(), filepath.Join(t.TempDir(), "nerdgenie.db"))
	if err != nil {
		t.Fatalf("cannot open a log for the test: %v", err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("closing the log failed: %v", err)
		}
	})
	return opened
}

func TestOpenCreatesTheSchemaWithVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nerdgenie.db")
	opened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the log file was not created at %s: %v", path, err)
	}
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("cannot open the file to look at its schema: %v", err)
	}
	defer handle.Close()

	version := 0
	if err := handle.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("the schema_version table is missing or empty: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("the schema version is %d, want %d", version, SchemaVersion)
	}
}

func TestOpenReopensALogItAlreadyMade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nerdgenie.db")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("closing the first log failed: %v", err)
	}

	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopening the log failed: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("closing the second log failed: %v", err)
	}
}

func TestOpenRefusesAFileThatIsNotACoeusLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "someone-elses.db")
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("cannot make the file that stands in for another program's database: %v", err)
	}
	if _, err := handle.Exec("CREATE TABLE recipes (name TEXT)"); err != nil {
		t.Fatalf("cannot write the other program's table: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("cannot close the other program's database: %v", err)
	}

	_, err = Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening another program's database returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}

func TestOpenRefusesANewerSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nerdgenie.db")
	opened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}

	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("cannot open the file to change its schema version: %v", err)
	}
	if _, err := handle.Exec("UPDATE schema_version SET version = ?", SchemaVersion+1); err != nil {
		t.Fatalf("cannot raise the schema version: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("cannot close the file after raising its schema version: %v", err)
	}

	_, err = Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening a log written by a newer Coeus returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}

func TestOpenRefusesAFileThatIsNotADatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("this is a person's notes, not a database"), 0o600); err != nil {
		t.Fatalf("cannot write the file that stands in for a stray text file: %v", err)
	}

	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening a text file returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}

func TestOpenRefusesAPathWithAQuestionMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "why?.db")
	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening a path with a question mark returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}

func TestOpenRefusesAPathInAFolderThatIsNotThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-folder", "nerdgenie.db")
	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening a log in a folder that is not there returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}

func TestCloseTwiceIsHarmless(t *testing.T) {
	opened, err := Open(context.Background(), filepath.Join(t.TempDir(), "nerdgenie.db"))
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("the first close failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Errorf("the second close failed, and closing a closed log must be harmless: %v", err)
	}
}
