package log

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// timeFormat is how a moment is written into the file: the internet date and
// time format with nanoseconds, always in UTC. It is text rather than a number
// so that the zero time and a time from any century both come back exactly as
// they went in, and so that a person reading the file with any SQLite tool can
// see when something happened.
const timeFormat = time.RFC3339Nano

// encodeTime writes a moment the way the file holds it.
func encodeTime(moment time.Time) string {
	return moment.UTC().Format(timeFormat)
}

// encodeBody checks an event's own fields on the way in. A body that is not
// JSON is refused here rather than stored, so that a reader of the log is never
// handed something it cannot parse.
func encodeBody(body json.RawMessage) ([]byte, error) {
	if len(body) == 0 {
		return []byte{}, nil
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("cannot append an event whose body is not JSON, so give it a body the log can read back, not %s", shortened(body))
	}
	return body, nil
}

// decodeEvent turns one row of the events table back into an event. Everything
// the file holds is checked here, because the file outlives the program that
// wrote it and may have been damaged, edited, or written by another version:
// anything this version cannot read is an error that says what to do, never a
// panic.
func decodeEvent(sequence int64, occurred string, taskID string, kind string, body []byte) (contract.Event, error) {
	if sequence < 1 {
		return contract.Event{}, fmt.Errorf("the log holds an event numbered %d and every sequence number is at least one, so the file is damaged and should be restored from a backup", sequence)
	}
	moment, err := time.Parse(timeFormat, occurred)
	if err != nil {
		return contract.Event{}, fmt.Errorf("cannot read the time %s on event %d, so the file is damaged and should be restored from a backup: %w", shortened([]byte(occurred)), sequence, err)
	}
	if !contract.KnownEventKind(contract.EventKind(kind)) {
		return contract.Event{}, fmt.Errorf("event %d is of the kind %s, which this version of Nerd Genie does not know, so update Nerd Genie and open the log again", sequence, shortened([]byte(kind)))
	}
	if len(body) > 0 && !json.Valid(body) {
		return contract.Event{}, fmt.Errorf("the body of event %d is not JSON, so the file is damaged and should be restored from a backup", sequence)
	}

	decoded := contract.Event{
		Sequence: sequence,
		Occurred: moment.UTC(),
		TaskID:   taskID,
		Kind:     contract.EventKind(kind),
	}
	if len(body) > 0 {
		decoded.Body = json.RawMessage(body)
	}
	return decoded, nil
}

// maxQuotedLength is how much of a bad value an error message shows. Enough to
// recognise the value, little enough that a whole tool result cannot end up in
// somebody's terminal.
const maxQuotedLength = 60

// shortened quotes a value for an error message and cuts it short, so that an
// error about one bad row never prints a megabyte of it.
func shortened(value []byte) string {
	if len(value) <= maxQuotedLength {
		return fmt.Sprintf("%q", value)
	}
	return fmt.Sprintf("%q and %d more bytes", value[:maxQuotedLength], len(value)-maxQuotedLength)
}
