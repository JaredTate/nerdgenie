// The design in this file is borrowed from OpenClaw's persisted queue at
// ~/Code/openclaw/src/infra/outbound/delivery-queue-storage.ts and written fresh
// in Go: a durable store on disk with a small, fixed set of read shapes and no
// clever indexing. There are five ways to read this log and no others. Each one
// is a query written out in full here, with the primary key or one of the two
// indexes behind it, and none of them is built out of anything the caller
// passed.

package log

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// selectColumns is the start of every read: the five columns of one row, in the
// order decodeEvent takes them.
const selectColumns = "SELECT sequence, occurred, task_id, kind, body FROM " + eventsTable

// rowScanner is what a single row and one row of a list have in common, so that
// one function reads either.
type rowScanner interface {
	// Scan reads the columns of one row into the values it is given.
	Scan(into ...any) error
}

// ByID returns one event by its sequence number. A number no event has is an
// error that names the number.
func (eventLog *Log) ByID(ctx context.Context, sequence int64) (contract.Event, error) {
	if sequence < 1 {
		return contract.Event{}, fmt.Errorf("cannot read event %d, because the log numbers its events from one upwards", sequence)
	}

	found, err := scanEvent(eventLog.reader.QueryRowContext(ctx, selectColumns+" WHERE sequence = ?", sequence))
	if errors.Is(err, sql.ErrNoRows) {
		return contract.Event{}, fmt.Errorf("there is no event numbered %d in the log at %s", sequence, eventLog.path)
	}
	if err != nil {
		return contract.Event{}, fmt.Errorf("cannot read event %d from the log at %s: %w", sequence, eventLog.path, err)
	}
	return found, nil
}

// scanEvent reads one row into an event.
func scanEvent(source rowScanner) (contract.Event, error) {
	var sequence int64
	var occurred, taskID, kind string
	var body []byte
	if err := source.Scan(&sequence, &occurred, &taskID, &kind, &body); err != nil {
		return contract.Event{}, err
	}
	return decodeEvent(sequence, occurred, taskID, kind, body)
}
