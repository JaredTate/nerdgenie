package log

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// aLogWithOneGoodEvent makes a log holding one event that reads back properly,
// closes it, and returns its path, so that a test can then damage the file
// behind the log's back.
func aLogWithOneGoodEvent(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coeus.db")
	opened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening a new log failed: %v", err)
	}
	if _, err := opened.Append(context.Background(), contract.Event{
		Occurred: aTime,
		TaskID:   "t1",
		Kind:     contract.EventMessage,
	}); err != nil {
		t.Fatalf("appending the one good event failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the log failed: %v", err)
	}
	return path
}

// runOnTheFile runs one statement straight against the file, which is how these
// tests damage a log the way a failing disk or another program would.
func runOnTheFile(t *testing.T, path string, statement string, arguments ...any) {
	t.Helper()
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("cannot open %s to damage it: %v", path, err)
	}
	defer handle.Close()
	if _, err := handle.Exec(statement, arguments...); err != nil {
		t.Fatalf("cannot damage %s: %v", path, err)
	}
}

func TestEveryReadRefusesARowItCannotUnderstand(t *testing.T) {
	path := aLogWithOneGoodEvent(t)
	runOnTheFile(t, path,
		"INSERT INTO events (occurred, task_id, kind, body) VALUES (?, ?, ?, ?)",
		"last tuesday", "t1", string(contract.EventMessage), []byte{})

	eventLog, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopening the damaged log failed, and the damage is in a row rather than the schema: %v", err)
	}
	defer eventLog.Close()
	ctx := context.Background()

	if _, err := eventLog.ByTask(ctx, "t1"); err == nil {
		t.Error("reading by task over a damaged row returned no error, and it must say the file is damaged")
	}
	if _, err := eventLog.ByKind(ctx, contract.EventMessage); err == nil {
		t.Error("reading by kind over a damaged row returned no error, and it must say the file is damaged")
	}
	if _, err := eventLog.ByRange(ctx, contract.EventRange{From: 1, To: 2}); err == nil {
		t.Error("reading a span over a damaged row returned no error, and it must say the file is damaged")
	}
	if _, err := eventLog.ByID(ctx, 2); err == nil {
		t.Error("reading a damaged row by its number returned no error, and it must say the file is damaged")
	}

	handed := 0
	err = eventLog.Replay(ctx, func(contract.Event) error {
		handed++
		return nil
	})
	if err == nil {
		t.Error("replaying over a damaged row returned no error, and it must say the file is damaged")
	}
	if handed != 1 {
		t.Errorf("the replay handed back %d events before the damaged one, want 1", handed)
	}
}

func TestOpenRefusesTablesWithTheRightNamesAndTheWrongShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "impostor.db")
	runOnTheFile(t, path, "CREATE TABLE events (anything TEXT)")
	runOnTheFile(t, path, "CREATE TABLE schema_version (anything TEXT)")

	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening a file whose tables only share their names returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}

func TestOpenRefusesALogWhoseSchemaVersionIsGone(t *testing.T) {
	path := aLogWithOneGoodEvent(t)
	runOnTheFile(t, path, "DELETE FROM schema_version")

	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("opening a log with no schema version returned no error, and it must refuse")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error is %q, and it must name the file %s", err, path)
	}
}
