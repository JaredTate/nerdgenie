package testkit

import (
	"context"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// fakeEventsHeld is how many events the fixture browser keeps for a reader that
// has fallen behind. A test scripts a handful, so this is room enough, and it is
// a cap rather than a growing list because nothing in this project holds an
// unbounded buffer.
const fakeEventsHeld = 64

// watcher is one reader of what the person did in the fixture browser.
type watcher struct {
	// events is where that reader takes them from.
	events chan contract.BrowserEvent
	// gone is closed when the reader is finished with, which is what stops the
	// goroutine watching its context.
	gone chan struct{}
}

// Events hands back the stream of what a person did in the window. The fixture
// browser has no window and no person in front of it, so it sends nothing on its
// own: a test says what the person did with PersonDoes.
func (worker *FakeBrowserWorker) Events(ctx context.Context) (<-chan contract.BrowserEvent, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if worker.closed {
		return nil, ErrBrowserGone
	}
	reader := &watcher{
		events: make(chan contract.BrowserEvent, fakeEventsHeld),
		gone:   make(chan struct{}),
	}
	worker.watchers = append(worker.watchers, reader)
	go worker.stopWatchingWhenDone(ctx, reader)
	return reader.events, nil
}

// PersonDoes is the script: it sends each event to everybody reading the stream,
// as though a person had done it in the window. An event with no time on it is
// stamped with the time now, because a recording is written in the order things
// happened.
func (worker *FakeBrowserWorker) PersonDoes(events ...contract.BrowserEvent) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	for _, event := range events {
		if event.At.IsZero() {
			event.At = time.Now().UTC()
		}
		for _, reader := range worker.watchers {
			sendOrDropTheOldest(reader.events, event)
		}
	}
}

// sendOrDropTheOldest puts one event in front of a reader, throwing the oldest
// away when the reader has fallen a whole buffer behind, which is what the
// contract promises every browser worker does.
func sendOrDropTheOldest(events chan contract.BrowserEvent, event contract.BrowserEvent) {
	select {
	case events <- event:
		return
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
}

// stopWatchingWhenDone ends one reader's stream when it stops listening, and
// ends itself when the stream was closed for some other reason.
func (worker *FakeBrowserWorker) stopWatchingWhenDone(ctx context.Context, reader *watcher) {
	select {
	case <-ctx.Done():
		worker.stopWatching(reader)
	case <-reader.gone:
	}
}

// stopWatching takes one reader off the list and closes its stream.
func (worker *FakeBrowserWorker) stopWatching(reader *watcher) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	for at, known := range worker.watchers {
		if known == reader {
			worker.watchers = append(worker.watchers[:at], worker.watchers[at+1:]...)
			finishWith(reader)
			return
		}
	}
}

// closeEveryWatcher ends every stream, which is what a worker that has gone owes
// the people reading it. The caller holds the guard.
func (worker *FakeBrowserWorker) closeEveryWatcher() {
	for _, reader := range worker.watchers {
		finishWith(reader)
	}
	worker.watchers = nil
}

// finishWith closes one reader's stream, once and no more than once.
func finishWith(reader *watcher) {
	select {
	case <-reader.gone:
		return
	default:
	}
	close(reader.gone)
	close(reader.events)
}
