// The design in this file is borrowed from Hermes' delivery ledger at
// ~/Code/hermes-agent/gateway/delivery_ledger.py and written fresh in Go. There
// the obligation to deliver a reply is written to disk before the send is tried,
// so that a crash between the two leaves a row that says "this may have
// happened" instead of leaving nothing at all. Append is the Nerd Genie half of that
// idea: it hands the sequence number back to the caller, so the caller can write
// down what it is about to do, do it, and write down what happened, with the
// first row standing as the record either way.

package log

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Append writes one event to the end of the log and returns the sequence number
// the log gave it. The number only ever grows and is never reused, so a caller
// that writes an obligation down before acting can always find that row again.
// The caller supplies the time on the event, because time in Nerd Genie is read from
// contract.Clock; a time in any zone is kept as the same moment in UTC.
func (eventLog *Log) Append(ctx context.Context, event contract.Event) (int64, error) {
	if !contract.KnownEventKind(event.Kind) {
		return 0, fmt.Errorf("cannot append an event of the kind %q, so use one of the kinds listed in internal/contract", event.Kind)
	}
	body, err := encodeBody(event.Body)
	if err != nil {
		return 0, err
	}

	eventLog.writing.Lock()
	defer eventLog.writing.Unlock()

	result, err := eventLog.writer.ExecContext(ctx,
		"INSERT INTO "+eventsTable+" (occurred, task_id, kind, body) VALUES (?, ?, ?, ?)",
		encodeTime(event.Occurred), event.TaskID, string(event.Kind), body)
	if err != nil {
		return 0, fmt.Errorf("cannot append the event to the log at %s: %w", eventLog.path, err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot read the number the log gave the event just appended to %s: %w", eventLog.path, err)
	}
	return sequence, nil
}
