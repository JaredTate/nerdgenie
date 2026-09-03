package browser

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// DefaultBufferedEvents is how many of the person's own events are held for a
// reader that has fallen behind, when the caller names no number. It is the
// default of contract.DefaultConfig().Caps.BufferedBrowserEvents, and a test
// pins the two together.
const DefaultBufferedEvents = 256

// eventStream is what the worker says the person did, handed on to whoever is
// reading. One stream belongs to one worker: when that worker goes, every reader
// of it is finished, because a new worker is a new window with a new page in it.
type eventStream struct {
	guard   sync.Mutex
	held    int
	note    func(format string, arguments ...any)
	readers map[*eventReader]struct{}
	closed  bool
	// toldAboutDrops is true once the log has been told that a reader is behind,
	// so that a reader nobody is draining costs one line and not thousands.
	toldAboutDrops bool
}

// eventReader is one caller of Events, with the events waiting for it.
type eventReader struct {
	events chan contract.BrowserEvent
	gone   chan struct{}
}

// newEventStream builds the stream one worker's events go through.
func newEventStream(held int, note func(format string, arguments ...any)) *eventStream {
	if held <= 0 {
		held = DefaultBufferedEvents
	}
	return &eventStream{held: held, note: note, readers: map[*eventReader]struct{}{}}
}

// listen hands back a channel of what the person does from now on. It ends when
// the caller's context is done or the worker goes, whichever comes first.
func (stream *eventStream) listen(ctx context.Context) (<-chan contract.BrowserEvent, error) {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	if stream.closed {
		return nil, errors.New("the browser worker this stream belonged to has gone, so ask for the events again")
	}
	reader := &eventReader{
		events: make(chan contract.BrowserEvent, stream.held),
		gone:   make(chan struct{}),
	}
	stream.readers[reader] = struct{}{}
	go stream.finishWhenDone(ctx, reader)
	return reader.events, nil
}

// finishWhenDone ends one reader's channel when it stops listening, and ends
// itself when the channel was closed for some other reason.
func (stream *eventStream) finishWhenDone(ctx context.Context, reader *eventReader) {
	select {
	case <-ctx.Done():
		stream.stopReading(reader)
	case <-reader.gone:
	}
}

// stopReading takes one reader off the stream and closes its channel.
func (stream *eventStream) stopReading(reader *eventReader) {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	if _, listening := stream.readers[reader]; !listening {
		return
	}
	delete(stream.readers, reader)
	finish(reader)
}

// deliver puts one event in front of every reader. A reader that has fallen a
// whole buffer behind loses its oldest event rather than holding the worker up,
// and the log is told once that it is happening.
func (stream *eventStream) deliver(event contract.BrowserEvent) {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	for reader := range stream.readers {
		if handedOver(reader.events, event) {
			continue
		}
		if !stream.toldAboutDrops {
			stream.toldAboutDrops = true
			stream.note("the browser event stream is more than %d events behind, so the oldest are being dropped", stream.held)
		}
	}
}

// handedOver puts one event in front of a reader and says whether it fitted.
// When it did not, the oldest event waiting is thrown away to make room for it,
// because what the person just did matters more than what they did a while ago.
func handedOver(events chan contract.BrowserEvent, event contract.BrowserEvent) bool {
	select {
	case events <- event:
		return true
	default:
	}
	select {
	case <-events:
	default:
	}
	select {
	case events <- event:
	default:
	}
	return false
}

// closeEveryReader ends every channel, which is what a worker that has gone owes
// the callers reading it. Calling it twice is harmless.
func (stream *eventStream) closeEveryReader() {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	stream.closed = true
	for reader := range stream.readers {
		finish(reader)
	}
	stream.readers = map[*eventReader]struct{}{}
}

// finish closes one reader's channel, once and no more than once.
func finish(reader *eventReader) {
	select {
	case <-reader.gone:
		return
	default:
	}
	close(reader.gone)
	close(reader.events)
}

// notification is a line the worker sent that nobody asked for, which the
// protocol allows for one thing only: an event saying what the person did.
type notification struct {
	// Version must be "2.0", the way every line of the protocol must be.
	Version string `json:"jsonrpc"`
	// ID is missing on a notification, which is what tells it from an answer.
	ID *int64 `json:"id"`
	// Method is the name of the notification, which is "event".
	Method string `json:"method"`
	// Params is the event itself.
	Params contract.BrowserEvent `json:"params"`
}

// eventInLine reads a line the worker sent as an event, and says no when the
// line is an answer to a request or an event the protocol does not define.
func eventInLine(line []byte) (contract.BrowserEvent, bool) {
	var sent notification
	if err := json.Unmarshal(line, &sent); err != nil {
		return contract.BrowserEvent{}, false
	}
	if sent.ID != nil || sent.Method != eventMethodName || sent.Version != "2.0" {
		return contract.BrowserEvent{}, false
	}
	if !contract.KnownBrowserEventKind(sent.Params.Kind) {
		return contract.BrowserEvent{}, false
	}
	return sent.Params, true
}

// eventMethodName is what worker/browser/PROTOCOL.md rule 7 calls the line the
// worker sends when the person does something themselves.
const eventMethodName = "event"

// Events is the stream of what the person did in the browser window themselves,
// which is what "/walk record" writes a procedure down from. It starts a worker
// when there is none, because there is nothing to watch without one.
func (browser *Browser) Events(ctx context.Context) (<-chan contract.BrowserEvent, error) {
	if _, err := browser.workerReady(ctx); err != nil {
		return nil, err
	}
	browser.guard.Lock()
	stream := browser.events
	browser.guard.Unlock()
	if stream == nil {
		return nil, errors.New("the browser worker is running with no event stream behind it, which is a fault in how it was wired")
	}
	return stream.listen(ctx)
}
