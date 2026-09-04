package testkit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// waitForAnEvent takes the next event off the stream, or fails the test when
// none arrives, so that a stream that has stopped is a failure rather than a
// test that hangs until the whole package times out.
func waitForAnEvent(t *testing.T, events <-chan contract.BrowserEvent) contract.BrowserEvent {
	t.Helper()
	select {
	case event, open := <-events:
		if !open {
			t.Fatal("the event stream closed while an event was still expected")
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived within five seconds")
		return contract.BrowserEvent{}
	}
}

func TestTheFakeBrowserEmitsTheEventsTheScriptNames(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	events, err := worker.Events(ctx)
	if err != nil {
		t.Fatalf("the fake browser would not hand out its event stream: %v", err)
	}
	worker.PersonDoes(
		contract.BrowserEvent{Kind: contract.BrowserEventClick, Ref: "e1", Text: "Change the page"},
		contract.BrowserEvent{Kind: contract.BrowserEventType, Ref: "e2", Length: 5},
		contract.BrowserEvent{Kind: contract.BrowserEventNavigate, Address: testkit.FixtureChangedPage},
	)

	clicked := waitForAnEvent(t, events)
	if clicked.Kind != contract.BrowserEventClick || clicked.Ref != "e1" || clicked.Text != "Change the page" {
		t.Errorf("the first event is %+v, want the click the script named", clicked)
	}
	if clicked.At.IsZero() {
		t.Error("the click event says nothing about when it happened, and a recording needs the order")
	}
	typed := waitForAnEvent(t, events)
	if typed.Kind != contract.BrowserEventType || typed.Length != 5 || typed.Text != "" {
		t.Errorf("the second event is %+v, want a typing event of five characters and no text", typed)
	}
	moved := waitForAnEvent(t, events)
	if moved.Kind != contract.BrowserEventNavigate || moved.Address != testkit.FixtureChangedPage {
		t.Errorf("the third event is %+v, want the navigation the script named", moved)
	}
}

func TestTheFakeBrowsersEventStreamClosesWhenTheWorkerGoes(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	events, err := worker.Events(context.Background())
	if err != nil {
		t.Fatalf("the fake browser would not hand out its event stream: %v", err)
	}

	if err := worker.Close(); err != nil {
		t.Fatalf("closing the fake browser failed: %v", err)
	}
	select {
	case _, open := <-events:
		if open {
			t.Error("an event arrived from a worker that has gone")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the event stream stayed open after the worker went")
	}
}

func TestTheFakeBrowsersEventStreamEndsWithItsContext(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	ctx, stop := context.WithCancel(context.Background())

	events, err := worker.Events(ctx)
	if err != nil {
		t.Fatalf("the fake browser would not hand out its event stream: %v", err)
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

func TestTheProtocolServerSendsTheEventsOnAsNotifications(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	connection, err := net.DialTimeout("unix", server.SocketPath(), 2*time.Second)
	if err != nil {
		t.Fatalf("cannot reach the browser worker's socket: %v", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("cannot set a deadline on the socket: %v", err)
	}
	reader := bufio.NewReader(connection)

	// One call first, so that the caller is known to be connected before the
	// person does anything in the window.
	if _, err := connection.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"health","params":{}}` + "\n")); err != nil {
		t.Fatalf("cannot ask the fake worker whether it is healthy: %v", err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("cannot read the health answer: %v", err)
	}
	worker.PersonDoes(contract.BrowserEvent{Kind: contract.BrowserEventClick, Ref: "e7", Text: "Post"})

	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("cannot read the event line: %v", err)
	}
	var sent struct {
		Version string                `json:"jsonrpc"`
		ID      any                   `json:"id"`
		Method  string                `json:"method"`
		Params  contract.BrowserEvent `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &sent); err != nil {
		t.Fatalf("the event line is not JSON: %v\n%s", err, line)
	}
	if sent.Version != "2.0" || sent.Method != "event" || sent.ID != nil {
		t.Errorf("the event line is %s, want a notification named event with no id", line)
	}
	if sent.Params.Kind != contract.BrowserEventClick || sent.Params.Ref != "e7" || sent.Params.Text != "Post" {
		t.Errorf("the event carried %+v, want the click the person made", sent.Params)
	}
}
