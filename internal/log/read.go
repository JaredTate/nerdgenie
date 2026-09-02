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

// ByTask returns every event of one task or job, in order. A task with more than
// MaxEventsPerRead events, which is far more than one task ever writes, comes
// back cut short: the events read, and an error saying where to carry on from.
func (eventLog *Log) ByTask(ctx context.Context, taskID string) ([]contract.Event, error) {
	if taskID == "" {
		return nil, errors.New("cannot read the events of a task with no id, so pass the id of the task you want")
	}
	return eventLog.events(ctx, "WHERE task_id = ?", eventsPerReadCap, taskID)
}

// ByKind returns every event of one kind, in order, and says so when there are
// more of them than one read returns. A kind the log does not know is an error
// rather than an empty answer, because an empty answer would look like a log
// that had nothing of that kind.
func (eventLog *Log) ByKind(ctx context.Context, kind contract.EventKind) ([]contract.Event, error) {
	if !contract.KnownEventKind(kind) {
		return nil, fmt.Errorf("cannot read the events of the kind %q, so use one of the kinds listed in internal/contract", kind)
	}
	return eventLog.events(ctx, "WHERE kind = ?", eventsPerReadCap, string(kind))
}

// ByRange returns every event in a span of sequence numbers, from the first up
// to and including the last, in order. It is how a caller walks a log longer than
// one read can hold: when a read comes back cut short, the error names the last
// event it handed back, and the next page starts at the number after that one.
func (eventLog *Log) ByRange(ctx context.Context, span contract.EventRange) ([]contract.Event, error) {
	if span.From < 1 {
		return nil, fmt.Errorf("cannot read the events from number %d, because the log numbers its events from one upwards", span.From)
	}
	if span.To < span.From {
		return nil, fmt.Errorf("cannot read the events from number %d to number %d, because that span ends before it starts, so put the smaller number first", span.From, span.To)
	}
	return eventLog.events(ctx, "WHERE sequence >= ? AND sequence <= ?", eventsPerReadCap, span.From, span.To)
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

// events runs one of the fixed read shapes and returns what it found, in
// sequence order. The condition is written out in one of the methods above and
// never built from anything a caller passed; the values a caller passed arrive
// as arguments and are bound by the driver.
//
// It asks the file for one row more than the limit, so that a result which just
// fits can be told apart from one that was cut short. A result that was cut short
// comes back as the events that were read together with an error naming the last
// of them, never quietly, because a caller handed part of an answer would believe
// it had the whole one.
func (eventLog *Log) events(ctx context.Context, condition string, limit int, arguments ...any) ([]contract.Event, error) {
	bounded := boundedLimit(limit)
	query := selectColumns + " " + condition + " ORDER BY sequence LIMIT ?"
	rows, err := eventLog.reader.QueryContext(ctx, query, append(arguments, bounded+1)...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the log at %s: %w", eventLog.path, err)
	}
	defer rows.Close()

	found := []contract.Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read a row of the log at %s: %w", eventLog.path, err)
		}
		found = append(found, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot finish reading the log at %s: %w", eventLog.path, err)
	}
	if len(found) > bounded {
		found = found[:bounded]
		return found, fmt.Errorf("this read of the log at %s stopped at %d events, which is all one read returns, so read the rest in pages with ByRange starting after event %d",
			eventLog.path, bounded, found[bounded-1].Sequence)
	}
	return found, nil
}

// boundedLimit returns the smaller of the limit asked for and the cap every read
// applies, and the cap itself for a limit that makes no sense, so that no read
// can pull the whole log into memory by accident. It never returns less than one,
// because a read that asks for no rows has nothing to hand back or to report.
func boundedLimit(limit int) int {
	bounded := limit
	if bounded < 1 || bounded > eventsPerReadCap {
		bounded = eventsPerReadCap
	}
	if bounded < 1 {
		bounded = 1
	}
	return bounded
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
