// The design in this file is borrowed from OpenClaw's delivery queue on disk at
// ~/Code/openclaw/src/infra/outbound/delivery-queue-storage.ts and its table at
// ~/Code/openclaw/src/infra/delivery-queue-sqlite.ts, written fresh in Go and
// turned around. There the queue holds work on its way out, and every row
// carries a state and the number of times it has been claimed, so that a crash
// between claiming a row and finishing it leaves a row that is claimed again
// with a higher count rather than a piece of work that is quietly lost. Nerd Genie
// queues work on its way in and keeps only that idea: written down before
// anything looks at it, taken when it is handed to the loop, done when the task
// it started ends, and handed out again with the duplicate marker when the two
// were interrupted.

package channel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"

	// The pure-Go SQLite driver, registered under the name "sqlite". It is the
	// one internal/log already opens the same file with.
	_ "modernc.org/sqlite"
)

// queuedMessagesTable is the one table this package owns inside the single
// SQLite file whose other tables belong to internal/log.
const queuedMessagesTable = "queued_messages"

// The three states one queued message passes through. A message is written down
// waiting, marked taken when the loop is handed it, and marked done when the
// task it started ends.
const (
	stateWaiting = "waiting"
	stateTaken   = "taken"
	stateDone    = "done"
)

// queueTimeFormat is how the time a message arrived is written into the file:
// the internet date and time format with nanoseconds, always in UTC, which is
// what internal/log writes its own times in.
const queueTimeFormat = time.RFC3339Nano

// queueDatabaseOptions are the settings the queue's connection is opened with,
// and they are the ones internal/log opens the same file with: wait rather than
// fail when another connection holds the write lock, keep the write-ahead file,
// and let the operating system decide when to flush.
const queueDatabaseOptions = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)"

// queueSchemaStatements make the queue's table and the one index it reads by.
var queueSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS queued_messages (
		sequence    INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id  TEXT    NOT NULL,
		sender      TEXT    NOT NULL,
		text        TEXT    NOT NULL,
		attachments TEXT    NOT NULL,
		received    TEXT    NOT NULL,
		channel     TEXT    NOT NULL,
		state       TEXT    NOT NULL,
		taken_count INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS queued_messages_by_state ON queued_messages (state, sequence)`,
}

// ErrQueueFull is what Add returns when the queue already holds as many
// messages as the configuration allows. Its text is written to be sent straight
// back to whoever wrote the message.
var ErrQueueFull = errors.New("the agent already has as many messages waiting as it will hold, so wait for it to finish one and send this again")

// Queued is one message the queue handed out, with the number the queue gave it
// and whether it has been handed out before.
type Queued struct {
	// Sequence is the queue's own number for the message, which Done takes.
	Sequence int64
	// Message is what the user sent, exactly as the channel handed it over.
	Message contract.Inbound
	// Duplicate says this message was handed out before and never finished,
	// which happens when the agent stopped part way through the task it
	// started, so the work it asked for may already have been done once.
	Duplicate bool
}

// Queue is the waiting line every channel feeds and the loop's caller drains:
// one table in the single SQLite file, so that a message is on disk before
// anything looks at it and survives a restart.
type Queue struct {
	path     string
	capacity int
	guard    sync.Mutex
	database *sql.DB
	arrived  chan struct{}
	closed   bool
}

// OpenQueue opens the queue in the SQLite file at the path, making its table
// when it is not there yet, and gives it room for the number of messages the
// configuration allows. The event log owns the other tables in the same file and
// is opened first, because a file holding only the queue's table is not one
// internal/log will open.
//
// Opening also recovers what the last run left behind: a message that was in the
// loop's hands when the agent stopped goes back into the line, and the finished
// messages of earlier runs are cleared out.
func OpenQueue(ctx context.Context, path string, capacity int) (*Queue, error) {
	if capacity < 1 {
		return nil, fmt.Errorf("cannot open the queue at %s with room for %d messages, so give it room for at least one", path, capacity)
	}
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("cannot keep the queue at %s, because the database driver reads a question mark in a path as the start of its own options, so choose a path without one", path)
	}

	database, err := sql.Open("sqlite", path+queueDatabaseOptions)
	if err != nil {
		return nil, fmt.Errorf("cannot open the queue at %s: %w", path, err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	queue := &Queue{path: path, capacity: capacity, database: database, arrived: make(chan struct{}, 1)}
	if err := queue.prepare(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return queue, nil
}

// Close closes the queue's connection and wakes anyone waiting on it. Closing a
// queue that is already closed does nothing and is not an error.
func (queue *Queue) Close() error {
	queue.guard.Lock()
	defer queue.guard.Unlock()
	if queue.closed {
		return nil
	}
	queue.closed = true
	close(queue.arrived)
	if err := queue.database.Close(); err != nil {
		return fmt.Errorf("cannot close the queue at %s: %w", queue.path, err)
	}
	return nil
}

// Arrived carries one nudge every time a message is added, so that the loop's
// caller can wait rather than ask again and again. It is closed when the queue
// closes, which is how a waiting caller learns to stop.
func (queue *Queue) Arrived() <-chan struct{} {
	return queue.arrived
}

// Add writes one message into the queue and returns the number the queue gave
// it. It refuses a message with nothing in it, a message from no channel, and
// every message once the queue holds as many as the configuration allows, and
// the refusal it gives then wraps ErrQueueFull so that the channel can send its
// text straight back to the sender.
func (queue *Queue) Add(ctx context.Context, message contract.Inbound) (int64, error) {
	if strings.TrimSpace(message.Text) == "" && len(message.Attachments) == 0 {
		return 0, errors.New("cannot queue a message with no words and no files in it, so send something to work on")
	}
	if message.Channel == "" {
		return 0, errors.New("cannot queue a message that names no channel, because the reply would have nowhere to go")
	}
	attachments, err := json.Marshal(message.Attachments)
	if err != nil {
		return 0, fmt.Errorf("cannot write down the files that came with the message: %w", err)
	}

	queue.guard.Lock()
	defer queue.guard.Unlock()
	if err := queue.usable(); err != nil {
		return 0, err
	}
	held, err := queue.countHeld(ctx)
	if err != nil {
		return 0, err
	}
	if held >= queue.capacity {
		return 0, fmt.Errorf("the queue already holds %d of the %d messages it has room for: %w", held, queue.capacity, ErrQueueFull)
	}
	return queue.insert(ctx, message, string(attachments))
}

// Take hands out the oldest message that is still waiting and marks it taken. It
// returns false when there is nothing waiting. A message that was taken before
// and never finished comes back with the duplicate marker set.
func (queue *Queue) Take(ctx context.Context) (Queued, bool, error) {
	queue.guard.Lock()
	defer queue.guard.Unlock()
	if err := queue.usable(); err != nil {
		return Queued{}, false, err
	}

	row := queue.database.QueryRowContext(ctx,
		"SELECT sequence, message_id, sender, text, attachments, received, channel, taken_count"+
			" FROM "+queuedMessagesTable+" WHERE state = ? ORDER BY sequence LIMIT 1", stateWaiting)
	taken, err := decodeQueued(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Queued{}, false, nil
	}
	if err != nil {
		return Queued{}, false, fmt.Errorf("cannot read the next message out of the queue at %s: %w", queue.path, err)
	}

	if _, err := queue.database.ExecContext(ctx,
		"UPDATE "+queuedMessagesTable+" SET state = ?, taken_count = taken_count + 1 WHERE sequence = ?",
		stateTaken, taken.Sequence); err != nil {
		return Queued{}, false, fmt.Errorf("cannot mark message %d in the queue at %s as taken: %w", taken.Sequence, queue.path, err)
	}
	return taken, true, nil
}

// Done marks the message finished, which is what stops it from being handed out
// again after a restart. A number the queue is not holding is an error, because
// the caller and the queue then disagree about what has been done.
func (queue *Queue) Done(ctx context.Context, sequence int64) error {
	queue.guard.Lock()
	defer queue.guard.Unlock()
	if err := queue.usable(); err != nil {
		return err
	}

	result, err := queue.database.ExecContext(ctx,
		"UPDATE "+queuedMessagesTable+" SET state = ? WHERE sequence = ? AND state <> ?",
		stateDone, sequence, stateDone)
	if err != nil {
		return fmt.Errorf("cannot mark message %d in the queue at %s as done: %w", sequence, queue.path, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot tell whether message %d in the queue at %s was marked done: %w", sequence, queue.path, err)
	}
	if changed == 0 {
		return fmt.Errorf("the queue at %s holds no unfinished message numbered %d, so check the number the queue gave you when it handed the message out", queue.path, sequence)
	}
	return nil
}

// Held is how many messages the queue holds that are not finished, counting the
// one the loop has in hand. It is what the cap counts and what a status line
// reports.
func (queue *Queue) Held(ctx context.Context) (int, error) {
	queue.guard.Lock()
	defer queue.guard.Unlock()
	if err := queue.usable(); err != nil {
		return 0, err
	}
	return queue.countHeld(ctx)
}

// usable says whether the queue can be used, and the caller holds the lock.
func (queue *Queue) usable() error {
	if queue.closed {
		return fmt.Errorf("the queue at %s is closed, so open it again before using it", queue.path)
	}
	return nil
}

// countHeld counts the messages that are not finished, and the caller holds the
// lock.
func (queue *Queue) countHeld(ctx context.Context) (int, error) {
	held := 0
	row := queue.database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+queuedMessagesTable+" WHERE state <> ?", stateDone)
	if err := row.Scan(&held); err != nil {
		return 0, fmt.Errorf("cannot count the messages in the queue at %s: %w", queue.path, err)
	}
	return held, nil
}

// insert writes one row and nudges whoever is waiting, and the caller holds the
// lock.
func (queue *Queue) insert(ctx context.Context, message contract.Inbound, attachments string) (int64, error) {
	result, err := queue.database.ExecContext(ctx,
		"INSERT INTO "+queuedMessagesTable+
			" (message_id, sender, text, attachments, received, channel, state, taken_count)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, 0)",
		message.ID, message.Sender, message.Text, attachments,
		message.Received.UTC().Format(queueTimeFormat), message.Channel, stateWaiting)
	if err != nil {
		return 0, fmt.Errorf("cannot put the message into the queue at %s: %w", queue.path, err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot read the number the queue at %s gave the message: %w", queue.path, err)
	}
	select {
	case queue.arrived <- struct{}{}:
	default:
	}
	return sequence, nil
}

// prepare makes the table when the file has none and recovers what the last run
// left behind.
func (queue *Queue) prepare(ctx context.Context) error {
	for _, statement := range queueSchemaStatements {
		if _, err := queue.database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("cannot make the queue's table in %s, so point Nerd Genie at a different file or move this one aside: %w", queue.path, err)
		}
	}
	if _, err := queue.database.ExecContext(ctx,
		"UPDATE "+queuedMessagesTable+" SET state = ? WHERE state = ?", stateWaiting, stateTaken); err != nil {
		return fmt.Errorf("cannot put the messages the last run was working on back into the queue at %s: %w", queue.path, err)
	}
	if _, err := queue.database.ExecContext(ctx,
		"DELETE FROM "+queuedMessagesTable+" WHERE state = ?", stateDone); err != nil {
		return fmt.Errorf("cannot clear the finished messages out of the queue at %s: %w", queue.path, err)
	}
	return nil
}

// decodeQueued reads one row into the message that went into it.
func decodeQueued(row *sql.Row) (Queued, error) {
	var (
		taken       Queued
		attachments string
		received    string
		takenCount  int64
	)
	if err := row.Scan(&taken.Sequence, &taken.Message.ID, &taken.Message.Sender,
		&taken.Message.Text, &attachments, &received, &taken.Message.Channel, &takenCount); err != nil {
		return Queued{}, err
	}
	moment, err := time.Parse(queueTimeFormat, received)
	if err != nil {
		return Queued{}, fmt.Errorf("cannot read the time %q on queued message %d, so the file is damaged and should be restored from a backup: %w", received, taken.Sequence, err)
	}
	if err := json.Unmarshal([]byte(attachments), &taken.Message.Attachments); err != nil {
		return Queued{}, fmt.Errorf("cannot read the files that came with queued message %d, so the file is damaged and should be restored from a backup: %w", taken.Sequence, err)
	}
	taken.Message.Received = moment.UTC()
	taken.Duplicate = takenCount > 0
	return taken, nil
}
