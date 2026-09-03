package channel

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// aQueueFileWithOneMessage makes a queue holding one message that reads back
// properly, closes it, and returns its path, so that a test can then damage the
// file behind the queue's back.
func aQueueFileWithOneMessage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coeus.db")
	opened, err := OpenQueue(context.Background(), path, 10)
	if err != nil {
		t.Fatalf("opening a new queue failed: %v", err)
	}
	if _, err := opened.Add(context.Background(), anInbound("the one good message")); err != nil {
		t.Fatalf("adding the one good message failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the queue failed: %v", err)
	}
	return path
}

// runOnTheQueueFile runs one statement straight against the file, which is how
// these tests damage a queue the way a failing disk or another program would.
func runOnTheQueueFile(t *testing.T, path string, statement string, arguments ...any) {
	t.Helper()
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("cannot open %s to damage it: %v", path, err)
	}
	defer func() { _ = handle.Close() }()
	if _, err := handle.Exec(statement, arguments...); err != nil {
		t.Fatalf("cannot damage %s: %v", path, err)
	}
}

// openDamagedQueue opens a queue on a file a test has damaged and closes it at
// the end of the test.
func openDamagedQueue(t *testing.T, path string) *Queue {
	t.Helper()
	queue, err := OpenQueue(context.Background(), path, 10)
	if err != nil {
		t.Fatalf("reopening the queue failed, and the damage is in a row rather than the table: %v", err)
	}
	t.Cleanup(func() { _ = queue.Close() })
	return queue
}

func TestTheQueueRefusesARowWhoseTimeItCannotRead(t *testing.T) {
	path := aQueueFileWithOneMessage(t)
	runOnTheQueueFile(t, path, "UPDATE queued_messages SET received = ?", "last tuesday")

	queue := openDamagedQueue(t, path)
	_, _, err := queue.Take(context.Background())
	if err == nil {
		t.Fatal("a message whose time is not a time was handed out, want an error saying the file is damaged")
	}
	if !strings.Contains(err.Error(), "last tuesday") {
		t.Errorf("the error reads %q, and it has to name what it could not read", err)
	}
}

func TestTheQueueRefusesARowWhoseFileListItCannotRead(t *testing.T) {
	path := aQueueFileWithOneMessage(t)
	runOnTheQueueFile(t, path, "UPDATE queued_messages SET attachments = ?", "not a list at all")

	queue := openDamagedQueue(t, path)
	if _, _, err := queue.Take(context.Background()); err == nil {
		t.Fatal("a message whose file list is not a list was handed out, want an error saying the file is damaged")
	}
}

func TestEveryCallSaysSoWhenTheQueuesTableIsGone(t *testing.T) {
	ctx := context.Background()
	path := aQueueFileWithOneMessage(t)
	queue := openDamagedQueue(t, path)
	runOnTheQueueFile(t, path, "DROP TABLE queued_messages")

	if _, err := queue.Add(ctx, anInbound("hello")); err == nil {
		t.Error("a message was queued into a table that is no longer there")
	}
	if _, _, err := queue.Take(ctx); err == nil {
		t.Error("a message came out of a table that is no longer there")
	}
	if err := queue.Done(ctx, 1); err == nil {
		t.Error("a message was finished in a table that is no longer there")
	}
	if _, err := queue.Held(ctx); err == nil {
		t.Error("a table that is no longer there was counted")
	}
}

func TestAddSaysSoWhenTheRowWillNotGoIn(t *testing.T) {
	path := aQueueFileWithOneMessage(t)
	queue := openDamagedQueue(t, path)
	// Counting still works after the column is renamed, so the insert is what
	// fails, which is the path this test is here for.
	runOnTheQueueFile(t, path, "ALTER TABLE queued_messages RENAME COLUMN text TO words")

	if _, err := queue.Add(context.Background(), anInbound("hello")); err == nil {
		t.Error("a message went into a table with no column to hold its words")
	}
}

func TestTakingAndFinishingSaySoWhenTheFileRefusesTheChange(t *testing.T) {
	ctx := context.Background()
	path := aQueueFileWithOneMessage(t)
	queue := openDamagedQueue(t, path)
	runOnTheQueueFile(t, path,
		"CREATE TRIGGER refuse_every_change BEFORE UPDATE ON queued_messages"+
			" BEGIN SELECT RAISE(ABORT, 'this file refuses changes'); END")

	if _, _, err := queue.Take(ctx); err == nil {
		t.Error("a message was marked taken in a file that refuses changes")
	}
	if err := queue.Done(ctx, 1); err == nil {
		t.Error("a message was marked done in a file that refuses changes")
	}
}

func TestOpeningRefusesAFileWhoseQueueTableIsTheWrongShape(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()

	wrongColumns := filepath.Join(folder, "wrong-columns.db")
	runOnTheQueueFile(t, wrongColumns, "CREATE TABLE queued_messages (whatever TEXT)")
	if _, err := OpenQueue(ctx, wrongColumns, 10); err == nil {
		t.Error("a file whose queue table has none of the right columns was opened as a queue")
	}

	notATable := filepath.Join(folder, "not-a-table.db")
	runOnTheQueueFile(t, notATable, "CREATE VIEW queued_messages AS SELECT 1")
	if _, err := OpenQueue(ctx, notATable, 10); err == nil {
		t.Error("a file whose queue table is not a table at all was opened as a queue")
	}
}
