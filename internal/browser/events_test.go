package browser

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// nextEvent takes the next event off the stream, or fails the test when none
// arrives, so that a stream which has stopped is a failure rather than a test
// that hangs.
func nextEvent(t *testing.T, events <-chan contract.BrowserEvent) contract.BrowserEvent {
	t.Helper()
	select {
	case event, open := <-events:
		if !open {
			t.Fatal("the event stream closed while an event was still expected")
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived from the browser worker within five seconds")
		return contract.BrowserEvent{}
	}
}

func TestThePersonsOwnClicksArriveOnTheEventStream(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	events, err := browser.Events(context.Background())
	if err != nil {
		t.Fatalf("the browser would not hand out its event stream: %v", err)
	}
	world.worker.PersonDoes(
		contract.BrowserEvent{Kind: contract.BrowserEventClick, Ref: "e1", Text: "Change the page"},
		contract.BrowserEvent{Kind: contract.BrowserEventType, Ref: "e2", Length: 12},
	)

	clicked := nextEvent(t, events)
	if clicked.Kind != contract.BrowserEventClick || clicked.Ref != "e1" || clicked.Text != "Change the page" {
		t.Errorf("the first event is %+v, want the click the person made", clicked)
	}
	typed := nextEvent(t, events)
	if typed.Kind != contract.BrowserEventType || typed.Length != 12 || typed.Text != "" {
		t.Errorf("the second event is %+v, want a typing event of twelve characters and no text", typed)
	}
}

func TestAskingForTheEventStreamStartsTheWorker(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)

	if _, err := browser.Events(context.Background()); err != nil {
		t.Fatalf("the browser would not hand out its event stream: %v", err)
	}
	if !browser.Running() {
		t.Error("asking for the event stream left no worker running, and there is nothing to watch without one")
	}
}

func TestTheOldestEventIsDroppedPastTheCapWithOneNote(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) { options.BufferedEvents = 2 })
	openTheSimplePage(t, browser)

	events, err := browser.Events(context.Background())
	if err != nil {
		t.Fatalf("the browser would not hand out its event stream: %v", err)
	}
	for number := 1; number <= 5; number++ {
		world.worker.PersonDoes(contract.BrowserEvent{
			Kind: contract.BrowserEventNavigate,
			// The address is the number of the event, so the test can say which
			// ones were kept.
			Address: "https://fixture.test/" + string(rune('0'+number)),
		})
	}
	waitUntil(t, "the cap was reached and a note was written", func() bool {
		return len(events) == 2 && strings.Contains(world.notes.written(), "oldest")
	})

	first := nextEvent(t, events)
	second := nextEvent(t, events)
	if first.Address != "https://fixture.test/4" || second.Address != "https://fixture.test/5" {
		t.Errorf("the stream held %s and %s, want the last two, because the oldest are dropped", first.Address, second.Address)
	}
	if written := world.notes.written(); strings.Count(written, "oldest") != 1 {
		t.Errorf("the log says %q, and a slow reader is told once and not once per event", written)
	}
}

func TestTheEventStreamClosesWhenTheWorkerGoes(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	events, err := browser.Events(context.Background())
	if err != nil {
		t.Fatalf("the browser would not hand out its event stream: %v", err)
	}
	if err := browser.Close(); err != nil {
		t.Fatalf("closing the browser failed: %v", err)
	}

	select {
	case _, open := <-events:
		if open {
			t.Error("an event arrived from a browser worker that has been stopped")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the event stream stayed open after the worker was stopped")
	}
}

func TestTheEventStreamEndsWhenTheReaderStopsListening(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)
	ctx, stop := context.WithCancel(context.Background())

	events, err := browser.Events(ctx)
	if err != nil {
		t.Fatalf("the browser would not hand out its event stream: %v", err)
	}
	stop()

	select {
	case _, open := <-events:
		if open {
			t.Error("an event arrived after the reader had stopped listening")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the event stream stayed open after its context was cancelled")
	}
}

func TestAnEventDoesNotConfuseTheAnswerToACall(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	events, err := browser.Events(context.Background())
	if err != nil {
		t.Fatalf("the browser would not hand out its event stream: %v", err)
	}
	world.worker.PersonDoes(contract.BrowserEvent{Kind: contract.BrowserEventClick, Ref: "e1", Text: "Change the page"})

	page, err := browser.Read(context.Background(), contract.ReadOptions{})
	if err != nil {
		t.Fatalf("reading the page while events were arriving failed: %v", err)
	}
	if page.URL == "" {
		t.Errorf("the read came back with %+v, want the page the browser is on", page)
	}
	if event := nextEvent(t, events); event.Kind != contract.BrowserEventClick {
		t.Errorf("the event is %+v, want the click the person made", event)
	}
}

func TestTheEventBufferIsTheCapTheDesignGives(t *testing.T) {
	if DefaultBufferedEvents != contract.DefaultConfig().Caps.BufferedBrowserEvents {
		t.Errorf("this package holds %d events for a slow reader and the configuration says %d, and they are the same cap",
			DefaultBufferedEvents, contract.DefaultConfig().Caps.BufferedBrowserEvents)
	}
}
