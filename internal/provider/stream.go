package provider

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/JaredTate/coeus/internal/contract"
)

// dataPrefix begins every line of a server-sent-event stream that carries a
// payload. Both wire protocols use the same shape, so both are read here.
var dataPrefix = []byte("data:")

// doneMarker is what the OpenAI-compatible stream sends instead of a payload to
// say that there is nothing more coming.
const doneMarker = "[DONE]"

// stallWatch cancels a call whose stream has sent nothing for the stall timeout.
// It counts progress rather than reading the clock, so that the wait is measured
// on the harness clock and no test ever waits on the real one.
type stallWatch struct {
	progress atomic.Uint64
	stalled  atomic.Bool
}

// saw records that some bytes arrived, which is what keeps the watch quiet.
func (watch *stallWatch) saw() {
	watch.progress.Add(1)
}

// watchFor sleeps in stall-length steps and cancels the call the first time a
// whole step passes with no bytes at all.
func (watch *stallWatch) watchFor(ctx context.Context, clock contract.Clock, giveUp context.CancelFunc) {
	for {
		seen := watch.progress.Load()
		if err := clock.Sleep(ctx, stallTimeout); err != nil {
			return
		}
		if watch.progress.Load() == seen {
			watch.stalled.Store(true)
			giveUp()
			return
		}
	}
}

// explain turns the error a cancelled call reports into the stalled-stream
// sentinel when the watch was the one that cancelled it, and otherwise says
// which model could not be reached.
func (watch *stallWatch) explain(modelName string, err error) error {
	if watch.stalled.Load() {
		return fmt.Errorf("the model %q sent nothing for %s: %w", modelName, stallTimeout, contract.ErrStalledStream)
	}
	return providerError{modelName: modelName, message: err.Error(), retryable: retryableConnection(err)}
}

// retryableConnection says whether a failure that happened before or during the
// stream is one a later attempt could get past. A cancelled call is the caller's
// own decision and is never retried; everything else on the wire is.
func retryableConnection(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// watchingReader passes bytes through and tells the stall watch that they
// arrived.
type watchingReader struct {
	inner io.Reader
	watch *stallWatch
}

// Read hands on what the stream gave and records the progress.
func (reader watchingReader) Read(into []byte) (int, error) {
	read, err := reader.inner.Read(into)
	if read > 0 {
		reader.watch.saw()
	}
	return read, err
}

// forEachDataLine walks a server-sent-event stream and hands the payload of
// every "data:" line to the function, stopping when the function says to or when
// the stream ends. Event names, comments, and blank lines are skipped, because
// both protocols repeat the event's name inside its payload.
func forEachDataLine(reader io.Reader, take func(payload []byte) (bool, error)) error {
	lines := bufio.NewScanner(reader)
	lines.Buffer(make([]byte, 0, 4096), maxEventLineBytes)
	for lines.Scan() {
		line := bytes.TrimRight(lines.Bytes(), "\r")
		if !bytes.HasPrefix(line, dataPrefix) {
			continue
		}
		payload := bytes.TrimSpace(line[len(dataPrefix):])
		if len(payload) == 0 || string(payload) == doneMarker {
			if string(payload) == doneMarker {
				return nil
			}
			continue
		}
		stop, err := take(payload)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	if err := lines.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("one line of the model's answer was longer than the %d byte limit, so the stream was given up on", maxEventLineBytes)
		}
		return err
	}
	return nil
}

// addText appends a piece of streamed text to the reply, keeping it inside the
// cap, and returns the piece that was actually kept so that the deltas the
// caller sees always join to the reply's text.
func addText(into *bytes.Buffer, piece string) string {
	room := maxReplyTextBytes - into.Len()
	if room <= 0 {
		return ""
	}
	if len(piece) > room {
		piece = piece[:room]
	}
	into.WriteString(piece)
	return piece
}
